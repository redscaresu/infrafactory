package cli

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// rehearsalAWSProviderTable names every attribute and nested block of
// provider "aws" in the hashicorp/aws harness.AWSProviderVersion schema,
// each with its verdict: true is admitted (ADR-0042 § provider "aws").
// TestRehearsalAWSProviderTableMatchesSchema fails when the schema and the
// table disagree. alias, a meta-argument the schema does not hold, is
// admitted beside it; any other name is refused under any lock version.
var rehearsalAWSProviderTable = map[string]bool{
	// Admitted: none names a host, a file, a credential or a role. region
	// is held to goal 11 besides.
	"region": true, "default_tags": true, "ignore_tags": true,
	"allowed_account_ids": true, "forbidden_account_ids": true,
	"max_retries": true, "retry_mode": true, "s3_use_path_style": true,
	"skip_credentials_validation": true, "skip_metadata_api_check": true, "skip_requesting_account_id": true,
	// Refused: they change where requests go or how they are sent.
	"endpoints": false, "http_proxy": false, "https_proxy": false, "no_proxy": false,
	"custom_ca_bundle": false, "insecure": false, "sts_region": false,
	"ec2_metadata_service_endpoint": false, "ec2_metadata_service_endpoint_mode": false,
	"use_fips_endpoint": false, "use_dualstack_endpoint": false,
	"s3_us_east_1_regional_endpoint": false, "skip_region_validation": false,
	"token_bucket_rate_limiter_capacity": false,
	// Refused: they choose the credential that signs requests.
	"access_key": false, "secret_key": false, "token": false, "profile": false,
	"shared_config_files": false, "shared_credentials_files": false,
	"assume_role": false, "assume_role_with_web_identity": false,
}

// awsProviderProblems holds a root provider "aws" block to the table and
// records its region for goal 11. A block with no region is refused.
func (m rehearsalModule) awsProviderProblems(file, at string, block *hclsyntax.Block) []string {
	addr := rehearsalAddress(block)
	var problems []string
	for name, attr := range block.Body.Attributes {
		problems = append(problems, rehearsalAWSArgProblems(rehearsalAt(file, attr.SrcRange, addr+"."+name), name)...)
	}
	for _, inner := range block.Body.Blocks {
		problems = append(problems, rehearsalAWSArgProblems(rehearsalAt(file, inner.DefRange(), addr+"."+inner.Type), inner.Type)...)
	}
	region, ok := block.Body.Attributes["region"]
	if !ok {
		return append(problems, at+`: refused: a provider "aws" block must set region, so goal 11 can check it`)
	}
	if _, aliased := block.Body.Attributes["alias"]; !aliased {
		m.regions.defaultProvider = true
	}
	m.regions.uses = append(m.regions.uses, rehearsalRegionUse{at: rehearsalAt(file, region.SrcRange, addr+".region"), expr: region.Expr, root: true})
	return problems
}

// rehearsalAWSArgProblems refuses a provider "aws" argument or block the
// table does not admit.
func rehearsalAWSArgProblems(at, name string) []string {
	admitted, named := rehearsalAWSProviderTable[name]
	switch {
	case name == "alias", admitted:
		return nil
	case named:
		var names []string
		for name, admitted := range rehearsalAWSProviderTable {
			if admitted {
				names = append(names, name)
			}
		}
		slices.Sort(names)
		return []string{fmt.Sprintf(`%s: refused: a provider "aws" block admits only alias, %s`, at, strings.Join(names, ", "))}
	}
	return []string{fmt.Sprintf(`%s: refused: the gate's table of hashicorp/aws %s provider arguments does not name it, so it is refused under any lock version`, at, harness.AWSProviderVersion)}
}

// recordAWSType records an aws_* block for goal 11: that it needs the
// root's unaliased provider "aws" block, and its region argument, if any.
func (m rehearsalModule) recordAWSType(file, at string, block *hclsyntax.Block) {
	named := rehearsalModuleDir{key: m.key}.named([]string{at})[0]
	if m.regions.awsUse == "" || named < m.regions.awsUse {
		m.regions.awsUse = named
	}
	if region, ok := block.Body.Attributes["region"]; ok {
		m.regions.uses = append(m.regions.uses, rehearsalRegionUse{
			at: rehearsalAt(file, region.SrcRange, rehearsalAddress(block)+".region"), expr: region.Expr, root: m.root, key: m.key,
		})
	}
}

// recordDefault records a root variable's default, the lowest of its
// sources.
func (m rehearsalModule) recordDefault(block *hclsyntax.Block) {
	if def, ok := block.Body.Attributes["default"]; ok && m.root && len(block.Labels) == 1 {
		m.regions.defaults[block.Labels[0]] = def
	}
}

// rehearsalRegions collects what goal 11 reads across one gate run: every
// region to resolve, the root's variable sources, and the first aws_*
// block, which needs an unaliased root provider "aws" block.
type rehearsalRegions struct {
	uses []rehearsalRegionUse
	// defaults are the root's variable defaults, by variable name.
	defaults hclsyntax.Attributes
	// tfvars are the root's .tfvars files, by display name.
	tfvars  map[string]*hclsyntax.Body
	varFile *hclsyntax.Body
	// awsUse is the first aws_* block's "file:line: address", in sort order.
	awsUse          string
	defaultProvider bool
}

func newRehearsalRegions() *rehearsalRegions {
	return &rehearsalRegions{defaults: hclsyntax.Attributes{}}
}

