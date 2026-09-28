package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

// The image and tag land in a script that runs as root on first boot, so
// they are refused outside a conservative charset rather than escaped. The
// tag pattern is Docker's own tag grammar; the image pattern is narrower
// than Docker's (no registry port, no uppercase, no digest).
var (
	awsUserDataImageRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*$`)
	awsUserDataTagRe   = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
)

// renderAWSUserData renders the Amazon Linux 2023 boot script that runs the
// scenario's service. infrafactory writes it, not the model, so the script
// is the same bytes at every layer and cannot start anything undeclared.
func renderAWSUserData(svc scenario.ServiceSpec) ([]byte, error) {
	if !awsUserDataImageRe.MatchString(svc.Image) {
		return nil, fmt.Errorf("service.image %q is refused: the user-data script accepts only %s", svc.Image, awsUserDataImageRe)
	}
	if !awsUserDataTagRe.MatchString(svc.Tag) {
		return nil, fmt.Errorf("service.tag %q is refused: the user-data script accepts only %s", svc.Tag, awsUserDataTagRe)
	}
	if svc.Port < 1 || svc.Port > 65535 {
		return nil, fmt.Errorf("service.port %d is outside 1-65535", svc.Port)
	}
	return fmt.Appendf(nil, `#!/bin/bash
# Rendered by infrafactory from the scenario's service: block.
set -euo pipefail
dnf install -y docker
systemctl enable --now docker
docker run -d --restart=always -p %d:%d '%s'
`, svc.Port, svc.Port, svc.Ref()), nil
}

// placeAWSUserData refuses a model file at the script's path, then adds the
// rendered script, so the run store snapshots it with the HCL. Without a
// script, an AWS configuration that references one is refused here, as
// repair feedback, rather than failing later in file().
func placeAWSUserData(files map[string][]byte, cloud string, script []byte) error {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		// EqualFold: on a case-insensitive filesystem a differently-cased
		// name is the same file.
		if strings.EqualFold(filepath.Clean(name), generator.AWSUserDataFile) {
			return fmt.Errorf("generated file %q is refused: infrafactory renders %s itself from the scenario's service: block", name, generator.AWSUserDataFile)
		}
	}
	if script != nil {
		files[generator.AWSUserDataFile] = script
		return nil
	}
	if cloud != "aws" {
		return nil
	}
	for _, name := range names {
		if bytes.Contains(files[name], []byte(generator.AWSUserDataFile)) {
			return fmt.Errorf("%s references %s, but the scenario declares no service: block, so there is no script to boot: remove the reference", name, generator.AWSUserDataFile)
		}
	}
	return nil
}

// writeFileExclusive creates path and fails if anything, a symlink
// included, already sits there.
func writeFileExclusive(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, perm)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}
