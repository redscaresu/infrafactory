package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// stubBackend captures every request it receives so the router test
// can assert which backend got which traffic.
type stubBackend struct {
	mu       sync.Mutex
	requests []capturedRequest
	respond  func(req *http.Request) (status int, body string, header http.Header)
}

type capturedRequest struct {
	Method   string
	Path     string
	RawQuery string
	Header   http.Header
	Body     string
}

func (s *stubBackend) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.requests = append(s.requests, capturedRequest{
			Method:   r.Method,
			Path:     r.URL.Path,
			RawQuery: r.URL.RawQuery,
			Header:   r.Header.Clone(),
			Body:     string(body),
		})
		s.mu.Unlock()
		status := http.StatusOK
		respBody := ""
		if s.respond != nil {
			st, b, h := s.respond(r)
			status = st
			respBody = b
			for k, vv := range h {
				for _, v := range vv {
					w.Header().Add(k, v)
				}
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}
}

func (s *stubBackend) snapshot() []capturedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]capturedRequest, len(s.requests))
	copy(out, s.requests)
	return out
}

func setupRouter(t *testing.T) (*router, *stubBackend, *stubBackend) {
	t.Helper()
	sw := &stubBackend{}
	fa := &stubBackend{}
	swServer := httptest.NewServer(sw.handler())
	faServer := httptest.NewServer(fa.handler())
	t.Cleanup(swServer.Close)
	t.Cleanup(faServer.Close)
	swURL, _ := url.Parse(swServer.URL)
	faURL, _ := url.Parse(faServer.URL)
	return &router{seaweed: swURL, fakeaws: faURL}, sw, fa
}

// TestRouter_PublicAccessBlockGoesToFakeaws pins the core S80
// behaviour: any request whose query string names a known fakeaws
// subresource routes to fakeaws with `/s3` prefixed onto the path.
func TestRouter_PublicAccessBlockGoesToFakeaws(t *testing.T) {
	r, sw, fa := setupRouter(t)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	for _, tc := range []struct {
		name   string
		method string
		path   string
		query  string
	}{
		{"PUT no-value", "PUT", "/my-bucket", "publicAccessBlock"},
		{"PUT with-value", "PUT", "/my-bucket", "publicAccessBlock="},
		{"GET", "GET", "/my-bucket", "publicAccessBlock"},
		{"DELETE", "DELETE", "/my-bucket", "publicAccessBlock"},
		{"camelCase query", "GET", "/my-bucket", "PublicAccessBlock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, server.URL+tc.path+"?"+tc.query, strings.NewReader("body"))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			defer resp.Body.Close()

			faReqs := fa.snapshot()
			if len(faReqs) == 0 {
				t.Fatalf("expected fakeaws to receive the %s request, got none (seaweed got %d)", tc.name, len(sw.snapshot()))
			}
			got := faReqs[len(faReqs)-1]
			if got.Path != "/s3"+tc.path {
				t.Errorf("expected path /s3%s, got %s", tc.path, got.Path)
			}
		})
	}

	// SeaweedFS should NOT have seen any of the publicAccessBlock
	// requests.
	if got := len(sw.snapshot()); got != 0 {
		t.Errorf("seaweed should have received 0 publicAccessBlock requests, got %d", got)
	}
}

// TestRouter_PlainObjectGoesToSeaweed pins the negative case: any
// request without a fakeaws-subresource query goes to SeaweedFS,
// unmodified.
func TestRouter_PlainObjectGoesToSeaweed(t *testing.T) {
	r, sw, fa := setupRouter(t)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	for _, tc := range []struct {
		name   string
		method string
		path   string
		query  string
	}{
		{"GET object", "GET", "/my-bucket/key.txt", ""},
		{"PUT object", "PUT", "/my-bucket/key.txt", ""},
		{"GET list", "GET", "/my-bucket", "list-type=2&prefix=foo"},
		{"GET versioning subresource", "GET", "/my-bucket", "versioning"},
		{"GET policy subresource", "GET", "/my-bucket", "policy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, server.URL+tc.path+"?"+tc.query, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			defer resp.Body.Close()
		})
	}

	if got := len(fa.snapshot()); got != 0 {
		t.Errorf("fakeaws should have received 0 plain-object requests, got %d", got)
	}
	if got := len(sw.snapshot()); got < 5 {
		t.Errorf("seaweed should have received >= 5 plain-object requests, got %d", got)
	}
}

