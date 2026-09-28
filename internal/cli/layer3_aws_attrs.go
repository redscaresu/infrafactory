package cli

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// awsRuleKind is what an allowlisted attribute's value must be.
type awsRuleKind int

const (
	// awsAny is free, but still under the cloud-neutral rules (the
	// pure-function allowlist, the index-without-try refusal).
	awsAny awsRuleKind = iota
	// awsDelegated is admitted here and checked by the user-data and AMI
	// leg of the gate (aws-gate-user-data-and-ami).
	awsDelegated
	awsEnum
	awsMax
	awsBool
	// awsRef is a bare <type>.<name>.id, and awsRefList a list of them.
	awsRef
	awsRefList
	// awsZone is a standard zone of the configured region.
	awsZone
	// awsBlock is a nested block with its own allowlist.
	awsBlock
)

// awsRule is one entry of awsAttrAllowlist: an attribute entry, which
// matches only an hclsyntax Attribute, or a nested-block entry (awsBlock),
// which matches only an hclsyntax Block. `ingress = [{...}]` is therefore
// an unlisted attribute: the provider accepts that attribute-as-blocks form
// (ConfigModeAttr), and this table's nested allowlist would never see it.
type awsRule struct {
	kind   awsRuleKind
	values []string           // awsEnum
	max    float64            // awsMax
	bools  []bool             // awsBool
	ref    string             // awsRef, awsRefList: the resource type referenced
	block  map[string]awsRule // awsBlock
	most   int                // awsBlock: how many a resource may hold; 0 is unbounded
}

var (
	awsFree      = awsRule{kind: awsAny}
	awsElsewhere = awsRule{kind: awsDelegated}
	awsEitherway = awsRule{kind: awsBool, bools: []bool{true, false}}
	awsInZone    = awsRule{kind: awsZone}
)

func awsOneOf(values ...string) awsRule { return awsRule{kind: awsEnum, values: values} }
func awsRefTo(resourceType string) awsRule {
	return awsRule{kind: awsRef, ref: resourceType}
}
func awsRefsTo(resourceType string) awsRule {
	return awsRule{kind: awsRefList, ref: resourceType}
}

// awsIngressRule is an ingress or egress block of aws_security_group. Which
// ports may be public is the Rego policy's decision, not this table's;
// prefix_list_ids and security_groups are absent because a prefix list is
// opaque and a group id names a group the run does not own.
var awsIngressRule = awsRule{kind: awsBlock, block: map[string]awsRule{
	"from_port":        awsFree,
	"to_port":          awsFree,
	"protocol":         awsFree,
	"cidr_blocks":      awsFree,
	"ipv6_cidr_blocks": awsFree,
	"description":      awsFree,
}}

// awsAttrAllowlist is every attribute and nested block an AWS Layer 3 stack
// may set, per resource type, in the layer3EnumBounds shape. Anything
// unlisted is refused, including a type with no entry: the AWS gate is an
// allowlist where the Scaleway gate grew as a denylist (HLD 2026-09-27,
// contradiction 14).
//
// The cost bounds are the HLD's: a t3.micro or t3.small, a root volume of
// at most 20 GB gp3 deleted with the instance, and no other volume. Every
// parent id is a typed reference to a resource in this stack, so nothing
// attaches to infrastructure the run does not own and will not destroy.
var awsAttrAllowlist = map[string]map[string]awsRule{
	"aws_instance": {
		"ami":                         awsElsewhere,
		"instance_type":               awsOneOf("t3.micro", "t3.small"),
		"subnet_id":                   awsRefTo("aws_subnet"),
		"vpc_security_group_ids":      awsRefsTo("aws_security_group"),
		"associate_public_ip_address": awsEitherway,
		"user_data":                   awsElsewhere,
		"tags":                        awsFree,
		// An omitted root_block_device is bounded against the real image by
		// awsAMIRootProblems.
		"root_block_device": {kind: awsBlock, most: 1, block: map[string]awsRule{
			"volume_size":           {kind: awsMax, max: 20},
			"volume_type":           awsOneOf("gp3"),
			"delete_on_termination": {kind: awsBool, bools: []bool{true}},
		}},
	},
	"aws_vpc": {
		"cidr_block":           awsFree,
		"enable_dns_support":   awsEitherway,
		"enable_dns_hostnames": awsEitherway,
		"instance_tenancy":     awsOneOf("default"),
		"tags":                 awsFree,
	},
	"aws_subnet": {
		"vpc_id":                  awsRefTo("aws_vpc"),
		"cidr_block":              awsFree,
		"availability_zone":       awsInZone,
		"map_public_ip_on_launch": awsEitherway,
		"tags":                    awsFree,
	},
	"aws_internet_gateway": {
		"vpc_id": awsRefTo("aws_vpc"),
		"tags":   awsFree,
	},
	"aws_route_table": {
		"vpc_id": awsRefTo("aws_vpc"),
		"tags":   awsFree,
	},
	"aws_route": {
		"route_table_id":         awsRefTo("aws_route_table"),
		"destination_cidr_block": awsFree,
		"gateway_id":             awsRefTo("aws_internet_gateway"),
	},
	"aws_route_table_association": {
		"subnet_id":      awsRefTo("aws_subnet"),
		"route_table_id": awsRefTo("aws_route_table"),
	},
	"aws_security_group": {
		"name":        awsFree,
		"description": awsFree,
		"vpc_id":      awsRefTo("aws_vpc"),
		"tags":        awsFree,
		"ingress":     awsIngressRule,
		"egress":      awsIngressRule,
	},
	"aws_eip": {
		"domain":   awsOneOf("vpc"),
		"instance": awsRefTo("aws_instance"),
		"tags":     awsFree,
	},
}

