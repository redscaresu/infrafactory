package cli

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// layer3AWSPin is the one required_providers entry an AWS stack may declare.
// Its version is the one generation writes and the e2e mirror installs, so
// there is no second constant to drift from it.
var layer3AWSPin = layer3ProviderPin{
	name:    "aws",
	source:  "hashicorp/aws",
	version: harness.AWSProviderVersion,
}

// layer3AWSSafeProviderAttrs are the only attributes the provider "aws"
// block may set (ADR-0039 decision 3). Credentials, endpoints, profiles and
// aliases come from the sealed environment or not at all.
var layer3AWSSafeProviderAttrs = map[string]bool{
	"region":            true,
	"s3_use_path_style": true,
}

// awsProviderProblems is the region boundary's gate leg: the terraform {}
// allowlist, the pinned hashicorp/aws requirement, and exactly one
// provider "aws" in the configured region carrying only what
// buildAwsProviderBlock writes. region is aws.region as configured; empty
// refuses, and never defaults the way awsRegion does.
func awsProviderProblems(parsed map[string]*hclsyntax.Body, varDefaults map[string]cty.Value, region string) []string {
	problems := make([]string, 0)
	if region == "" {
		problems = append(problems, `aws.region is not configured, so the provider "aws" region cannot be checked; the gate never defaults it`)
	}
	sawPin := false
	var awsProviders []string
	for _, file := range slices.Sorted(maps.Keys(parsed)) {
		for _, block := range parsed[file].Blocks {
			switch block.Type {
			case "terraform":
				problems = append(problems, layer3TerraformBlockProblems(block, file)...)
				sourceProblems, saw := layer3ProviderSourceProblems(block, file, layer3AWSPin)
				problems = append(problems, sourceProblems...)
				problems = append(problems, awsRequiredProviderEntryProblems(block, file)...)
				sawPin = sawPin || saw
			case "provider":
				problems = append(problems, awsProviderBlockProblems(block, file, varDefaults, region)...)
				if slices.Equal(block.Labels, []string{layer3AWSPin.name}) {
					awsProviders = append(awsProviders, fmt.Sprintf("%s:%d", file, block.DefRange().Start.Line))
				}
				continue
			case "resource":
				if problem := awsProviderMetaArgProblem(block, file); problem != "" {
					problems = append(problems, problem)
				}
			}
			if problem := awsRunIDTagProblem(block, file); problem != "" {
				problems = append(problems, problem)
			}
		}
	}
	if problem := awsProviderCountProblem(awsProviders); problem != "" {
		problems = append(problems, problem)
	}
	if problem := layer3MissingProviderProblem(sawPin, layer3AWSPin); problem != "" {
		problems = append(problems, problem)
	}
	return problems
}

// awsRequiredProviderEntryProblems refuses what layer3ProviderSourceProblems
// admits beside the pin: an entry under another local name, and any key in
// the aws entry other than source and version. configuration_aliases is
// how a stack gets a second, aliased aws provider.
func awsRequiredProviderEntryProblems(tfBlock *hclsyntax.Block, file string) []string {
	problems := make([]string, 0)
	for _, inner := range tfBlock.Body.Blocks {
		if inner.Type != "required_providers" {
			continue
		}
		for name, attr := range inner.Body.Attributes {
			if name != layer3AWSPin.name {
				problems = append(problems, fmt.Sprintf("%s: required_provider %q is not permitted; an AWS stack declares only %q", file, name, layer3AWSPin.name))
				continue
			}
			obj, ok := attr.Expr.(*hclsyntax.ObjectConsExpr)
			if !ok {
				continue // layer3ProviderSourceProblems refuses a non-literal entry
			}
			for _, item := range obj.Items {
				if key := awsObjectKey(item); key != `"source"` && key != `"version"` {
					problems = append(problems, fmt.Sprintf("%s: required_provider %q sets %s; only source and version are permitted", file, name, key))
				}
			}
		}
	}
	return problems
}

// awsObjectKey is an object item's key, quoted, whether written bare or as
// a string; a computed key reads as one.
func awsObjectKey(item hclsyntax.ObjectConsItem) string {
	key, diags := item.KeyExpr.Value(nil)
	if diags.HasErrors() || !key.IsKnown() || key.IsNull() || key.Type() != cty.String {
		return "a computed key"
	}
	return fmt.Sprintf("%q", key.AsString())
}

