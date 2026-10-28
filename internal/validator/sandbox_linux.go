//go:build linux

package validator

import (
	"context"
	"errors"
	"os"
	"os/exec"
)

// Only the private validation directory and the locked static core are visible.
// A separate network namespace blocks both internet access and the host loopback.
func sandboxCommand(ctx context.Context, binary, dir, file string) (*exec.Cmd, error) {
	args := []string{"-t", "-d", "/work", "-f", "/work/config.yaml"}
	if file == "" {
		args = []string{"-v"}
	}
	return isolatedCommand(ctx, binary, dir, args)
}
func isolatedCommand(ctx context.Context, binary, dir string, args []string) (*exec.Cmd, error) {
	for _, path := range []string{"/usr/bin/bwrap", "/usr/bin/prlimit"} {
		info, e := os.Stat(path)
		if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSetuid != 0 {
			return nil, errors.New("validator_sandbox_unavailable")
		}
	}
	flags := []string{"--as=2147483648", "--cpu=10", "--fsize=134217728", "--nofile=128", "--nproc=96", "--", "/usr/bin/bwrap", "--unshare-all", "--unshare-user", "--die-with-parent", "--new-session", "--cap-drop", "ALL", "--clearenv", "--setenv", "HOME", "/work", "--setenv", "GOMEMLIMIT", "128MiB", "--setenv", "GOMAXPROCS", "2", "--ro-bind", binary, "/validator", "--bind", dir, "/work", "--tmpfs", "/tmp", "--dev", "/dev", "--proc", "/proc", "--chdir", "/work", "--", "/validator"}
	flags = append(flags, args...)
	cmd := exec.CommandContext(ctx, "/usr/bin/prlimit", flags...)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.Dir = dir
	return cmd, nil
}
func checkSandbox(binary, dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3e9)
	defer cancel()
	cmd, e := sandboxCommand(ctx, binary, dir, "")
	if e != nil {
		return e
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if cmd.Run() != nil {
		return errors.New("validator_sandbox_unavailable")
	}
	return nil
}
