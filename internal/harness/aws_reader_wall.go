package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// ErrAWSReadWallRefused is wrapped by every request the wall refuses.
var ErrAWSReadWallRefused = errors.New("aws read wall: refused")

// The media types the SDK sends for each protocol; the wall passes no
// other, so it never validates a body the server would read another way.
// Every json service in the reader table (SSM) speaks json 1.1.
const (
	awsQueryMediaType = "application/x-www-form-urlencoded"
	awsJSONMediaType  = "application/x-amz-json-1.1"
)

// AWSReadWall is the doer every account-read request leaves through,
// bound to one reader-table service. Do passes a request on only when it
// is exactly one of that service's reader-table actions or, on EC2, a
// DryRun-table action carrying the single field DryRun=true; anything
// else is refused before it leaves the process. It counts what it sends.
// What it admits is copied from the tables when it is built, so a later
// edit to them cannot widen it.
type AWSReadWall struct {
	service, protocol, targetPrefix string
	reads, dryRuns                  map[string]bool
	inner                           aws.HTTPClient
	calls                           atomic.Int64
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
	w := &AWSReadWall{
		service: svc.Service, protocol: svc.Protocol, targetPrefix: svc.TargetPrefix,
		reads: awsActionSet(svc.Actions), inner: inner,
	}
	if service == awsDryRunService {
		w.dryRuns = awsActionSet(AWSReaderDryRunActions)
	}
	return w, nil
}

func awsActionSet(actions []string) map[string]bool {
	set := make(map[string]bool, len(actions))
	for _, action := range actions {
		set[action] = true
	}
	return set
}

// Calls is the number of requests the wall has passed on.
func (w *AWSReadWall) Calls() int {
	return int(w.calls.Load())
}

// Do refuses req, with nothing sent, unless the wall admits it; then it
// counts it and sends it through the inner doer.
func (w *AWSReadWall) Do(req *http.Request) (*http.Response, error) {
	entry, dryRun, err := w.entryOf(req)
	if err == nil {
		err = w.admit(entry.Action, dryRun)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s request: %w", ErrAWSReadWallRefused, w.service, err)
	}
	w.calls.Add(1)
	return w.inner.Do(req)
}

func (w *AWSReadWall) admit(action string, dryRun bool) error {
	switch {
	case w.reads[action]:
		return nil
	case w.dryRuns[action]:
		if dryRun {
			return nil
		}
		return fmt.Errorf("%s is sent only with the single field DryRun=true", action)
	default:
		return fmt.Errorf("%s %q is not in the reader table", w.service, action)
	}
}

// entryOf reads req into its ADR-0041 entry and puts the body back
// byte-identical. dryRun reports the field DryRun=true. It refuses any
// request whose wire form could carry an action or a DryRun the entry
// does not show.
func (w *AWSReadWall) entryOf(req *http.Request) (AWSReadEntry, bool, error) {
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
	if err := w.checkContentType(req.Header); err != nil {
		return AWSReadEntry{}, false, err
	}
	targets := awsHeaderValues(req.Header, "X-Amz-Target")
	if w.protocol == awsProtocolJSON {
		entry, err := w.jsonEntry(targets, body)
		return entry, false, err
	}
	if len(targets) > 0 {
		return AWSReadEntry{}, false, errors.New("X-Amz-Target on a query service")
	}
	return w.queryEntry(body)
}

// checkContentType passes exactly one Content-Type of the protocol's
// media type, with no parameter but a query body's charset=utf-8.
func (w *AWSReadWall) checkContentType(h http.Header) error {
	types := awsHeaderValues(h, "Content-Type")
	if len(types) != 1 {
		return fmt.Errorf("the request carries %d Content-Type values, not one", len(types))
	}
	mediaType, params, err := mime.ParseMediaType(types[0])
	if err != nil {
		return fmt.Errorf("the Content-Type %q does not parse: %w", types[0], err)
	}
	want := awsQueryMediaType
	if w.protocol == awsProtocolJSON {
		want = awsJSONMediaType
	}
	if mediaType != want {
		return fmt.Errorf("the Content-Type is %q, not %s", mediaType, want)
	}
	for key, value := range params {
		if w.protocol != awsProtocolQuery || key != "charset" || !strings.EqualFold(value, "utf-8") {
			return fmt.Errorf("the Content-Type carries %s=%q; only a query body's charset=utf-8 may pass", key, value)
		}
	}
	return nil
}

func (w *AWSReadWall) queryEntry(body []byte) (AWSReadEntry, bool, error) {
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
	return AWSReadEntry{Service: w.service, Protocol: w.protocol, Action: form.Get("Action"), Params: raw}, hasDryRun, nil
}

func (w *AWSReadWall) jsonEntry(targets []string, body []byte) (AWSReadEntry, error) {
	if len(targets) != 1 {
		return AWSReadEntry{}, fmt.Errorf("the request carries %d X-Amz-Target values, not one", len(targets))
	}
	prefix, action, ok := strings.Cut(targets[0], ".")
	if !ok || prefix != w.targetPrefix {
		return AWSReadEntry{}, fmt.Errorf("X-Amz-Target %q is not %s.<action>", targets[0], w.targetPrefix)
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
	return AWSReadEntry{Service: w.service, Protocol: w.protocol, Action: action, Params: body}, nil
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
