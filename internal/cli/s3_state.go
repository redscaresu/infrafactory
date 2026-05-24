package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Third-party S3 backend state polyfill (M59) — SeaweedFS (default,
// Apache 2.0) and any other S3-compliant backend speaks the standard
// S3 wire surface (ListAllMyBuckets, ListObjectsV2) but has no
// `/mock/state` endpoint of its own. This file synthesises the JSON
// shape infrafactory's existing AWS state assertions (e.g.
// awsStateItemCount(state, "s3", "buckets") in TestE2E_AWSFullStack)
// expect, by calling the real S3 APIs and reshaping the response.
//
// Decision rationale for SeaweedFS over Adobe S3Mock / Garage / etc.
// is documented in CONCEPT.md "Third-Party Mock Integration" section.
// Short version: S3Mock only implements the object surface (no
// GetBucketPolicy / GetBucketTagging / etc.), Garage requires per-key
// bootstrap dance + is AGPLv3, LocalStack community is gone.
// SeaweedFS implements the full bucket-management surface with
// correct error codes (NoSuchBucketPolicy, NoSuchTagSet, etc.) and
// is Apache 2.0.
//
// Output shape, matching fakeaws's gatherS3State() block:
//
//	{
//	  "s3": {
//	    "buckets": [
//	      {"name": "...", "region": "...", "object_count": N, "creation_date": "..."},
//	      ...
//	    ]
//	  }
//	}
//
// The cloudMockStateRouter merges this into the fakeaws state when
// the loaded scenario is AWS-typed AND the s3 backend is configured
// — fakeaws's stripped-down S3 surface is shadowed by the third-party
// backend's authoritative view for these runs.

// s3HTTPTimeout caps any single S3 backend call. List flows poll
// every bucket; a runaway listing would otherwise stall a test.
const s3HTTPTimeout = 5 * time.Second

// listAllMyBucketsResultXML mirrors the standard S3
// ListAllMyBuckets XML response. Field names match the on-wire
// element names — Go's xml package handles the case conversion.
type listAllMyBucketsResultXML struct {
	XMLName xml.Name `xml:"ListAllMyBucketsResult"`
	Buckets struct {
		Bucket []struct {
			Name         string `xml:"Name"`
			BucketRegion string `xml:"BucketRegion"`
			CreationDate string `xml:"CreationDate"`
		} `xml:"Bucket"`
	} `xml:"Buckets"`
}

// listBucketResultV2XML is the standard ListObjectsV2 response.
// We only consume KeyCount (the bucket's object count) — full
// object listing isn't needed for the /mock/state shape.
type listBucketResultV2XML struct {
	XMLName  xml.Name `xml:"ListBucketResult"`
	KeyCount int      `xml:"KeyCount"`
}

// resetS3Backend wipes every bucket the configured S3 backend
// (SeaweedFS by default) currently holds, returning the backend to
// its initial-empty state. Mirrors the contract that mockway /
// fakegcp / fakeaws's /mock/reset endpoints satisfy, but goes
// through the native S3 admin surface: list every bucket, then
// list+delete every object in each bucket, then `DELETE /<bucket>`.
// Empty-bucket-delete is the standard S3 contract — SeaweedFS,
// MinIO, real S3 all enforce it.
func resetS3Backend(ctx context.Context, client *mockStateClient) error {
	httpClient := &http.Client{Timeout: s3HTTPTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+"/", nil)
	if err != nil {
		return fmt.Errorf("s3 reset: build list buckets: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("s3 reset: list buckets: %w", err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("s3 reset: list buckets status %d", resp.StatusCode)
	}
	var parsed listAllMyBucketsResultXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("s3 reset: parse list response: %w", err)
	}
	for _, b := range parsed.Buckets.Bucket {
		// Best-effort empty before delete. If listing/deleting
		// objects fails, the bucket delete will surface the real
		// reason (e.g., "BucketNotEmpty").
		_ = emptyS3Bucket(ctx, httpClient, client.baseURL, b.Name)
		delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete,
			client.baseURL+"/"+b.Name, bytes.NewReader(nil))
		if err != nil {
			return fmt.Errorf("s3 reset: build delete %s: %w", b.Name, err)
		}
		delResp, err := httpClient.Do(delReq)
		if err != nil {
			return fmt.Errorf("s3 reset: delete %s: %w", b.Name, err)
		}
		_ = delResp.Body.Close()
		if delResp.StatusCode != http.StatusNoContent && delResp.StatusCode != http.StatusOK {
			return fmt.Errorf("s3 reset: delete %s: status %d", b.Name, delResp.StatusCode)
		}
	}
	return nil
}

