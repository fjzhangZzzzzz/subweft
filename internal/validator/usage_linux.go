//go:build linux

package validator

import (
	"os"
	"syscall"
)

func processPeakRSS(state *os.ProcessState) uint64 {
	if state == nil {
		return 0
	}
	usage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok || usage.Maxrss < 0 {
		return 0
	}
	return uint64(usage.Maxrss) * 1024
}
