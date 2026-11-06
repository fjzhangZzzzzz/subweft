//go:build !linux

package validator

import (
	"context"
	"os/exec"
)

// Non-Linux builds are local development builds, not production targets.
func sandboxCommand(ctx context.Context, binary, dir, file string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, binary, "-t", "-d", dir, "-f", file), nil
}
func checkSandbox(binary, dir string) error { return nil }