// awsMetaAttributes and awsMetaBlocks are the resource meta-arguments this
// table does not judge. depends_on only orders the apply. count, for_each
// and provider are refused by their own rules (layer3MultiplicityProblems,
// awsProviderProblems), and lifecycle by layer3NestedProblems.
var (
	awsMetaAttributes = map[string]bool{"depends_on": true, "count": true, "for_each": true, "provider": true}
	awsMetaBlocks     = map[string]bool{"lifecycle": true}
)

// awsResourceProblems checks one resource block against awsAttrAllowlist.
// Values are resolved through layer3ResolveConstant, and one that cannot
// be resolved is refused rather than assumed small.
func awsResourceProblems(resource *hclsyntax.Block, file string, varDefaults map[string]cty.Value, region string) []string {
	check := awsAttrCheck{file: file, name: "<unnamed>", varDefaults: varDefaults, region: region}
	if len(resource.Labels) > 0 {
		check.resourceType = resource.Labels[0]
	}
	if len(resource.Labels) > 1 {
		check.name = resource.Labels[1]
	}
	rules, ok := awsAttrAllowlist[check.resourceType]
	if !ok {
		return []string{fmt.Sprintf("%s: %s %s is a type the AWS gate has no attribute allowlist for, so nothing it sets could be checked",
			file, check.resourceType, check.name)}
	}
	return check.bodyProblems(resource.Body, rules, "")
}

// awsAttrCheck is one resource's check: who to name in a refusal, and what
// values resolve against.
type awsAttrCheck struct {
	file, resourceType, name string
	varDefaults              map[string]cty.Value
	region                   string
}

// bodyProblems checks a body against rules. path is "" for the resource
// itself and "<block>." inside a nested block.
func (c awsAttrCheck) bodyProblems(body *hclsyntax.Body, rules map[string]awsRule, path string) []string {
	problems := make([]string, 0)
	for attrName, attr := range body.Attributes {
		if path == "" && awsMetaAttributes[attrName] {
			continue
		}
		rule, ok := rules[attrName]
		if !ok || rule.kind == awsBlock {
			problems = append(problems, fmt.Sprintf("%s: %s %s sets %s%s, which the AWS gate does not admit as an attribute (anything not on its allowlist is refused)",
				c.file, c.resourceType, c.name, path, attrName))
			continue
		}
		if problem := c.valueProblem(attr.Expr, rule, path+attrName); problem != "" {
			problems = append(problems, problem)
		}
	}
	seen := make(map[string]int)
	for _, inner := range body.Blocks {
		if path == "" && awsMetaBlocks[inner.Type] {
			continue
		}
		rule, ok := rules[inner.Type]
		if !ok || rule.kind != awsBlock {
			problems = append(problems, fmt.Sprintf("%s: %s %s has nested block %s%s, which the AWS gate does not admit (anything not on its allowlist is refused)",
				c.file, c.resourceType, c.name, path, inner.Type))
			continue
		}
		seen[inner.Type]++
		if seen[inner.Type] == rule.most+1 && rule.most > 0 {
			problems = append(problems, fmt.Sprintf("%s: %s %s has more than %d %s%s block; the gate admits at most %d",
				c.file, c.resourceType, c.name, rule.most, path, inner.Type, rule.most))
		}
		problems = append(problems, c.bodyProblems(inner.Body, rule.block, path+inner.Type+".")...)
	}
	return problems
}

// valueProblem checks one allowlisted attribute's value against its rule.
func (c awsAttrCheck) valueProblem(expr hclsyntax.Expression, rule awsRule, attr string) string {
	switch rule.kind {
	case awsAny, awsDelegated:
		return ""
	case awsRef:
		if awsIsIDRef(expr, rule.ref) {
			return ""
		}
		return c.refProblem(attr, rule.ref+".<name>.id")
	case awsRefList:
		if awsIsIDRefList(expr, rule.ref) {
			return ""
		}
		return c.refProblem(attr, "a list of "+rule.ref+".<name>.id")
	}

	val, resolved := layer3ResolveConstant(expr, c.varDefaults)
	unresolved := fmt.Sprintf("%s: %s %s sets %s to something this check cannot resolve to a constant, so it cannot be bounded",
		c.file, c.resourceType, c.name, attr)
	switch rule.kind {
	case awsEnum:
		if !resolved || val.Type() != cty.String {
			return unresolved
		}
		if !slices.Contains(rule.values, val.AsString()) {
			return fmt.Sprintf("%s: %s %s sets %s to %q; the gate permits only %v", c.file, c.resourceType, c.name, attr, val.AsString(), rule.values)
		}
	case awsMax:
		if !resolved || val.Type() != cty.Number {
			return unresolved
		}
		if f, _ := val.AsBigFloat().Float64(); f > rule.max {
			return fmt.Sprintf("%s: %s %s sets %s to %g; the gate caps it at %g because it applies to real, billed infrastructure",
				c.file, c.resourceType, c.name, attr, f, rule.max)
		}
	case awsBool:
		if !resolved || val.Type() != cty.Bool {
			return unresolved
		}
		if !slices.Contains(rule.bools, val.True()) {
			return fmt.Sprintf("%s: %s %s sets %s to %t; the gate permits only %v", c.file, c.resourceType, c.name, attr, val.True(), rule.bools)
		}
	case awsZone:
		if !resolved || val.Type() != cty.String {
			return unresolved
		}
		return c.zoneProblem(attr, val.AsString())
	}
	return ""
}