// awsProviderBlockProblems checks one provider block: its label and
// attributes through layer3ProviderBlockProblems, then the values and the
// nested blocks that function does not look at.
func awsProviderBlockProblems(block *hclsyntax.Block, file string, varDefaults map[string]cty.Value, region string) []string {
	problems := layer3ProviderBlockProblems(block, file, layer3AWSPin.name, layer3AWSSafeProviderAttrs)
	if len(block.Labels) != 1 {
		return append(problems, fmt.Sprintf("%s: a provider block must be labelled exactly %q", file, layer3AWSPin.name))
	}
	if block.Labels[0] != layer3AWSPin.name {
		return problems
	}
	if problem := awsProviderRegionProblem(block, file, varDefaults, region); problem != "" {
		problems = append(problems, problem)
	}
	if problem := awsPathStyleProblem(block, file); problem != "" {
		problems = append(problems, problem)
	}
	return append(problems, awsProviderNestedBlockProblems(block, file)...)
}

// awsProviderRegionProblem requires region to resolve to exactly the
// configured region. The region is the gate leg of the region boundary: a
// stack cannot choose where it lands. An unconfigured region is refused
// once, by awsProviderProblems.
func awsProviderRegionProblem(block *hclsyntax.Block, file string, varDefaults map[string]cty.Value, region string) string {
	if region == "" {
		return ""
	}
	attr, ok := block.Body.Attributes["region"]
	if !ok {
		return fmt.Sprintf("%s: provider \"aws\" sets no region; it must be the configured aws.region %q", file, region)
	}
	val, ok := layer3ResolveConstant(attr.Expr, varDefaults)
	if !ok || val.Type() != cty.String {
		return fmt.Sprintf("%s: provider \"aws\" region is not a literal, or a variable with a literal default, so it cannot be checked against aws.region %q", file, region)
	}
	if val.AsString() != region {
		return fmt.Sprintf("%s: provider \"aws\" region %q is not the configured aws.region %q", file, val.AsString(), region)
	}
	return ""
}

// awsPathStyleProblem requires s3_use_path_style = true, the one setting
// buildAwsProviderBlock writes that has no environment form.
func awsPathStyleProblem(block *hclsyntax.Block, file string) string {
	if attr, ok := block.Body.Attributes["s3_use_path_style"]; ok {
		if val, diags := attr.Expr.Value(nil); !diags.HasErrors() && val.RawEquals(cty.True) {
			return ""
		}
	}
	return fmt.Sprintf("%s: provider \"aws\" s3_use_path_style must be literal true", file)
}

// awsProviderNestedBlockProblems admits at most one default_tags and no
// other nested block: endpoints would retarget the apply, assume_role
// would change who it runs as, and ignore_tags could hide the run-id tag.
func awsProviderNestedBlockProblems(block *hclsyntax.Block, file string) []string {
	problems := make([]string, 0)
	defaultTags := 0
	for _, nested := range block.Body.Blocks {
		if nested.Type != "default_tags" {
			problems = append(problems, fmt.Sprintf("%s: provider \"aws\" block %q is not permitted; the only nested block is infrafactory's default_tags", file, nested.Type))
			continue
		}
		defaultTags++
		problems = append(problems, awsDefaultTagsProblems(nested, file)...)
	}
	if defaultTags > 1 {
		problems = append(problems, fmt.Sprintf("%s: provider \"aws\" has %d default_tags blocks; at most one is permitted", file, defaultTags))
	}
	return problems
}

// awsDefaultTagsProblems requires default_tags to be what
// buildAwsProviderBlock writes: tags = { "infrafactory-run-id" = "<literal>" }
// and nothing else, so every resource the run applies carries its run id
// and no other tag rides along on all of them.
func awsDefaultTagsProblems(defaultTags *hclsyntax.Block, file string) []string {
	problems := make([]string, 0)
	for _, nested := range defaultTags.Body.Blocks {
		problems = append(problems, fmt.Sprintf("%s: provider \"aws\" default_tags block %q is not permitted", file, nested.Type))
	}
	for name := range defaultTags.Body.Attributes {
		if name != "tags" {
			problems = append(problems, fmt.Sprintf("%s: provider \"aws\" default_tags setting %q is not permitted", file, name))
		}
	}
	attr, ok := defaultTags.Body.Attributes["tags"]
	if !ok {
		return append(problems, fmt.Sprintf("%s: provider \"aws\" default_tags sets no tags; it carries exactly %q", file, awsRunIDTagKey))
	}
	obj, ok := attr.Expr.(*hclsyntax.ObjectConsExpr)
	if !ok || len(obj.Items) != 1 {
		problems = append(problems, fmt.Sprintf("%s: provider \"aws\" default_tags tags must be a literal object with exactly the key %q", file, awsRunIDTagKey))
	}
	if !ok {
		return problems
	}
	for _, item := range obj.Items {
		if key := awsObjectKey(item); key != fmt.Sprintf("%q", awsRunIDTagKey) {
			problems = append(problems, fmt.Sprintf("%s: provider \"aws\" default_tags sets tag %s; it carries only %q", file, key, awsRunIDTagKey))
			continue
		}
		if val, diags := item.ValueExpr.Value(nil); diags.HasErrors() || val.IsNull() || val.Type() != cty.String {
			problems = append(problems, fmt.Sprintf("%s: provider \"aws\" default_tags tag %q must be a literal string", file, awsRunIDTagKey))
		}
	}
	return problems
}

