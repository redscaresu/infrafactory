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
