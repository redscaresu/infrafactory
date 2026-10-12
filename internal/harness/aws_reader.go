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
	"net/http"
	"net/url"
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
	// Endpoints replaces a service's endpoint URL, by service. Tests only.
	Endpoints map[string]string
	// Now is the clock for the timestamp, the duration and signing; nil
	// means time.Now.
	Now func() time.Time
}

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
// sent unless the read set is valid, and none after a failing identity
// check or preflight. A NotFound-class answer named in the reader table
// is recorded; any other non-2xx answer, and any answer carrying a
// truncation marker, fails the read, naming the entry.
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
		exchange, err := r.replay(ctx, entry)
		if err != nil {
			return AWSRecordedRead{}, fmt.Errorf("aws read: %s: %w", awsReadEntryName(i, entry), err)
		}
		exchanges = append(exchanges, exchange)
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
	}
	if err := ValidateAWSReadSet(in.ReadSet); err != nil {
		return nil, err
	}
	for i, entry := range in.ReadSet {
		if err := checkAWSReadParams(entry); err != nil {
			return nil, fmt.Errorf("%s: %w", awsReadEntryName(i, entry), err)
		}
	}
	r := &awsReplay{
		walls: map[string]*AWSReadWall{}, endpoints: awsReadEndpoints(in.Config.Region),
		signer: v4.NewSigner(), now: in.Now,
	}
	if r.now == nil {
		r.now = time.Now
	}
	for service, u := range in.Endpoints {
		endpoint, ok := r.endpoints[service]
		if !ok {
			return nil, fmt.Errorf("an endpoint is given for %q, which is not a reader-table service", service)
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

// replay sends entry and returns its exchange.
func (r *awsReplay) replay(ctx context.Context, entry AWSReadEntry) (AWSReadExchange, error) {
	svc, _ := awsReaderService(entry.Service)
	endpoint, ok := r.endpoints[entry.Service]
	if !ok {
		return AWSReadExchange{}, errors.New("its service has no endpoint")
	}
	ctx, cancel := context.WithTimeout(ctx, awsReadCallTimeout)
	defer cancel()
	req, err := r.request(ctx, svc, endpoint, entry)
	if err != nil {
		return AWSReadExchange{}, err
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
	if err := checkAWSReadAnswer(svc, resp.StatusCode, body); err != nil {
		return AWSReadExchange{}, err
	}
	return AWSReadExchange{AWSReadEntry: entry, Status: resp.StatusCode, Body: string(body)}, nil
}

// request builds entry's signed wire request: query params are the form
// fields beside Action and Version, json params are the body, verbatim.
func (r *awsReplay) request(ctx context.Context, svc AWSReaderService, endpoint awsReadEndpoint, entry AWSReadEntry) (*http.Request, error) {
	header := http.Header{}
	var body []byte
	switch svc.Protocol {
	case awsProtocolQuery:
		form, err := awsQueryForm(entry.Params)
		if err != nil {
			return nil, err
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
		return nil, fmt.Errorf("protocol %q cannot be replayed", svc.Protocol)
	}
	target, err := url.Parse(endpoint.url)
	if err != nil {
		return nil, fmt.Errorf("endpoint %q: %w", endpoint.url, err)
	}
	if target.Path == "" {
		target.Path = "/"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = header
	hash := sha256.Sum256(body)
	if err := r.signer.SignHTTP(ctx, r.creds, req, hex.EncodeToString(hash[:]), svc.Service, endpoint.signingRegion, r.now()); err != nil {
		return nil, fmt.Errorf("signing: %w", err)
	}
	return req, nil
}

// checkAWSReadParams refuses params that are not the shape ADR-0041 gives
// the entry's protocol: for query an object of strings with no repeated
// key and no Action or Version, for json an object.
func checkAWSReadParams(entry AWSReadEntry) error {
	if entry.Protocol == awsProtocolQuery {
		_, err := awsQueryForm(entry.Params)
		return err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(entry.Params, &fields) != nil || fields == nil {
		return errors.New("json params must be a JSON object")
	}
	return nil
}

// awsQueryForm is query params as form fields. It reads them key by key,
// because a map would keep one of two repeated keys and drop the other.
// Absent or null params are no fields.
func awsQueryForm(params json.RawMessage) (url.Values, error) {
	form := url.Values{}
	if len(params) == 0 || string(params) == "null" {
		return form, nil
	}
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
		if strings.EqualFold(key, "Action") || strings.EqualFold(key, "Version") {
			return nil, fmt.Errorf("query params carry %q; the replay sets Action and Version", key)
		}
		form.Set(key, value)
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("query params: %w", err)
	}
	return form, nil
}

// checkAWSReadAnswer passes a 2xx answer with no truncation marker, and a
// 4xx answer whose error code is one of svc's NotFound codes.
func checkAWSReadAnswer(svc AWSReaderService, status int, body []byte) error {
	if status < 200 || status > 299 {
		code := awsAnswerErrorCode(svc.Protocol, body)
		if status >= 400 && status < 500 && slices.Contains(svc.NotFound, code) {
			return nil
		}
		return fmt.Errorf("answered %d %q", status, code)
	}
	truncated, err := awsAnswerTruncated(svc.Protocol, body)
	if err != nil {
		return fmt.Errorf("answered %d with a body that does not parse: %w", status, err)
	}
	// ponytail: a next page fails the read until read-paging pages instead.
	if truncated {
		return errors.New("the answer is truncated (it carries a next-page marker), and paging is not built yet")
	}
	return nil
}

// awsAnswerErrorCode is an error answer's code, "" when it has none: the
// XML Code element (EC2's Errors/Error/Code, IAM's and STS's Error/Code),
// or a json body's __type or code, as smithy reads it.
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

// awsAnswerTruncated reports a next-page marker: a non-empty nextToken or
// IsTruncated=true in XML, a non-empty NextToken in json.
func awsAnswerTruncated(protocol string, body []byte) (bool, error) {
	if protocol == awsProtocolQuery {
		fields, err := awsXMLFields(body, "nextToken", "IsTruncated")
		if err != nil {
			return false, err
		}
		for _, token := range fields["nextToken"] {
			if strings.TrimSpace(token) != "" {
				return true, nil
			}
		}
		return slices.ContainsFunc(fields["IsTruncated"], func(v string) bool {
			return strings.EqualFold(strings.TrimSpace(v), "true")
		}), nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return false, err
	}
	token, ok := fields["NextToken"]
	return ok && string(token) != "null" && string(token) != `""`, nil
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
