package harness

import (
	"context"
	"encoding/xml"
	"fmt"
)

// readIAM reads, for each role a replayed GetRole answered with, in
// order of first mention, its policies and instance profiles
// (readIAMRole). Each request is an ADR-0041 entry sent through r.pages,
// so it is walled, signed for us-east-1 to iam.amazonaws.com, paged by
// IsTruncated/Marker, throttled and recorded as a replayed entry is. A
// replay that read no role adds nothing.
func (r *awsReplay) readIAM(ctx context.Context, replayed []AWSReadExchange) ([]AWSReadExchange, error) {
	var roles []string
	for _, exchange := range replayed {
		if exchange.Service != "iam" || exchange.Action != "GetRole" || !awsAnswerOK(exchange.Status) {
			continue
		}
		var answer iamRoleAnswer
		if err := xml.Unmarshal([]byte(exchange.Body), &answer); err != nil {
			return nil, fmt.Errorf("finding the role GetRole %s read: the answer does not parse: %w", exchange.Params, err)
		}
		roles = awsAppendNew(roles, answer.RoleName)
	}
	var out []AWSReadExchange
	for _, role := range roles {
		exchanges, err := r.readIAMRole(ctx, role)
		if err != nil {
			return nil, fmt.Errorf("role %q: %w", role, err)
		}
		out = append(out, exchanges...)
	}
	return out, nil
}

// readIAMRole reads role's inline policy names and each policy by name,
// then its attached policies and its instance profiles: the calls
// terraform-provider-aws makes to read an aws_iam_role and its policies.
func (r *awsReplay) readIAMRole(ctx context.Context, role string) ([]AWSReadExchange, error) {
	var out []AWSReadExchange
	read := func(action string, params map[string]string) ([]AWSReadExchange, error) {
		pages, err := r.followUp(ctx, "iam", action, params)
		out = append(out, pages...)
		return pages, err
	}
	ofRole := map[string]string{"RoleName": role}

	inline, err := read("ListRolePolicies", ofRole)
	if err != nil {
		return out, err
	}
	names, err := awsPageValues(inline, func(a iamRolePoliciesAnswer) []string { return a.Names })
	if err != nil {
		return out, err
	}
	for _, name := range names {
		if _, err := read("GetRolePolicy", map[string]string{"RoleName": role, "PolicyName": name}); err != nil {
			return out, err
		}
	}
	for _, action := range []string{"ListAttachedRolePolicies", "ListInstanceProfilesForRole"} {
		if _, err := read(action, ofRole); err != nil {
			return out, err
		}
	}
	return out, nil
}

// The parts of IAM answers a follow-up is read by, each by its path from
// the answer's root.
type iamRoleAnswer struct {
	RoleName string `xml:"GetRoleResult>Role>RoleName"`
}

type iamRolePoliciesAnswer struct {
	Names []string `xml:"ListRolePoliciesResult>PolicyNames>member"`
}
