package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const goldenAWSUserData = `#!/bin/bash
# Rendered by infrafactory from the scenario's service: block.
set -euo pipefail
dnf install -y docker
systemctl enable --now docker
docker run -d --restart=always -p 80:80 'nginx:1.27'
`

const awsServiceScenarioYAML = `scenario: aws-web
cloud: aws
service:
  image: nginx
  tag: "1.27"
  port: 80
  ttl: 4h
`

var awsInstanceScenarioPath = filepath.Join("..", "..", "scenarios", "training", "aws-instance.yaml")

const awsInstanceHCL = `resource "aws_instance" "web" {
  ami           = "ami-0al2023x8664"
  instance_type = "t3.micro"
}
`

func TestRenderAWSUserDataGolden(t *testing.T) {
	got, err := renderAWSUserData(scenario.ServiceSpec{Image: "nginx", Tag: "1.27", Port: 80})
	require.NoError(t, err)
	assert.Equal(t, goldenAWSUserData, string(got))
}

func TestRenderAWSUserDataRefusesUnsafeImageAndTag(t *testing.T) {
	for _, tc := range []struct {
		name, image, tag, field string
	}{
		{name: "command in image", image: "nginx;curl x|sh", tag: "1.27", field: "service.image"},
		{name: "command in tag", image: "nginx", tag: "1.27 && id", field: "service.tag"},
		{name: "newline in image", image: "nginx\nid", tag: "1.27", field: "service.image"},
		{name: "trailing newline in image", image: "nginx\n", tag: "1.27", field: "service.image"},
		{name: "quote in tag", image: "nginx", tag: "1.27'", field: "service.tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := renderAWSUserData(scenario.ServiceSpec{Image: tc.image, Tag: tc.tag, Port: 80})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.field)
		})
	}
}

// userDataHarness runs generate against a fake generator that returns files
// and records the Request it was given.
type userDataHarness struct {
	rt        *CommandRuntime
	scenario  string
	outputDir string
	req       generator.Request
}

func newUserDataHarness(t *testing.T, scenarioPath string, files map[string][]byte) *userDataHarness {
	t.Helper()
	h := &userDataHarness{scenario: scenarioPath, outputDir: filepath.Join(t.TempDir(), "out")}
	h.rt = &CommandRuntime{
		Config:    config.Default(),
		outputDir: h.outputDir,
		Deps: RuntimeDependencies{Generator: generator.SeedGeneratorFunc(
			func(_ context.Context, req generator.Request) (*generator.GeneratedCode, error) {
				h.req = req
				copied := make(map[string][]byte, len(files))
				for name, content := range files {
					copied[name] = content
				}
				return &generator.GeneratedCode{Files: copied}, nil
			})},
	}
	return h
}

func (h *userDataHarness) generate(mode generatedFileWriteMode) error {
	_, _, err := generateAndWriteFilesWithResult(context.Background(), h.rt, h.scenario, 1, nil, mode)
	return err
}

func writeScenarioForTest(t *testing.T, payload string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scenario.yaml")
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
	return path
}

func TestGenerateWritesAWSUserDataForAServiceScenario(t *testing.T) {
	for _, mode := range []generatedFileWriteMode{generatedFileWriteModeClean, generatedFileWriteModeIncremental} {
		t.Run(string(mode), func(t *testing.T) {
			h := newUserDataHarness(t, writeScenarioForTest(t, awsServiceScenarioYAML), map[string][]byte{
				"main.tf": []byte(awsInstanceHCL),
			})
			scriptPath := filepath.Join(h.outputDir, generator.AWSUserDataFile)
			require.NoError(t, os.MkdirAll(h.outputDir, 0o755))
			require.NoError(t, os.WriteFile(scriptPath, []byte("stale from the previous iteration\n"), 0o644))

			require.NoError(t, h.generate(mode))

			assert.Equal(t, generator.AWSUserDataLine, h.req.UserDataLine)
			got, err := os.ReadFile(scriptPath)
			require.NoError(t, err)
			assert.Equal(t, goldenAWSUserData, string(got))
		})
	}
}

