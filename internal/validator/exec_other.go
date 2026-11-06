//go:build !windows

package validator

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
