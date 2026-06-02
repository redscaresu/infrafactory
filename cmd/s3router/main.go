// s3router — minimal HTTP shim that splits S3 traffic between
// SeaweedFS (bucket + object data plane) and fakeaws (bucket
// subresources that SeaweedFS doesn't implement). Specifically:
//
//   - /<bucket>?publicAccessBlock → fakeaws (SeaweedFS returns 501).
//   - PUT /<bucket> (no query)    → fan out to both, so the bucket
//     exists in both backends for subsequent reads.
//   - DELETE /<bucket> (no query) → fan out to both.
//   - everything else             → SeaweedFS.
//
// Why this exists: the terraform-provider-aws `aws_s3_bucket_public_access_block`
// resource calls PUT/GET/DELETE `<bucket>?publicAccessBlock`.
// SeaweedFS responds 501 NotImplemented for that subresource and
// breaks any scenario that uses public-access-block configuration.
// fakeaws DOES implement the subresource (handlers/s3.go, since
// S43-T8) but only under its own /s3/ prefix; pointing s3.url at
// fakeaws is not viable because fakeaws's S3 surface is a
// stripped-down direct-HTTP fixture, not a full terraform-provider-aws
// Read target. The shim is the cheapest middle ground.
//
// Scope-bounded by design: today only ?publicAccessBlock routes to
// fakeaws. If a future scenario surfaces another SeaweedFS 501,
// add the subresource name to the routing set below. Don't generalise
// preemptively — every added route is one more wire-shape to keep
// in sync between two backends.
//
// Closes S80 from docs/plans/slices-79-83-plan.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// fakeawsSubresources lists the S3 bucket subresources that
// SeaweedFS doesn't model (or models differently from real AWS) and
// that fakeaws DOES handle. Lower-cased; matched against the raw
// query string by substring presence. Order doesn't matter.
//
// Add to this set when a new SeaweedFS 501 surfaces. Until then,
// resist the urge to predict — each added route burns a coordination
// surface between two backends.
var fakeawsSubresources = []string{
	"publicaccessblock",
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9091", "listen address")
	seaweed := flag.String("seaweed-url", "http://127.0.0.1:9090", "SeaweedFS S3 endpoint")
	fakeaws := flag.String("fakeaws-url", "http://127.0.0.1:8082", "fakeaws endpoint (path /s3 prefix added)")
	flag.Parse()

	swURL, err := url.Parse(*seaweed)
	if err != nil {
		log.Fatalf("bad --seaweed-url: %v", err)
	}
	faURL, err := url.Parse(*fakeaws)
	if err != nil {
		log.Fatalf("bad --fakeaws-url: %v", err)
	}

	r := &router{seaweed: swURL, fakeaws: faURL}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("s3router listening on %s (seaweed=%s fakeaws=%s)", *addr, swURL, faURL)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}

type router struct {
	seaweed *url.URL
	fakeaws *url.URL
}

func (r *router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	bucket, _ := splitS3Path(req.URL.Path)

	if bucket != "" && isBucketLifecycle(req) {
		r.fanOutBucket(w, req)
		return
	}

	if subresourceForFakeaws(req.URL.RawQuery) {
		r.forwardFakeaws(w, req)
		return
	}

	r.forwardSeaweed(w, req)
}

// splitS3Path returns (bucket, rest) for a path like
// "/foo/bar/baz" → ("foo", "/bar/baz"). Empty path → ("", "/").
func splitS3Path(path string) (bucket, rest string) {
	trimmed := strings.TrimPrefix(path, "/")
	if trimmed == "" {
		return "", "/"
	}
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		return trimmed[:idx], "/" + trimmed[idx+1:]
	}
	return trimmed, "/"
}

// isBucketLifecycle is true for the bucket Create/Delete shapes that
// must exist in BOTH backends so subsequent subresource calls find
// the bucket. PUT/DELETE on /<bucket> with no query string.
func isBucketLifecycle(req *http.Request) bool {
	if req.URL.RawQuery != "" {
		return false
	}
	if req.URL.Path == "/" {
		return false
	}
	// Path must be exactly /<bucket> (no trailing key segment).
	rest := strings.TrimPrefix(req.URL.Path, "/")
	if strings.Contains(rest, "/") && rest[len(rest)-1] != '/' {
		return false
	}
	return req.Method == http.MethodPut || req.Method == http.MethodDelete
}

