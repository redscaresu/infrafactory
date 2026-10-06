package cli

import "fmt"

// layer3Cloud is the cloud a Layer 3 path acts on. Every entry point
// parses it from the string it holds -- the scenario's cloud, or a live
// record's -- and dispatches on the result, so a cloud with no Layer 3
// branch is refused instead of reaching code written for Scaleway.
type layer3Cloud string

const (
	layer3Scaleway layer3Cloud = "scaleway"
	layer3AWS      layer3Cloud = "aws"
)

// parseLayer3Cloud maps a cloud string onto a layer3Cloud.
//
// "" is Scaleway, and only because of where it can appear: the schema
// requires cloud, so an empty one comes from a Go fixture or from a live
// record written before Deployment.Cloud existed, when Scaleway was the
// only live cloud. Anything unlisted is an error, never a default.
func parseLayer3Cloud(raw string) (layer3Cloud, error) {
	switch raw {
	case "", "scaleway":
		return layer3Scaleway, nil
	case "aws":
		return layer3AWS, nil
	}
	return "", fmt.Errorf("cloud %q has no Layer 3 support, so nothing may run against its real API", raw)
}

// layer3PreflightHCLForCloud runs the cloud's own structural gate before
// any tofu starts. Scaleway reads only in.AllowedResourceTypes; AWS checks
// the stack against every field, and a zero field refuses. Any other cloud
// is refused rather than judged by rules that say nothing about it.
func layer3PreflightHCLForCloud(cloud layer3Cloud, outputDir string, in awsGateInputs) error {
	switch cloud {
	case layer3Scaleway:
		return layer3PreflightHCL(outputDir, in.AllowedResourceTypes)
	case layer3AWS:
		return validateAWSLayer3HCLShape(outputDir, in)
	}
	return fmt.Errorf("cloud %s has no Layer 3 HCL gate yet, so its configuration may not reach a real account", cloud)
}

// layer3HCLGate is layer3PreflightHCLForCloud unless a test replaced it
// through Deps.Layer3HCLGate.
func (runtime *CommandRuntime) layer3HCLGate(cloud layer3Cloud, outputDir string, in awsGateInputs) error {
	if runtime.Deps.Layer3HCLGate != nil {
		return runtime.Deps.Layer3HCLGate(cloud, outputDir, in)
	}
	return layer3PreflightHCLForCloud(cloud, outputDir, in)
}

// allowlistOnlyGateInputs is the gate input for a path aws never reaches
// (deploy, live upgrade: layer3LiveCloud refuses aws first), so only the
// Scaleway gate's allowlist is filled.
func allowlistOnlyGateInputs(runtime *CommandRuntime) awsGateInputs {
	return awsGateInputs{AllowedResourceTypes: runtime.Config.Validation.Layers.SandboxDeploy.AllowResourceTypes}
}

// layer3LiveCloud parses the cloud for a live path (--keep, deploy,
// live upgrade) and refuses every cloud but Scaleway. A live stack
// outlives the command, and only Scaleway has the record, teardown and
// reap that bound it; `run` and `test` destroy what they apply.
func layer3LiveCloud(raw, livePath string) (layer3Cloud, error) {
	cloud, err := parseLayer3Cloud(raw)
	if err != nil {
		return "", fmt.Errorf("%s: %w", livePath, err)
	}
	if cloud != layer3Scaleway {
		return "", fmt.Errorf(
			"%s is refused for cloud %s until its live path lands: nothing could tear down or reap what it left running. "+
				"Use `infrafactory run` or `infrafactory test`, which destroy what they apply", livePath, cloud)
	}
	return cloud, nil
}

// layer3TeardownCloud is the cloud a teardown path is keyed on. Unlike
// parseLayer3Cloud it cannot fail: an unlisted cloud keeps its name, and
// since every seam runs its body for Scaleway alone, the name changes
// only what the refusal says. A run or reap has to be able to say which
// cloud it will not tear down.
func layer3TeardownCloud(raw string) layer3Cloud {
	if cloud, err := parseLayer3Cloud(raw); err == nil {
		return cloud
	}
	return layer3Cloud(raw)
}

// layer3SeamRefused is what a Layer 3 seam returns for any cloud but
// Scaleway, before it calls anything. Every body behind these seams is
// Scaleway's; pointed at another cloud it would act on, or vouch for, an
// account it never looked at.
func layer3SeamRefused(cloud layer3Cloud, seam string) error {
	return fmt.Errorf("cloud %s has no Layer 3 %s, so it was not attempted", cloud, seam)
}

// layer3TeardownNotBuilt is what a run, reap or interrupt says instead of
// tearing down a cloud with no Layer 3 teardown. It names no command:
// reap refuses the same workdir, and Scaleway's advice does not apply.
func layer3TeardownNotBuilt(cloud layer3Cloud, statePath string) string {
	return fmt.Sprintf("%s Layer 3 teardown is not built; resources recorded in %s may exist: destroy them by hand",
		cloud, statePath)
}
