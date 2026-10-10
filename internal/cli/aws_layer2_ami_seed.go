package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// awsSeededAMIName is the name the resolved AMI is seeded under. fakeaws
// requires one; nothing reads it, since the gate admits an AMI only as a
// literal id.
const awsSeededAMIName = "al2023-ami"

// newMockDeployHarness is the Layer 2 deploy, seeding the run's resolved
// AMI after each reset or restore (seedAWSLayer2AMI).
func newMockDeployHarness(runtime *CommandRuntime, mock harness.MockStateClient) *harness.MockDeployHarness {
	deploy := harness.NewMockDeployHarness(execCommandRunner{}, mock)
	deploy.Seed = runtime.seedAWSLayer2AMI
	return deploy
}

// seedAWSLayer2AMI registers the AMI an aws Layer 3 run resolved with
// fakeaws, so the Layer 2 apply runs the HCL the gate admitted: it names
// that id as a literal, and fakeaws refuses an image it has not seeded.
// Rewriting the id to fakeaws's fixture instead would apply other HCL
// than the real apply gets. The mock deploy calls this after every reset
// or restore, since both drop the seed. The region is the Layer 2 env's;
// the account is fakeaws's only one, which that env's key reaches.
// Layer 3 off, or another cloud, has no resolved id and seeds nothing.
func (runtime *CommandRuntime) seedAWSLayer2AMI(ctx context.Context) error {
	if runtime.AWSLayer3AMI == "" || runtime.loadedScenario == nil || layer3TeardownCloud(runtime.loadedScenario.Cloud) != layer3AWS {
		return nil
	}
	action := "seed fakeaws image " + runtime.AWSLayer3AMI
	url := strings.TrimSpace(runtime.Config.Fakeaws.URL)
	if url == "" {
		return fmt.Errorf("%s: fakeaws.url is not set", action)
	}
	body, err := json.Marshal(map[string]string{
		"ami_id":           runtime.AWSLayer3AMI,
		"name":             awsSeededAMIName,
		"root_device_name": runtime.AWSLayer3AMIRoot.DeviceName,
		"region":           awsRegion(runtime.Config.AWS),
	})
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return newMockStateClient(url).post(ctx, "/mock/images", action, body)
}
