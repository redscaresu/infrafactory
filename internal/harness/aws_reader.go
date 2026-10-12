package harness

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// AWSReadInput is one account read. Identity and Preflight are
// VerifyAWSReadIdentity and PreflightAWSReadKey in a real read; a nil one
// refuses the read with nothing sent.
type AWSReadInput struct {
	// Config carries the read key and the region (AWSReaderConfig).
	Config aws.Config
	// Account must match the key's; Principal, when set, its ARN.
	Account, Principal string
	ReadSet            []AWSReadEntry
	// Doer sends every request, each through its service's wall.
	Doer      aws.HTTPClient
	Identity  func(ctx context.Context, cfg aws.Config, wall *AWSReadWall, account, principal string) error
	Preflight func(ctx context.Context, cfg aws.Config, wall *AWSReadWall) error
	// Endpoints replaces a service's endpoint URL, by service, for tests
	// against a local mock: each must be http or https on a loopback IP.
	Endpoints map[string]string
	// Now times the read (its timestamp and duration); nil means
	// time.Now. Requests are always signed at the real time, which AWS
	// checks.
	Now func() time.Time
	// Sleep waits out a throttled request's backoff; nil means a real
	// sleep.
	Sleep func(context.Context, time.Duration) error
}

// awsPartitionRegionPattern is the aws partition's regionRegex in the
// SDK's partition metadata (internal/endpoints/awsrulesfn/partitions.go).
// awsReadEndpoints hosts every service on amazonaws.com with IAM signed
// for us-east-1, which holds for no other partition (cn-*, us-gov-*,
// eusc-*, the iso partitions), so the read admits only this one.
var awsPartitionRegionPattern = regexp.MustCompile(`^(us|eu|ap|sa|ca|me|af|il|mx)\-\w+\-\d+$`)

// awsReadEndpoint is where a service's requests go, the region they are
// signed for and, for query, the API version.
type awsReadEndpoint struct {
	url, signingRegion, version string
}

// awsReadEndpoints is every reader-table service's endpoint in region.
// IAM is global: one endpoint, signed for us-east-1.
func awsReadEndpoints(region string) map[string]awsReadEndpoint {
	return map[string]awsReadEndpoint{
		"ec2": {url: "https://ec2." + region + ".amazonaws.com", signingRegion: region, version: "2016-11-15"},
		"iam": {url: "https://iam.amazonaws.com", signingRegion: "us-east-1", version: "2010-05-08"},
		"ssm": {url: "https://ssm." + region + ".amazonaws.com", signingRegion: region},
		"sts": {url: "https://sts." + region + ".amazonaws.com", signingRegion: region, version: "2011-06-15"},
	}
}

// ReadAWSAccount validates the read set, checks the key's identity, runs
// the preflight, then replays each entry verbatim, signed, through its
// service's wall, and returns the ADR-0041 recorded read. No request is
// sent unless every entry is a reader-table row whose wire request its
// wall admits, and none after a failing identity check or preflight. A
// list is read to its last page, each page an exchange, and a throttled
// request is sent again after a backoff (awsReplay.pages). A NotFound-class
// answer named in the reader table, to an entry's first request, is
// recorded; any other non-2xx answer, and a list that cannot be read in
// full, fails the read, naming the entry.
func ReadAWSAccount(ctx context.Context, in AWSReadInput) (AWSRecordedRead, error) {
	r, err := newAWSReplay(ctx, in)
	if err != nil {
		return AWSRecordedRead{}, fmt.Errorf("aws read: refusing: %w", err)
	}
	start := r.now()
	if err := in.Identity(ctx, r.endpoints["sts"].config(in.Config), r.walls["sts"], in.Account, in.Principal); err != nil {
		return AWSRecordedRead{}, fmt.Errorf("aws read: %w", err)
	}
	if err := in.Preflight(ctx, r.endpoints["ec2"].config(in.Config), r.walls["ec2"]); err != nil {
		return AWSRecordedRead{}, fmt.Errorf("aws read: %w", err)
	}
	exchanges := make([]AWSReadExchange, 0, len(in.ReadSet))
	for i, entry := range in.ReadSet {
		pages, err := r.pages(ctx, entry)
		if err != nil {
			return AWSRecordedRead{}, fmt.Errorf("aws read: %s: %w", awsReadEntryName(i, entry), err)
		}
		exchanges = append(exchanges, pages...)
	}
	calls := 0
	for _, wall := range r.walls {
		calls += wall.Calls()
	}
	return AWSRecordedRead{
		Account: in.Account, Region: in.Config.Region,
		Timestamp: start.UTC(), DurationMS: r.now().Sub(start).Milliseconds(),
		Calls: calls, Exchanges: exchanges,
	}, nil
}

