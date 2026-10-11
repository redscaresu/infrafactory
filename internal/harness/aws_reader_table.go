package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// The wire protocols of the reader table's services.
const (
	awsProtocolQuery = "query"
	awsProtocolJSON  = "json"
)

// AWSReaderService is one service the account read may call: its wire
// protocol and the actions it may send. NotFound holds the error codes
// that mean the thing asked for does not exist: an answer, not a
// failure. TargetPrefix is a json service's X-Amz-Target prefix: the
// target is TargetPrefix + "." + action.
type AWSReaderService struct {
	Service      string
	Protocol     string
	TargetPrefix string
	Actions      []string
	NotFound     []string
}

// AWSReaderTable is the reader table: every action the account read
// (`rehearse read`) sends. It is the one source: the wall admits only its
// rows (and, with DryRun=true, the DryRun table's), and the reader policy
// and docs derive from it. It is not AWSSweepCollections and adds no
// sweep row.
var AWSReaderTable = []AWSReaderService{
	{
		Service:  "ec2",
		Protocol: awsProtocolQuery,
		Actions: []string{
			"DescribeAddresses", "DescribeAvailabilityZones", "DescribeImages",
			"DescribeInstanceAttribute", "DescribeInstances", "DescribeInternetGateways",
			"DescribeKeyPairs", "DescribeNetworkInterfaces", "DescribeRouteTables",
			"DescribeSecurityGroupRules", "DescribeSecurityGroups", "DescribeSubnets",
			"DescribeVpcAttribute", "DescribeVpcs",
		},
		NotFound: []string{
			"InvalidAddress.NotFound", "InvalidAllocationID.NotFound", "InvalidAMIID.NotFound",
			"InvalidGroup.NotFound", "InvalidInstanceID.NotFound", "InvalidInternetGatewayID.NotFound",
			"InvalidKeyPair.NotFound", "InvalidNetworkInterfaceID.NotFound", "InvalidRouteTableID.NotFound",
			"InvalidSecurityGroupRuleId.NotFound", "InvalidSubnetID.NotFound", "InvalidVpcID.NotFound",
		},
	},
	{
		Service:  "iam",
		Protocol: awsProtocolQuery,
		Actions: []string{
			"GetRole", "GetRolePolicy", "ListAttachedRolePolicies",
			"ListInstanceProfilesForRole", "ListRolePolicies",
		},
		NotFound: []string{"NoSuchEntity"},
	},
	{
		Service:      "ssm",
		Protocol:     awsProtocolJSON,
		TargetPrefix: "AmazonSSM",
		Actions:      []string{"GetParameter"},
		NotFound:     []string{"ParameterNotFound", "ParameterVersionNotFound"},
	},
	{
		Service:  "sts",
		Protocol: awsProtocolQuery,
		Actions:  []string{"GetCallerIdentity"},
	},
}

// AWSReaderDryRunActions are the EC2 actions the preflight sends with
// DryRun=true, to prove the read key cannot send them: every action the
// reap sends that is not a Describe. They are derived from AWSReapSteps,
// never listed by hand, and are never reader-table rows, so the reader
// policy never grants them.
var AWSReaderDryRunActions = awsReapWriteActions()

// awsDryRunService is the service the DryRun table's actions belong to.
const awsDryRunService = "ec2"

func awsReapWriteActions() []string {
	var actions []string
	for _, step := range AWSReapSteps {
		for _, action := range step.EC2Actions {
			if !strings.HasPrefix(action, "Describe") && !slices.Contains(actions, action) {
				actions = append(actions, action)
			}
		}
	}
	return actions
}

// awsReaderService returns the reader table's row for service.
func awsReaderService(service string) (AWSReaderService, bool) {
	i := slices.IndexFunc(AWSReaderTable, func(s AWSReaderService) bool { return s.Service == service })
	if i < 0 {
		return AWSReaderService{}, false
	}
	return AWSReaderTable[i], true
}

// awsReaderTableHas reports whether entry's service, protocol and action
// are a reader-table row.
func awsReaderTableHas(entry AWSReadEntry) bool {
	svc, ok := awsReaderService(entry.Service)
	return ok && svc.Protocol == entry.Protocol && slices.Contains(svc.Actions, entry.Action)
}

// AWSReadEntry is an ADR-0041 read-set entry: one request. Params is the
// form fields but Action and Version for query, the body's JSON object
// verbatim for json, and {path, query} for rest.
type AWSReadEntry struct {
	Service  string          `json:"service"`
	Protocol string          `json:"protocol"`
	Action   string          `json:"action"`
	Params   json.RawMessage `json:"params"`
}

// AWSReadExchange is an ADR-0041 exchange: an entry and its answer.
// Body is the raw response bytes.
type AWSReadExchange struct {
	AWSReadEntry
	Status int    `json:"status"`
	Body   string `json:"body"`
}

// AWSRecordedRead is the ADR-0041 recorded read. Calls counts every
// request sent, retries, identity and preflight included.
type AWSRecordedRead struct {
	Account    string            `json:"account"`
	Region     string            `json:"region"`
	Timestamp  time.Time         `json:"timestamp"`
	DurationMS int64             `json:"duration_ms"`
	Calls      int               `json:"calls"`
	Exchanges  []AWSReadExchange `json:"exchanges"`
}

// ValidateAWSReadSet refuses a read set holding any entry that is not a
// reader-table row, naming every such entry in one error.
func ValidateAWSReadSet(entries []AWSReadEntry) error {
	var errs []error
	for i, entry := range entries {
		if !awsReaderTableHas(entry) {
			errs = append(errs, fmt.Errorf("entry %d (service %q, protocol %q, action %q) is not in the reader table",
				i, entry.Service, entry.Protocol, entry.Action))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("aws read set: %w", errors.Join(errs...))
	}
	return nil
}