// TestRouter_BucketLifecycleFanOut pins the PUT/DELETE /<bucket>
// fan-out so the bucket exists in both backends. Required because
// a later /<bucket>?publicAccessBlock call needs the bucket in
// fakeaws's store; without the fan-out fakeaws would return 404 on
// the subresource.
func TestRouter_BucketLifecycleFanOut(t *testing.T) {
	r, sw, fa := setupRouter(t)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	// PUT /<bucket> with no query → fan-out.
	req, _ := http.NewRequest("PUT", server.URL+"/my-bucket", strings.NewReader("body"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	swReqs := sw.snapshot()
	faReqs := fa.snapshot()
	if len(swReqs) != 1 || swReqs[0].Path != "/my-bucket" {
		t.Errorf("expected 1 seaweed PUT /my-bucket, got %+v", swReqs)
	}
	if len(faReqs) != 1 || faReqs[0].Path != "/s3/my-bucket" {
		t.Errorf("expected 1 fakeaws PUT /s3/my-bucket, got %+v", faReqs)
	}

	// DELETE /<bucket> with no query → fan-out.
	req, _ = http.NewRequest("DELETE", server.URL+"/my-bucket", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK on DELETE, got %d", resp.StatusCode)
	}
	if got := len(sw.snapshot()); got != 2 {
		t.Errorf("expected seaweed to receive 2 reqs (PUT + DELETE), got %d", got)
	}
	if got := len(fa.snapshot()); got != 2 {
		t.Errorf("expected fakeaws to receive 2 reqs (PUT + DELETE), got %d", got)
	}
}

// TestRouter_BucketCreateRoutesNotKey pins that an object PUT to
// /<bucket>/<key> does NOT trigger fan-out (only the bucket itself
// should land in both backends).
func TestRouter_ObjectPutDoesNotFanOut(t *testing.T) {
	r, sw, fa := setupRouter(t)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	req, _ := http.NewRequest("PUT", server.URL+"/my-bucket/path/to/key", strings.NewReader("body"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()

	if got := len(sw.snapshot()); got != 1 {
		t.Errorf("expected 1 seaweed object PUT, got %d", got)
	}
	if got := len(fa.snapshot()); got != 0 {
		t.Errorf("expected 0 fakeaws calls for object PUT, got %d", got)
	}
}

// TestRouter_FanOutPrefersSeaweedOn2xx pins the response selection:
// when both backends 2xx, return SeaweedFS's response (it's the
// data-plane authority).
func TestRouter_FanOutPrefersSeaweedOn2xx(t *testing.T) {
	r, sw, fa := setupRouter(t)
	sw.respond = func(_ *http.Request) (int, string, http.Header) {
		return http.StatusOK, "seaweed-body", http.Header{"X-Source": []string{"seaweed"}}
	}
	fa.respond = func(_ *http.Request) (int, string, http.Header) {
		return http.StatusOK, "fakeaws-body", http.Header{"X-Source": []string{"fakeaws"}}
	}
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	req, _ := http.NewRequest("PUT", server.URL+"/my-bucket", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "seaweed-body" {
		t.Errorf("expected seaweed-body in primary response, got %q", body)
	}
	if resp.Header.Get("X-Source") != "seaweed" {
		t.Errorf("expected X-Source=seaweed, got %q", resp.Header.Get("X-Source"))
	}
}

// TestRouter_FanOutFallsBackOnSeaweedError pins that if SeaweedFS
// fails the fan-out PUT, the client still sees a success when
// fakeaws succeeded — but only because the bucket exists in at
// least one backend. We promote fakeaws's response in that case.
func TestRouter_FanOutFallsBackOnSeaweedError(t *testing.T) {
	r, sw, fa := setupRouter(t)
	sw.respond = func(_ *http.Request) (int, string, http.Header) {
		return http.StatusInternalServerError, "boom", nil
	}
	fa.respond = func(_ *http.Request) (int, string, http.Header) {
		return http.StatusOK, "fakeaws-body", nil
	}
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	req, _ := http.NewRequest("PUT", server.URL+"/my-bucket", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "fakeaws-body" {
		t.Errorf("expected fakeaws promotion: got status=%d body=%q", resp.StatusCode, body)
	}
}

// TestSubresourceForFakeaws pins the predicate directly so future
// edits to fakeawsSubresources are caught.
func TestSubresourceForFakeaws(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  bool
	}{
		{"publicAccessBlock", true},
		{"publicAccessBlock=", true},
		{"PUBLICACCESSBLOCK", true},
		{"policy", false},
		{"versioning", false},
		{"", false},
		{"list-type=2", false},
	} {
		if got := subresourceForFakeaws(tc.query); got != tc.want {
			t.Errorf("subresourceForFakeaws(%q): got %v, want %v", tc.query, got, tc.want)
		}
	}
}

// TestSplitS3Path exercises the helper directly.
func TestSplitS3Path(t *testing.T) {
	cases := []struct {
		path, bucket, rest string
	}{
		{"/foo", "foo", "/"},
		{"/foo/", "foo", "/"},
		{"/foo/bar", "foo", "/bar"},
		{"/foo/bar/baz", "foo", "/bar/baz"},
		{"/", "", "/"},
		{"", "", "/"},
	}
	for _, tc := range cases {
		b, r := splitS3Path(tc.path)
		if b != tc.bucket || r != tc.rest {
			t.Errorf("splitS3Path(%q): got (%q,%q), want (%q,%q)", tc.path, b, r, tc.bucket, tc.rest)
		}
	}
}

// TestIsBucketLifecycle pins the PUT/DELETE /<bucket> predicate.
func TestIsBucketLifecycle(t *testing.T) {
	mk := func(method, path, query string) *http.Request {
		u, _ := url.Parse("http://x" + path + map[bool]string{true: "?" + query, false: ""}[query != ""])
		return &http.Request{Method: method, URL: u}
	}
	cases := []struct {
		name string
		req  *http.Request
		want bool
	}{
		{"PUT bucket no query", mk("PUT", "/my-bucket", ""), true},
		{"DELETE bucket no query", mk("DELETE", "/my-bucket", ""), true},
		{"GET bucket no query", mk("GET", "/my-bucket", ""), false},
		{"PUT bucket with query", mk("PUT", "/my-bucket", "publicAccessBlock"), false},
		{"PUT object", mk("PUT", "/my-bucket/key", ""), false},
		{"PUT root", mk("PUT", "/", ""), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBucketLifecycle(tc.req); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// helper so the linter doesn't complain about unused atomic.
var _ = atomic.AddInt64
