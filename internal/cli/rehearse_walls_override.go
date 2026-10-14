package cli

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// rehearsalModelledEndpoints maps each provider endpoints key the
// rehearsal's fakeaws serves to its awsFakeawsPaths service id: the
// services rehearsalWallsEnv sends to fakeaws. Every other key, s3
// included, goes to rehearsalDeadProxy.
var rehearsalModelledEndpoints = map[string]string{
	"dynamodb":       "DYNAMODB",
	"ec2":            "EC2",
	"eks":            "EKS",
	"iam":            "IAM",
	"kms":            "KMS",
	"rds":            "RDS",
	"route53":        "ROUTE_53",
	"secretsmanager": "SECRETS_MANAGER",
	"sqs":            "SQS",
	"ssm":            "SSM",
	"sts":            "STS",
}

// rehearsalEndpointKeys is the sorted attribute names of the aws
// provider's endpoints block in schemaJSON, the output of `tofu providers
// schema -json` (ADR-0042 § Provider override). The provider is the one
// entry <host>/hashicorp/aws for a host in rehearsalLockHosts. No such
// entry or two, no endpoints block, or a key set without sts is refused.
func rehearsalEndpointKeys(schemaJSON []byte) ([]string, error) {
	var schema struct {
		ProviderSchemas map[string]struct {
			Provider struct {
				Block struct {
					BlockTypes map[string]struct {
						Block struct {
							Attributes map[string]json.RawMessage `json:"attributes"`
						} `json:"block"`
					} `json:"block_types"`
				} `json:"block"`
			} `json:"provider"`
		} `json:"provider_schemas"`
	}
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return nil, fmt.Errorf("the provider schema does not parse: %w", err)
	}
	var addresses []string
	for _, host := range slices.Sorted(maps.Keys(rehearsalLockHosts)) {
		if _, ok := schema.ProviderSchemas[host+"/hashicorp/aws"]; ok {
			addresses = append(addresses, host+"/hashicorp/aws")
		}
	}
	if len(addresses) != 1 {
		return nil, fmt.Errorf("the provider schema names %d hashicorp/aws providers (%s), and the endpoints keys come from exactly one",
			len(addresses), strings.Join(addresses, ", "))
	}
	endpoints, ok := schema.ProviderSchemas[addresses[0]].Provider.Block.BlockTypes["endpoints"]
	if !ok {
		return nil, fmt.Errorf("the %s schema has no endpoints block", addresses[0])
	}
	keys := slices.Sorted(maps.Keys(endpoints.Block.Attributes))
	if !slices.Contains(keys, "sts") {
		return nil, fmt.Errorf("the %s endpoints block names no sts key, so the override cannot keep STS off AWS", addresses[0])
	}
	return keys, nil
}

// rehearsalProviderOverride is the provider override's exact bytes: one
// provider "aws" block for the default provider, then one per alias,
// sorted ("" in aliases names the default). Each holds the fake keys,
// skip_metadata_api_check and an endpoints block naming every key: a
// modelled one at fakeawsURL plus its path, every other one, and every
// one when fakeawsURL is empty, at rehearsalDeadProxy.
func rehearsalProviderOverride(fakeawsURL, region string, keys, aliases []string) []byte {
	fakeaws := strings.TrimRight(strings.TrimSpace(fakeawsURL), "/")
	file := hclwrite.NewEmptyFile()
	for i, alias := range slices.Compact(slices.Sorted(slices.Values(append([]string{""}, aliases...)))) {
		if i > 0 {
			file.Body().AppendNewline()
		}
		provider := file.Body().AppendNewBlock("provider", []string{"aws"}).Body()
		if alias != "" {
			provider.SetAttributeValue("alias", cty.StringVal(alias))
		}
		provider.SetAttributeValue("access_key", cty.StringVal(rehearsalFakeAccessKeyID))
		provider.SetAttributeValue("secret_key", cty.StringVal(rehearsalFakeSecretAccessKey))
		provider.SetAttributeValue("skip_metadata_api_check", cty.True)
		endpoints := provider.AppendNewBlock("endpoints", nil).Body()
		for _, key := range slices.Sorted(slices.Values(keys)) {
			endpoint := rehearsalDeadProxy
			if svc, ok := rehearsalModelledEndpoints[key]; ok && fakeaws != "" {
				endpoint = fakeaws + strings.ReplaceAll(awsFakeawsPaths[svc], "{region}", region)
			}
			endpoints.SetAttributeValue(key, cty.StringVal(endpoint))
		}
	}
	return hclwrite.Format(file.Bytes())
}