// emptyS3Bucket deletes every object in a bucket via the standard
// list+per-object-delete pattern. Skips pagination — fine for
// dev/test, would need ContinuationToken handling for buckets with
// > 1000 objects.
func emptyS3Bucket(ctx context.Context, httpClient *http.Client, baseURL, bucket string) error {
	listReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		baseURL+"/"+bucket+"?list-type=2", nil)
	if err != nil {
		return err
	}
	listResp, err := httpClient.Do(listReq)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(io.LimitReader(listResp.Body, 1<<20))
	_ = listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		return nil
	}
	// Cheap key extraction: find all <Key>...</Key>. Avoids a
	// dedicated struct since we only need the keys.
	for {
		i := bytes.Index(body, []byte("<Key>"))
		if i < 0 {
			return nil
		}
		j := bytes.Index(body[i+5:], []byte("</Key>"))
		if j < 0 {
			return nil
		}
		key := string(body[i+5 : i+5+j])
		body = body[i+5+j+6:]
		delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete,
			baseURL+"/"+bucket+"/"+key, nil)
		if err != nil {
			continue
		}
		delResp, err := httpClient.Do(delReq)
		if err != nil {
			continue
		}
		_ = delResp.Body.Close()
	}
}

// mergeS3IntoAWSState fetches the third-party S3 backend's bucket+
// object state, reshapes it into infrafactory's `{s3: {buckets:
// [...]}}` JSON, and overlays it onto the fakeaws state. Errors
// during the S3 fetch are non-fatal: the caller (cloudMockStateRouter
// .State) gets the fakeaws-only state back as a graceful degrade.
// The trade-off is that an unavailable S3 backend looks like "no
// buckets" instead of "S3 backend down" — assertions on bucket count
// will fail loudly, which surfaces the misconfiguration without
// breaking the rest of the run.
func mergeS3IntoAWSState(ctx context.Context, base []byte, client *mockStateClient) ([]byte, error) {
	buckets, err := fetchS3Buckets(ctx, client)
	if err != nil {
		return base, nil
	}
	var state map[string]any
	if err := json.Unmarshal(base, &state); err != nil {
		return nil, fmt.Errorf("merge s3 state: parse fakeaws state: %w", err)
	}
	state["s3"] = map[string]any{
		"buckets": buckets,
	}
	out, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("merge s3 state: re-marshal: %w", err)
	}
	return out, nil
}

// fetchS3Buckets calls the backend's standard ListAllMyBuckets, then
// for each bucket a single ListObjectsV2 to read KeyCount. Output
// is a slice of {name, region, object_count, creation_date} maps
// matching fakeaws's per-bucket shape.
func fetchS3Buckets(ctx context.Context, client *mockStateClient) ([]map[string]any, error) {
	httpClient := &http.Client{Timeout: s3HTTPTimeout}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+"/", nil)
	if err != nil {
		return nil, fmt.Errorf("s3 list buckets: build request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("s3 list buckets: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("s3 list buckets: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("s3 list buckets: read body: %w", err)
	}
	var parsed listAllMyBucketsResultXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("s3 list buckets: parse xml: %w", err)
	}

	out := make([]map[string]any, 0, len(parsed.Buckets.Bucket))
	for _, b := range parsed.Buckets.Bucket {
		region := strings.TrimSpace(b.BucketRegion)
		if region == "" {
			region = "us-east-1"
		}
		entry := map[string]any{
			"name":          b.Name,
			"region":        region,
			"creation_date": b.CreationDate,
			"object_count":  fetchBucketObjectCount(ctx, httpClient, client.baseURL, b.Name),
		}
		out = append(out, entry)
	}
	return out, nil
}

// fetchBucketObjectCount reads ListObjectsV2 for one bucket and
// returns KeyCount. Errors fall back to 0 (the count assertion is
// "at least N" so 0 is a clear "couldn't read" signal that fails
// the test loudly rather than silently passing with a wrong count).
func fetchBucketObjectCount(ctx context.Context, httpClient *http.Client, baseURL, bucket string) int {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/"+bucket+"?list-type=2", nil)
	if err != nil {
		return 0
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0
	}
	var parsed listBucketResultV2XML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return 0
	}
	return parsed.KeyCount
}
