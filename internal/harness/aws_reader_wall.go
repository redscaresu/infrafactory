package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// ErrAWSReadWallRefused is wrapped by every request the wall refuses.
var ErrAWSReadWallRefused = errors.New("aws read wall: refused")

// AWSReadWall is the doer every account-read request leaves through,
// bound to one reader-table service. Do passes a request on only when it
// is exactly one of that service's reader-table actions or, on EC2, a
// DryRun-table action carrying the single field DryRun=true; anything
// else is refused before it leaves the process. It counts what it sends.
type AWSReadWall struct {
	service AWSReaderService
	inner   aws.HTTPClient
	calls   atomic.Int64
}

// NewAWSReadWall binds a wall to service, a reader-table service, around
// inner, which sends what the wall passes.
func NewAWSReadWall(service string, inner aws.HTTPClient) (*AWSReadWall, error) {
	svc, ok := awsReaderService(service)
	if !ok {
		return nil, fmt.Errorf("aws read wall: %q is not a reader-table service", service)
	}
	if inner == nil {
		return nil, errors.New("aws read wall: needs an HTTP client, and none was given")
	}
	return &AWSReadWall{service: svc, inner: inner}, nil
}

// Calls is the number of requests the wall has passed on.
func (w *AWSReadWall) Calls() int {
	return int(w.calls.Load())
}

// Do refuses req, with nothing sent, unless the wall admits it; then it
// counts it and sends it through the inner doer.
func (w *AWSReadWall) Do(req *http.Request) (*http.Response, error) {
	entry, dryRun, err := awsReadEntryOf(w.service, req)
	if err == nil {
		err = admitAWSRead(entry, dryRun)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s request: %w", ErrAWSReadWallRefused, w.service.Service, err)
	}
	w.calls.Add(1)
	return w.inner.Do(req)
}

func admitAWSRead(entry AWSReadEntry, dryRun bool) error {
	switch {
	case awsReaderTableHas(entry):
		return nil
	case entry.Service == awsDryRunService && slices.Contains(AWSReaderDryRunActions, entry.Action):
		if dryRun {
			return nil
		}
		return fmt.Errorf("%s is sent only with the single field DryRun=true", entry.Action)
	default:
		return fmt.Errorf("%s %q is not in the reader table", entry.Service, entry.Action)
	}
}

// awsReadEntryOf reads req into its ADR-0041 entry for svc and puts the
// body back byte-identical. dryRun reports the field DryRun=true. It
// refuses any request whose wire form could carry an action or a DryRun
// the entry does not show.
func awsReadEntryOf(svc AWSReaderService, req *http.Request) (AWSReadEntry, bool, error) {
	body, err := restoreAWSRequestBody(req)
	if err != nil {
		return AWSReadEntry{}, false, err
	}
	// RequestURI is what goes on the wire, Opaque included.
	if req.URL == nil || strings.Contains(req.URL.RequestURI(), "?") {
		return AWSReadEntry{}, false, errors.New("the URL carries a query; parameters go in the body")
	}
	if enc := awsHeaderValues(req.Header, "Content-Encoding"); len(enc) > 0 {
		return AWSReadEntry{}, false, fmt.Errorf("the body is encoded (%q), so it is not the bytes read here", enc)
	}
	targets := awsHeaderValues(req.Header, "X-Amz-Target")
	if svc.Protocol == awsProtocolJSON {
		entry, err := awsJSONReadEntry(svc, targets, body)
		return entry, false, err
	}
	if len(targets) > 0 {
		return AWSReadEntry{}, false, errors.New("X-Amz-Target on a query service")
	}
	return awsQueryReadEntry(svc, body)
}

func awsQueryReadEntry(svc AWSReaderService, body []byte) (AWSReadEntry, bool, error) {
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return AWSReadEntry{}, false, fmt.Errorf("the form body does not parse: %w", err)
	}
	params := map[string]string{}
	for key, values := range form {
		if len(values) != 1 {
			return AWSReadEntry{}, false, fmt.Errorf("the form repeats %q", key)
		}
		for _, name := range []string{"Action", "DryRun"} {
			if key != name && strings.EqualFold(key, name) {
				return AWSReadEntry{}, false, fmt.Errorf("the form carries %q, a case variant of %s", key, name)
			}
		}
		if key != "Action" && key != "Version" {
			params[key] = values[0]
		}
	}
	if !form.Has("Action") {
		return AWSReadEntry{}, false, errors.New("the request names no action: the form has no Action and there is no X-Amz-Target")
	}
	dryRun, hasDryRun := params["DryRun"]
	if hasDryRun && dryRun != "true" {
		return AWSReadEntry{}, false, fmt.Errorf("DryRun is %q, not exactly true", dryRun)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return AWSReadEntry{}, false, err
	}
	return AWSReadEntry{Service: svc.Service, Protocol: svc.Protocol, Action: form.Get("Action"), Params: raw}, hasDryRun, nil
}

func awsJSONReadEntry(svc AWSReaderService, targets []string, body []byte) (AWSReadEntry, error) {
	if len(targets) != 1 {
		return AWSReadEntry{}, fmt.Errorf("the request carries %d X-Amz-Target values, not one", len(targets))
	}
	prefix, action, ok := strings.Cut(targets[0], ".")
	if !ok || prefix != svc.TargetPrefix {
		return AWSReadEntry{}, fmt.Errorf("X-Amz-Target %q is not %s.<action>", targets[0], svc.TargetPrefix)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return AWSReadEntry{}, errors.New("the body is not a JSON object")
	}
	for key := range fields {
		if strings.EqualFold(key, "Action") {
			return AWSReadEntry{}, fmt.Errorf("the json body carries %q, a query-protocol field", key)
		}
	}
	return AWSReadEntry{Service: svc.Service, Protocol: svc.Protocol, Action: action, Params: body}, nil
}

// restoreAWSRequestBody reads req's body and puts the same bytes back,
// for the send and for any resend through GetBody.
func restoreAWSRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	if err = errors.Join(err, req.Body.Close()); err != nil {
		return nil, fmt.Errorf("reading the body: %w", err)
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	return body, nil
}

// awsHeaderValues is every value of name in h, whatever its key's case: a
// key set straight into the map skips canonicalisation but still goes on
// the wire.
func awsHeaderValues(h http.Header, name string) []string {
	var values []string
	for key, v := range h {
		if strings.EqualFold(key, name) {
			values = append(values, v...)
		}
	}
	return values
}