func TestGenerateRefusesAUserDataReferenceWithoutAService(t *testing.T) {
	h := newUserDataHarness(t, awsInstanceScenarioPath, map[string][]byte{
		"main.tf": []byte(awsInstanceHCL + `
resource "aws_instance" "boot" {
  ` + generator.AWSUserDataLine + `
}
`),
	})

	err := h.generate(generatedFileWriteModeClean)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "service: block")
	assert.Empty(t, h.req.UserDataLine)
}

func TestGenerateLeavesUserDataLineEmptyWithoutAService(t *testing.T) {
	h := newUserDataHarness(t, awsInstanceScenarioPath, map[string][]byte{
		"main.tf": []byte(awsInstanceHCL),
	})

	require.NoError(t, h.generate(generatedFileWriteModeClean))

	assert.Empty(t, h.req.UserDataLine)
	assert.NoFileExists(t, filepath.Join(h.outputDir, generator.AWSUserDataFile))
}

func TestGenerateRefusesAModelWrittenUserDataScript(t *testing.T) {
	for _, name := range []string{
		generator.AWSUserDataFile,
		"./" + generator.AWSUserDataFile,
		"sub/../" + generator.AWSUserDataFile,
	} {
		t.Run(name, func(t *testing.T) {
			h := newUserDataHarness(t, writeScenarioForTest(t, awsServiceScenarioYAML), map[string][]byte{
				"main.tf": []byte(awsInstanceHCL),
				name:      []byte("#!/bin/bash\npython3 -m http.server 80\n"),
			})

			err := h.generate(generatedFileWriteModeClean)

			require.Error(t, err)
			assert.Contains(t, err.Error(), name)
			assert.NoDirExists(t, h.outputDir, "nothing is written")
		})
	}
}

func TestGenerateIncrementalRefusesASymlinkAtTheUserDataPath(t *testing.T) {
	h := newUserDataHarness(t, writeScenarioForTest(t, awsServiceScenarioYAML), map[string][]byte{
		"main.tf": []byte(awsInstanceHCL),
	})
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.WriteFile(target, []byte("untouched\n"), 0o644))
	require.NoError(t, os.MkdirAll(h.outputDir, 0o755))
	require.NoError(t, os.Symlink(target, filepath.Join(h.outputDir, generator.AWSUserDataFile)))

	err := h.generate(generatedFileWriteModeIncremental)

	require.Error(t, err)
	assert.Contains(t, err.Error(), generator.AWSUserDataFile)
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "untouched\n", string(got))
}

func TestGenerateWritesNoUserDataForAScalewayService(t *testing.T) {
	h := newUserDataHarness(t, writeScenarioForTest(t, strings.Replace(awsServiceScenarioYAML, "cloud: aws", "cloud: scaleway", 1)), map[string][]byte{
		"main.tf": []byte(`resource "scaleway_instance_server" "web" {}` + "\n"),
	})

	require.NoError(t, h.generate(generatedFileWriteModeClean))

	assert.Empty(t, h.req.UserDataLine)
	assert.NoFileExists(t, filepath.Join(h.outputDir, generator.AWSUserDataFile))
}

func TestAWSUserDataIsTheSameWithLayer3OnAndOff(t *testing.T) {
	scriptFor := func(layer3 bool) string {
		h := newUserDataHarness(t, writeScenarioForTest(t, awsServiceScenarioYAML), map[string][]byte{
			"main.tf": []byte(awsInstanceHCL),
		})
		h.rt.Config.Validation.Layers.SandboxDeploy.Enabled = layer3
		// The Layer 3 gate runs after the write and may refuse AWS; only
		// the written script matters here.
		_ = h.generate(generatedFileWriteModeClean)
		got, err := os.ReadFile(filepath.Join(h.outputDir, generator.AWSUserDataFile))
		require.NoError(t, err)
		return string(got)
	}

	assert.Equal(t, goldenAWSUserData, scriptFor(false))
	assert.Equal(t, scriptFor(false), scriptFor(true))
}