// awsReplay is what replaying an entry needs: a wall and an endpoint per
// service, the key and the clock.
type awsReplay struct {
	walls     map[string]*AWSReadWall
	endpoints map[string]awsReadEndpoint
	creds     aws.Credentials
	signer    *v4.Signer
	now       func() time.Time
	sleep     func(context.Context, time.Duration) error
}

// newAWSReplay checks every input a read needs and builds its walls,
// sending nothing.
func newAWSReplay(ctx context.Context, in AWSReadInput) (*awsReplay, error) {
	switch {
	case in.Identity == nil:
		return nil, errors.New("no identity check was given")
	case in.Preflight == nil:
		return nil, errors.New("no preflight was given")
	case in.Config.Credentials == nil:
		return nil, errors.New("the config carries no credentials")
	case !ValidAWSRegion(in.Config.Region):
		return nil, fmt.Errorf("region %q is not an AWS region name", in.Config.Region)
	case !awsPartitionRegionPattern.MatchString(in.Config.Region):
		return nil, fmt.Errorf("region %q is outside the aws partition, the only one the read's endpoints serve", in.Config.Region)
	}
	if len(in.ReadSet) == 0 {
		// A read of nothing would record no exchanges, which a load could
		// take for an empty account.
		return nil, errors.New("the read set is empty")
	}
	if err := ValidateAWSReadSet(in.ReadSet); err != nil {
		return nil, err
	}
	r := &awsReplay{
		walls: map[string]*AWSReadWall{}, endpoints: awsReadEndpoints(in.Config.Region),
		signer: v4.NewSigner(), now: in.Now, sleep: in.Sleep,
	}
	if r.now == nil {
		r.now = time.Now
	}
	if r.sleep == nil {
		r.sleep = sleepContext
	}
	for service, u := range in.Endpoints {
		endpoint, ok := r.endpoints[service]
		if !ok {
			return nil, fmt.Errorf("an endpoint is given for %q, which is not a reader-table service", service)
		}
		if err := checkAWSLoopbackEndpoint(u); err != nil {
			return nil, fmt.Errorf("the %s endpoint: %w", service, err)
		}
		endpoint.url = u
		r.endpoints[service] = endpoint
	}
	for _, svc := range AWSReaderTable {
		wall, err := NewAWSReadWall(svc.Service, in.Doer)
		if err != nil {
			return nil, err
		}
		r.walls[svc.Service] = wall
	}
	// Each entry's wire request must pass its wall before anything is
	// sent, identity and preflight included; the wall's check is the rule.
	for i, entry := range in.ReadSet {
		if _, _, err := r.request(ctx, entry); err != nil {
			return nil, fmt.Errorf("%s: %w", awsReadEntryName(i, entry), err)
		}
	}
	creds, err := in.Config.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("the read key: %w", err)
	}
	r.creds = creds
	return r, nil
}

// config is cfg sending to the endpoint, for the SDK clients of the
// identity check and the preflight.
func (e awsReadEndpoint) config(cfg aws.Config) aws.Config {
	cfg.BaseEndpoint = aws.String(e.url)
	return cfg
}

// checkAWSLoopbackEndpoint passes an http or https URL whose host is a
// loopback IP, so an override cannot send the read key's signatures to
// another host.
func checkAWSLoopbackEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("%q does not parse: %w", endpoint, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%q is not http or https", endpoint)
	}
	if ip := net.ParseIP(u.Hostname()); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%q is not on a loopback IP", endpoint)
	}
	return nil
}

