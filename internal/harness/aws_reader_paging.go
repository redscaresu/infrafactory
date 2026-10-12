package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// The request fields a reader-table service's lists take the next page's
// token in (AWSReaderService.Paging).
const (
	// awsPagingNextToken is EC2's: the answer's <nextToken>, while it is
	// not empty.
	awsPagingNextToken = "NextToken"
	// awsPagingMarker is IAM's: the answer's <Marker>, while
	// <IsTruncated> is true.
	awsPagingMarker = "Marker"
)

// The bounds on reading one entry: sends of a throttled request, with a
// backoff from awsReadBackoff, doubling, between them, and pages of a list.
const (
	awsReadAttempts = 5
	awsReadBackoff  = time.Second
	awsReadMaxPages = 100
)

// pages reads entry to its last page, each page its own exchange, whose
// params carry the token that asked for it. awsPages' rule holds: a token
// seen before fails, so does a list past awsReadMaxPages, and so does any
// answer awsNextPageToken cannot read a next page from.
func (r *awsReplay) pages(ctx context.Context, entry AWSReadEntry) ([]AWSReadExchange, error) {
	svc, _ := awsReaderService(entry.Service)
	var exchanges []AWSReadExchange
	err := awsPages(func(token *string) (*string, error) {
		if len(exchanges) == awsReadMaxPages {
			return nil, fmt.Errorf("the list runs past %d pages", awsReadMaxPages)
		}
		page := entry
		if token != nil {
			var err error
			if page, err = awsPageEntry(entry, svc.Paging, *token); err != nil {
				return nil, err
			}
		}
		exchange, err := r.answer(ctx, svc, page)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", len(exchanges)+1, err)
		}
		next, err := awsNextPageToken(svc, exchange.Status, []byte(exchange.Body))
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", len(exchanges)+1, err)
		}
		exchanges = append(exchanges, exchange)
		return &next, nil
	})
	if err != nil {
		return nil, err
	}
	return exchanges, nil
}

// answer sends entry until it is answered with anything but one of svc's
// Throttling codes, waiting on the injected clock between sends, and gives
// up after awsReadAttempts. The wall counts every send in calls.
func (r *awsReplay) answer(ctx context.Context, svc AWSReaderService, entry AWSReadEntry) (AWSReadExchange, error) {
	backoff := awsReadBackoff
	for attempt := 1; ; attempt++ {
		exchange, err := r.replay(ctx, entry)
		if err != nil {
			return AWSReadExchange{}, err
		}
		body := []byte(exchange.Body)
		code := awsAnswerErrorCode(svc.Protocol, body)
		if awsAnswerOK(exchange.Status) || !slices.Contains(svc.Throttling, code) {
			return exchange, checkAWSReadAnswer(svc, exchange.Status, body)
		}
		if attempt == awsReadAttempts {
			return AWSReadExchange{}, fmt.Errorf("throttled: answered %d %q to each of %d attempts", exchange.Status, code, attempt)
		}
		if err := r.sleep(ctx, backoff); err != nil {
			return AWSReadExchange{}, err
		}
		backoff *= 2
	}
}

// awsPageEntry is entry asking for the page token names: its params with
// field set to token.
func awsPageEntry(entry AWSReadEntry, field, token string) (AWSReadEntry, error) {
	form, err := awsQueryForm(entry.Params)
	if err != nil {
		return AWSReadEntry{}, err
	}
	form.Set(field, token)
	params := make(map[string]string, len(form))
	for key := range form {
		params[key] = form.Get(key)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return AWSReadEntry{}, err
	}
	entry.Params = raw
	return entry, nil
}

// awsNextPageToken is the token that asks for the page after this
// answer's, "" when it is the last page or an error answer. A next-page
// marker svc does not page by fails (ADR-0040 rule 10): a json NextToken
// (no json service pages), a nextToken off EC2, IsTruncated off IAM. So
// do IsTruncated=true without exactly one Marker, an IsTruncated that is
// neither true nor false, and more than one nextToken.
func awsNextPageToken(svc AWSReaderService, status int, body []byte) (string, error) {
	if !awsAnswerOK(status) {
		return "", nil
	}
	if svc.Protocol == awsProtocolJSON {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			return "", fmt.Errorf("answered %d with a body that does not parse: %w", status, err)
		}
		if token, ok := fields["NextToken"]; ok && string(token) != "null" && string(token) != `""` {
			return "", fmt.Errorf("the answer is truncated: it carries a NextToken, and %s pages no list", svc.Service)
		}
		return "", nil
	}
	fields, err := awsXMLFields(body, "nextToken", "IsTruncated", "Marker")
	if err != nil {
		return "", fmt.Errorf("answered %d with a body that does not parse: %w", status, err)
	}
	tokens := awsNonEmpty(fields["nextToken"])
	truncated := false
	for _, v := range fields["IsTruncated"] {
		switch strings.TrimSpace(v) {
		case "true":
			truncated = true
		case "false":
		default:
			return "", fmt.Errorf("IsTruncated is %q, not true or false", v)
		}
	}
	switch {
	case len(tokens) > 0 && svc.Paging != awsPagingNextToken:
		return "", fmt.Errorf("the answer is truncated: it carries a nextToken, and %s does not page by one", svc.Service)
	case truncated && svc.Paging != awsPagingMarker:
		return "", fmt.Errorf("the answer is truncated: IsTruncated is true, and %s does not page by a Marker", svc.Service)
	case len(tokens) > 1:
		return "", fmt.Errorf("the answer carries %d nextTokens, not one", len(tokens))
	case len(tokens) == 1:
		return tokens[0], nil
	case !truncated:
		return "", nil
	}
	markers := awsNonEmpty(fields["Marker"])
	if len(markers) != 1 {
		return "", fmt.Errorf("IsTruncated is true with %d Markers, not one", len(markers))
	}
	return markers[0], nil
}

// awsNonEmpty is values without the blank ones.
func awsNonEmpty(values []string) []string {
	return slices.DeleteFunc(slices.Clone(values), func(v string) bool { return strings.TrimSpace(v) == "" })
}