// subresourceForFakeaws is true if the request's query string names
// a subresource we route to fakeaws. Matches by substring on the
// raw query (case-insensitive), so `?publicAccessBlock` and
// `?publicAccessBlock=` both match.
func subresourceForFakeaws(rawQuery string) bool {
	if rawQuery == "" {
		return false
	}
	q := strings.ToLower(rawQuery)
	for _, sub := range fakeawsSubresources {
		if strings.Contains(q, sub) {
			return true
		}
	}
	return false
}

func (r *router) forwardSeaweed(w http.ResponseWriter, req *http.Request) {
	proxy := httputil.NewSingleHostReverseProxy(r.seaweed)
	proxy.ErrorHandler = proxyErrorHandler
	proxy.ServeHTTP(w, req)
}

// forwardFakeaws rewrites the request URL to prepend /s3 (fakeaws's
// S3 router lives there per ../fakeaws/handlers/s3.go:31) and
// forwards. Preserves method, headers, body, and query string.
func (r *router) forwardFakeaws(w http.ResponseWriter, req *http.Request) {
	proxy := httputil.NewSingleHostReverseProxy(r.fakeaws)
	director := proxy.Director
	proxy.Director = func(out *http.Request) {
		director(out)
		out.URL.Path = "/s3" + req.URL.Path
		out.Host = r.fakeaws.Host
	}
	proxy.ErrorHandler = proxyErrorHandler
	proxy.ServeHTTP(w, req)
}

// fanOutBucket sends the request to both backends concurrently and
// returns the SeaweedFS response if both succeed (2xx). If either
// returns an error or non-2xx, returns the first failure verbatim.
// On bucket DELETE, "Not Found" from one backend is benign — treat
// 404 as success when paired with a 2xx or 404 from the other.
func (r *router) fanOutBucket(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadGateway)
		return
	}
	_ = req.Body.Close()

	type result struct {
		name string
		resp *http.Response
		body []byte
		err  error
	}

	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()

	send := func(base *url.URL, prefix string) result {
		u := *base
		u.Path = strings.TrimSuffix(prefix, "/") + req.URL.Path
		u.RawQuery = req.URL.RawQuery
		newReq, _ := http.NewRequestWithContext(ctx, req.Method, u.String(), strings.NewReader(string(body)))
		for k, vv := range req.Header {
			for _, v := range vv {
				newReq.Header.Add(k, v)
			}
		}
		newReq.Header.Set("Host", base.Host)
		resp, err := http.DefaultClient.Do(newReq)
		if err != nil {
			return result{name: base.String(), err: err}
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return result{name: base.String(), resp: resp, body: b}
	}

	var wg sync.WaitGroup
	var sw, fa result
	wg.Add(2)
	go func() { defer wg.Done(); sw = send(r.seaweed, "") }()
	go func() { defer wg.Done(); fa = send(r.fakeaws, "/s3") }()
	wg.Wait()

	primary := sw
	secondary := fa
	if !ok(primary.resp, primary.err, req.Method) && ok(secondary.resp, secondary.err, req.Method) {
		primary, secondary = secondary, primary
	}

	if primary.err != nil {
		http.Error(w, primary.err.Error(), http.StatusBadGateway)
		return
	}
	for k, vv := range primary.resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(primary.resp.StatusCode)
	_, _ = w.Write(primary.body)
	_ = secondary // result intentionally unused beyond ranking
}

func ok(resp *http.Response, err error, method string) bool {
	if err != nil || resp == nil {
		return false
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true
	}
	// DELETE: 404 means "already gone", which is benign for fan-out.
	if method == http.MethodDelete && resp.StatusCode == http.StatusNotFound {
		return true
	}
	return false
}

func proxyErrorHandler(w http.ResponseWriter, req *http.Request, err error) {
	if isCanceled(err) {
		// Client disconnected — quiet failure.
		return
	}
	log.Printf("s3router: proxy error for %s %s: %v", req.Method, req.URL, err)
	http.Error(w, fmt.Sprintf("s3router: upstream error: %v", err), http.StatusBadGateway)
}

func isCanceled(err error) bool {
	if err == nil {
		return false
	}
	if err == context.Canceled || err == context.DeadlineExceeded {
		return true
	}
	if ne, isNet := err.(net.Error); isNet && ne.Timeout() {
		return true
	}
	return strings.Contains(err.Error(), "context canceled")
}