func (c awsAttrCheck) refProblem(attr, want string) string {
	return fmt.Sprintf("%s: %s %s must set %s to %s, a resource in this stack; a literal id or any other expression names infrastructure the run does not own and will not destroy",
		c.file, c.resourceType, c.name, attr, want)
}

// zoneProblem admits only a standard zone of the configured region: the
// region and one letter. That refuses a Local Zone (us-east-1-bos-1a) and
// a Wavelength zone (us-east-1-wl1-bos-wlz-1), which bill differently and
// are opted into per account, and a zone id (use1-az1), which this check
// cannot place in a region.
func (c awsAttrCheck) zoneProblem(attr, zone string) string {
	if c.region == "" {
		return fmt.Sprintf("%s: %s %s sets %s, and no region is configured to check it against", c.file, c.resourceType, c.name, attr)
	}
	if regexp.MustCompile(`^` + regexp.QuoteMeta(c.region) + `[a-z]$`).MatchString(zone) {
		return ""
	}
	return fmt.Sprintf("%s: %s %s sets %s to %q; the gate admits only a standard zone of %s, such as %sa (not a Local Zone, a Wavelength zone or a zone id)",
		c.file, c.resourceType, c.name, attr, zone, c.region, c.region)
}

// awsIsIDRef reports whether expr IS `<resourceType>.<name>.id`: bare, with
// no index (count and for_each are refused, so there is nothing to index),
// and not merely mentioning one, as a ternary or a template would.
func awsIsIDRef(expr hclsyntax.Expression, resourceType string) bool {
	scope, ok := expr.(*hclsyntax.ScopeTraversalExpr)
	if !ok || len(scope.Traversal) != 3 || scope.Traversal.RootName() != resourceType {
		return false
	}
	_, named := scope.Traversal[1].(hcl.TraverseAttr)
	id, isAttr := scope.Traversal[2].(hcl.TraverseAttr)
	return named && isAttr && id.Name == "id"
}

// awsIsIDRefList reports whether expr is a list literal of awsIsIDRef.
func awsIsIDRefList(expr hclsyntax.Expression, resourceType string) bool {
	tuple, ok := expr.(*hclsyntax.TupleConsExpr)
	if !ok {
		return false
	}
	for _, element := range tuple.Exprs {
		if !awsIsIDRef(element, resourceType) {
			return false
		}
	}
	return true
}

// awsMultiplicityProblems bounds how many of a type the whole stack
// declares, across every file. Exactly one aws_instance: with none the gate
// would probe nothing and report green, and a second would be a server whose
// user_data and AMI the run did not choose. At most one aws_eip: with one
// instance's own public address, that bounds the stack to two addresses.
func awsMultiplicityProblems(parsed map[string]*hclsyntax.Body) []string {
	declared := make(map[string][]string)
	for _, file := range slices.Sorted(maps.Keys(parsed)) {
		for _, block := range parsed[file].Blocks {
			if block.Type != "resource" || len(block.Labels) == 0 {
				continue
			}
			name := "<unnamed>"
			if len(block.Labels) > 1 {
				name = block.Labels[1]
			}
			declared[block.Labels[0]] = append(declared[block.Labels[0]], file+": "+name)
		}
	}
	problems := make([]string, 0)
	if instances := declared["aws_instance"]; len(instances) == 0 {
		problems = append(problems, fmt.Sprintf("no aws_instance is declared in %s; an AWS stack declares exactly one, or the gate would verify nothing and still report green",
			strings.Join(slices.Sorted(maps.Keys(parsed)), ", ")))
	} else if len(instances) > 1 {
		problems = append(problems, fmt.Sprintf("aws_instance is declared %d times (%s); an AWS stack declares exactly one, the server whose user_data and AMI this run chose",
			len(instances), strings.Join(instances, ", ")))
	}
	if eips := declared["aws_eip"]; len(eips) > 1 {
		problems = append(problems, fmt.Sprintf("aws_eip is declared %d times (%s); the gate admits at most one, which bounds the stack to two public addresses",
			len(eips), strings.Join(eips, ", ")))
	}
	return problems
}