// awsProviderMetaArgProblem refuses `provider =` on a resource. The stack
// has one provider "aws" and no alias, so the meta-argument can only name
// a configuration this gate has not checked.
func awsProviderMetaArgProblem(block *hclsyntax.Block, file string) string {
	if _, ok := block.Body.Attributes["provider"]; !ok {
		return ""
	}
	return fmt.Sprintf("%s: %s sets provider; every resource uses the one provider \"aws\" block", file, awsBlockName(block))
}

// awsRunIDTagProblem refuses any mention of awsRunIDTagKey outside the
// provider block, as refuseAwsRunIDTag does on the generation path: a
// resource's own tag of that key overrides default_tags, so the stack
// would choose which run the resource belongs to. The provider block is
// skipped because awsDefaultTagsProblems checks the one place it belongs.
//
// It is mention-based, like refuseAwsRunIDTag, so a key assembled from
// parts ("${"infrafactory"}-run-id", join(), a local) passes. That is
// enough because tags report and never gate (HLD 2026-09-27): the sweep
// enumerates the whole scope, so a mistagged resource is misreported, not
// left behind. Closing it would mean evaluating every tags expression.
func awsRunIDTagProblem(block *hclsyntax.Block, file string) string {
	found := false
	hclsyntax.VisitAll(block, func(node hclsyntax.Node) hcl.Diagnostics {
		found = found || slices.ContainsFunc(awsNodeText(node), func(text string) bool {
			return strings.Contains(text, awsRunIDTagKey)
		})
		return nil
	})
	if !found {
		return ""
	}
	return fmt.Sprintf("%s: %s names the %q tag, which infrafactory sets from the run's id: remove it", file, awsBlockName(block), awsRunIDTagKey)
}

// awsNodeText is every piece of source text a node carries: names, labels,
// identifiers and literal strings. Comments are not nodes.
func awsNodeText(node hclsyntax.Node) []string {
	switch n := node.(type) {
	case *hclsyntax.Block:
		return append([]string{n.Type}, n.Labels...)
	case *hclsyntax.Attribute:
		return []string{n.Name}
	case *hclsyntax.FunctionCallExpr:
		return []string{n.Name}
	case *hclsyntax.ObjectConsKeyExpr:
		// VisitAll does not descend into a bare key such as
		// `infrafactory-run-id = "x"`; its text is only here.
		return []string{hcl.ExprAsKeyword(n.Wrapped)}
	case *hclsyntax.ForExpr:
		return []string{n.KeyVar, n.ValVar}
	case *hclsyntax.LiteralValueExpr:
		return awsStringText(n.Val)
	case *hclsyntax.ScopeTraversalExpr:
		return awsTraversalText(n.Traversal)
	case *hclsyntax.RelativeTraversalExpr:
		return awsTraversalText(n.Traversal)
	}
	return nil
}

func awsTraversalText(traversal hcl.Traversal) []string {
	var text []string
	for _, step := range traversal {
		switch s := step.(type) {
		case hcl.TraverseRoot:
			text = append(text, s.Name)
		case hcl.TraverseAttr:
			text = append(text, s.Name)
		case hcl.TraverseIndex:
			text = append(text, awsStringText(s.Key)...)
		}
	}
	return text
}

func awsStringText(val cty.Value) []string {
	if !val.IsKnown() || val.IsNull() || val.Type() != cty.String {
		return nil
	}
	return []string{val.AsString()}
}

// awsProviderCountProblem requires exactly one provider "aws" block across
// the stack. at lists each block's file:line.
func awsProviderCountProblem(at []string) string {
	switch len(at) {
	case 1:
		return ""
	case 0:
		return `no provider "aws" block; an AWS stack needs exactly one, the one infrafactory writes`
	}
	return fmt.Sprintf("%d provider \"aws\" blocks (%s); exactly one is permitted", len(at), strings.Join(at, ", "))
}

// awsBlockName renders a block as it is written: resource "aws_vpc" "main".
func awsBlockName(block *hclsyntax.Block) string {
	name := block.Type
	for _, label := range block.Labels {
		name += fmt.Sprintf(" %q", label)
	}
	return name
}
