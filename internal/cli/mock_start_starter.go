package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

	"github.com/redscaresu/infrafactory/internal/config"
)

type dockerMockStarter struct{}

func (s *dockerMockStarter) Start(ctx context.Context, cfg config.MockwayConfig) error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker binary not found in PATH: %w", err)
	}

	hostPort := "8080"
	if parsed, err := url.Parse(cfg.URL); err == nil {
		if port := parsed.Port(); port != "" {
			hostPort = port
		}
	}
	if _, err := strconv.Atoi(hostPort); err != nil {
		return fmt.Errorf("invalid mockway url port %q", hostPort)
	}

	cmd := exec.CommandContext(
		ctx,
		"docker",
		"run",
		"-d",
		"--rm",
		"--name",
		"infrafactory-mockway",
		"-p",
		hostPort+":8080",
		"ghcr.io/redscaresu/mockway",
	)

	var stderr bytes.Buffer
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("start mockway container: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return nil
}

func (s *dockerMockStarter) Stop(ctx context.Context, _ config.MockwayConfig) error {
	cmd := exec.CommandContext(ctx, "docker", "stop", "infrafactory-mockway")
	var stderr bytes.Buffer
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("stop mockway container: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (s *dockerMockStarter) Status(ctx context.Context, _ config.MockwayConfig) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "ps", "--filter", "name=infrafactory-mockway", "--format", "{{.Status}}")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("get mockway container status: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	status := strings.TrimSpace(stdout.String())
	if status == "" {
		return "stopped", nil
	}
	return status, nil
}

func (s *dockerMockStarter) Logs(ctx context.Context, _ config.MockwayConfig) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", "200", "infrafactory-mockway")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("read mockway container logs: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	logs := strings.TrimSpace(stdout.String())
	if logs == "" {
		return "no logs", nil
	}
	return logs, nil
}