// rehearsalRegionUse is one region argument: where it is, its expression,
// and whether it is in the root, where var.x resolves.
type rehearsalRegionUse struct {
	at   string
	expr hclsyntax.Expression
	root bool
	key  string
}

// rehearsalVarSource is one source of the root's variable values.
type rehearsalVarSource struct {
	name  string
	attrs hclsyntax.Attributes
}

// problems is goal 11 (ADR-0042 § Goal 11, statically): every region
// resolves to --region, and aws_* types have a root provider "aws" block
// without an alias, since tofu's implied empty one has no region.
func (r *rehearsalRegions) problems(region string) []string {
	var problems []string
	if r.awsUse != "" && !r.defaultProvider {
		problems = append(problems, r.awsUse+`: refused: the root has no provider "aws" block without an alias, so tofu's implied empty one, which has no region, is the default`)
	}
	sources, sourceProblems := r.sources()
	problems = append(problems, sourceProblems...)
	for _, use := range r.uses {
		if problem := use.problem(sources, region); problem != "" {
			problems = append(problems, rehearsalModuleDir{key: use.key}.named([]string{problem})...)
		}
	}
	return problems
}

// sources are the root's variable sources in tofu's precedence, lowest
// first: the defaults, terraform.tfvars, each *.auto.tfvars in lexical
// order, then the var file (TestAssumption_VariablePrecedenceMatchesGate).
// Any other spelling of those names is refused: whether tofu loads it
// is not something the gate can say.
func (r *rehearsalRegions) sources() ([]rehearsalVarSource, []string) {
	sources := []rehearsalVarSource{{name: "its default", attrs: r.defaults}}
	var auto, problems []string
	for _, name := range slices.Sorted(maps.Keys(r.tfvars)) {
		base := filepath.Base(name)
		lower := strings.ToLower(base)
		switch {
		case base == "terraform.tfvars":
			sources = append(sources, rehearsalVarSource{name: name, attrs: r.tfvars[name].Attributes})
		case strings.HasSuffix(base, ".auto.tfvars"):
			auto = append(auto, name)
		case lower == "terraform.tfvars", strings.HasSuffix(lower, ".auto.tfvars"):
			problems = append(problems, name+": refused: tofu loads terraform.tfvars and *.auto.tfvars by name, and the gate cannot say whether it loads this spelling")
		}
	}
	for _, name := range auto {
		sources = append(sources, rehearsalVarSource{name: name, attrs: r.tfvars[name].Attributes})
	}
	if r.varFile != nil {
		sources = append(sources, rehearsalVarSource{name: "--var-file", attrs: r.varFile.Attributes})
	}
	return sources, problems
}

// problem is why one region is refused, or "".
func (u rehearsalRegionUse) problem(sources []rehearsalVarSource, region string) string {
	value, from, problem := u.resolve(sources)
	switch {
	case problem != "":
		return u.at + ": refused: " + problem
	case value != region:
		return fmt.Sprintf("%s: refused: region %q%s is not --region %q", u.at, value, from, region)
	}
	return ""
}

// resolve is a region's value and where a variable's came from: a string
// literal, or in the root var.x, resolved by tofu's precedence over
// sources. Anything else is a problem naming it.
func (u rehearsalRegionUse) resolve(sources []rehearsalVarSource) (string, string, string) {
	expr := u.expr
	if wrap, ok := expr.(*hclsyntax.TemplateWrapExpr); ok {
		expr = wrap.Wrapped
	}
	if value, ok := rehearsalLiteral(expr); ok {
		return value, "", ""
	}
	name, ok := rehearsalVarName(expr)
	switch {
	case !ok:
		return "", "", "region must be a string literal or var.<name>, so " + rehearsalRefNames(expr) + " is refused"
	case !u.root:
		return "", "", fmt.Sprintf("var.%s is in a module, and the gate resolves variables only in the root, so a module's region must be a string literal", name)
	}
	for i := len(sources) - 1; i >= 0; i-- {
		attr, ok := sources[i].attrs[name]
		if !ok {
			continue
		}
		value, ok := rehearsalLiteral(attr.Expr)
		if !ok {
			return "", "", fmt.Sprintf("var.%s takes its value from %s, which is not a string literal", name, sources[i].name)
		}
		return value, fmt.Sprintf(" (var.%s from %s)", name, sources[i].name), ""
	}
	return "", "", fmt.Sprintf("var.%s has no value: no default, terraform.tfvars, *.auto.tfvars or --var-file sets it", name)
}

// rehearsalVarName is x for an expression that is exactly var.x.
func rehearsalVarName(expr hclsyntax.Expression) (string, bool) {
	ref, ok := expr.(*hclsyntax.ScopeTraversalExpr)
	if !ok || len(ref.Traversal) != 2 || ref.Traversal.RootName() != "var" {
		return "", false
	}
	attr, ok := ref.Traversal[1].(hcl.TraverseAttr)
	return attr.Name, ok
}

// rehearsalRefNames names what an expression refers to, such as
// local.region, or calls it computed when it refers to nothing.
func rehearsalRefNames(expr hclsyntax.Expression) string {
	var names []string
	for _, traversal := range expr.Variables() {
		name := traversal.RootName()
		for _, step := range traversal[1:] {
			attr, ok := step.(hcl.TraverseAttr)
			if !ok {
				break
			}
			name += "." + attr.Name
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return "a computed value"
	}
	return strings.Join(names, ", ")
}
