//go:build linux

package validator

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
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
	launcher, err := os.Executable()
	if err != nil {
		return nil, errors.New("validator_sandbox_launcher_unavailable")
	}
	// The child launcher applies the same hard process-count limit inside the new user namespace.
	flags := []string{"--as=2147483648", "--cpu=10", "--fsize=134217728", "--nofile=128", "--", "/usr/bin/bwrap", "--unshare-all", "--unshare-user", "--die-with-parent", "--new-session", "--cap-drop", "ALL", "--clearenv", "--setenv", "HOME", "/work", "--setenv", "SUBWEFT_VALIDATOR_CHILD", "1", "--setenv", "GOMEMLIMIT", "128MiB", "--setenv", "GOMAXPROCS", "2", "--ro-bind", binary, "/validator", "--ro-bind", launcher, "/runner", "--bind", dir, "/work", "--tmpfs", "/tmp", "--dev", "/dev", "--proc", "/proc", "--chdir", "/work", "--", "/runner", childMarker}
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		reason := strings.ToLower(string(output))
		switch {
		case strings.Contains(reason, "rtm_newaddr"), strings.Contains(reason, "namespace"), strings.Contains(reason, "uid map"), strings.Contains(reason, "gid map"):
			return errors.New("validator_sandbox_namespace_denied")
		case strings.Contains(reason, "no such file"), strings.Contains(reason, "not found"):
			return errors.New("validator_sandbox_static_executable_required")
		case strings.Contains(reason, "memory"), strings.Contains(reason, "reserve"):
			return errors.New("validator_sandbox_memory_limit")
		case strings.Contains(reason, "resource temporarily unavailable"):
			return errors.New("validator_sandbox_process_limit")
		case strings.Contains(reason, "mount"):
			return errors.New("validator_sandbox_mount_denied")
		default:
			return errors.New("validator_sandbox_preflight_failed")
		}
	}
	return nil
}