// replay signs and sends entry once and returns its answer, unchecked.
func (r *awsReplay) replay(ctx context.Context, entry AWSReadEntry) (AWSReadExchange, error) {
	ctx, cancel := context.WithTimeout(ctx, awsReadCallTimeout)
	defer cancel()
	req, signingRegion, err := r.request(ctx, entry)
	if err != nil {
		return AWSReadExchange{}, err
	}
	payload, err := restoreAWSRequestBody(req)
	if err != nil {
		return AWSReadExchange{}, err
	}
	hash := sha256.Sum256(payload)
	if err := r.signer.SignHTTP(ctx, r.creds, req, hex.EncodeToString(hash[:]), entry.Service, signingRegion, time.Now()); err != nil {
		return AWSReadExchange{}, fmt.Errorf("signing: %w", err)
	}
	resp, err := r.walls[entry.Service].Do(req)
	if err != nil {
		return AWSReadExchange{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return AWSReadExchange{}, fmt.Errorf("reading the answer: %w", err)
	}
	return AWSReadExchange{AWSReadEntry: entry, Status: resp.StatusCode, Body: string(body)}, nil
}

// request builds entry's unsigned wire request, which its wall has
// admitted, and the region to sign it for. Query params are the form
// fields beside Action and Version, json params are the body, verbatim.
func (r *awsReplay) request(ctx context.Context, entry AWSReadEntry) (*http.Request, string, error) {
	svc, _ := awsReaderService(entry.Service)
	endpoint, ok := r.endpoints[entry.Service]
	if !ok {
		return nil, "", errors.New("its service has no endpoint")
	}
	header := http.Header{}
	var body []byte
	switch svc.Protocol {
	case awsProtocolQuery:
		form, err := awsQueryForm(entry.Params)
		if err != nil {
			return nil, "", err
		}
		form.Add("Action", entry.Action)
		form.Add("Version", endpoint.version)
		body = []byte(form.Encode())
		header.Set("Content-Type", awsQueryMediaType)
	case awsProtocolJSON:
		body = entry.Params
		header.Set("Content-Type", awsJSONMediaType)
		header.Set("X-Amz-Target", svc.TargetPrefix+"."+entry.Action)
	default:
		return nil, "", fmt.Errorf("protocol %q cannot be replayed", svc.Protocol)
	}
	target, err := url.Parse(endpoint.url)
	if err != nil {
		return nil, "", fmt.Errorf("endpoint %q: %w", endpoint.url, err)
	}
	if target.Path == "" {
		target.Path = "/"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header = header
	if err := r.walls[entry.Service].check(req); err != nil {
		return nil, "", err
	}
	return req, endpoint.signingRegion, nil
}

// awsQueryForm is query params, one JSON object of strings, as form
// fields. It reads them key by key, because a map would keep one of two
// repeated keys and drop the other. Absent or null params are refused, as
// for json: an entry with no params carries {}.
func awsQueryForm(params json.RawMessage) (url.Values, error) {
	form := url.Values{}
	dec := json.NewDecoder(bytes.NewReader(params))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errors.New("query params must be a JSON object of strings")
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("query params: %w", err)
		}
		key, _ := tok.(string)
		var raw json.RawMessage
		var value string
		if err := dec.Decode(&raw); err != nil || len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil {
			return nil, fmt.Errorf("query param %q must be a string", key)
		}
		if form.Has(key) {
			return nil, fmt.Errorf("query params repeat %q", key)
		}
		form.Set(key, value)
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("query params: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("query params carry data after the object")
	}
	return form, nil
}

// checkAWSReadAnswer passes a 2xx answer, and a 4xx answer whose error
// code is one of svc's NotFound codes.
func checkAWSReadAnswer(svc AWSReaderService, status int, body []byte) error {
	if awsAnswerOK(status) {
		return nil
	}
	code := awsAnswerErrorCode(svc.Protocol, body)
	if status >= http.StatusBadRequest && status < http.StatusInternalServerError && slices.Contains(svc.NotFound, code) {
		return nil
	}
	return fmt.Errorf("answered %d %q", status, code)
}

func awsAnswerOK(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices
}

// awsAnswerErrorCode is an error answer's code, "" when it has none: the
// first XML Code element (EC2's Errors/Error/Code, IAM's and STS's
// Error/Code), or a json body's __type, else its code, without any prefix
// up to a '#' or suffix from a ':'. It reads no header; a code it cannot
// read fails the read.
func awsAnswerErrorCode(protocol string, body []byte) string {
	if protocol == awsProtocolQuery {
		fields, err := awsXMLFields(body, "Code")
		if err != nil || len(fields["Code"]) == 0 {
			return ""
		}
		return strings.TrimSpace(fields["Code"][0])
	}
	var answer struct {
		Type string `json:"__type"`
		Code string `json:"code"`
	}
	if json.Unmarshal(body, &answer) != nil {
		return ""
	}
	code := cmp.Or(answer.Type, answer.Code)
	if i := strings.LastIndex(code, "#"); i >= 0 {
		code = code[i+1:]
	}
	code, _, _ = strings.Cut(code, ":")
	return code
}

// awsXMLFields is the text of every element in body named one of names,
// at any depth.
func awsXMLFields(body []byte, names ...string) (map[string][]string, error) {
	fields := map[string][]string{}
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return fields, nil
		}
		if err != nil {
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok || !slices.Contains(names, start.Name.Local) {
			continue
		}
		var text string
		if err := dec.DecodeElement(&text, &start); err != nil {
			return nil, err
		}
		fields[start.Name.Local] = append(fields[start.Name.Local], text)
	}
}

// awsReadEntryName names a read-set entry in an error.
func awsReadEntryName(i int, entry AWSReadEntry) string {
	return fmt.Sprintf("entry %d (service %q, protocol %q, action %q)", i, entry.Service, entry.Protocol, entry.Action)
}
