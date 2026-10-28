//go:build linux

package validator

import (
	"os"

	"golang.org/x/sys/unix"
)

const childMarker = "__subweft_validator_child__"

// The static application reexecutes inside bubblewrap before executing the core.
// Apply RLIMIT_NPROC here: applying it before creating the user namespace counts
// unrelated host processes belonging to the service account as well.
func init() {
	if len(os.Args) < 2 || os.Args[1] != childMarker || os.Getenv("SUBWEFT_VALIDATOR_CHILD") != "1" {
		return
	}
	if cwd, _ := os.Getwd(); cwd != "/work" {
		os.Exit(126)
	}
	if unix.Setrlimit(unix.RLIMIT_NPROC, &unix.Rlimit{Cur: 96, Max: 96}) != nil {
		os.Exit(126)
	}
	args := append([]string{"/validator"}, os.Args[2:]...)
	_ = unix.Exec("/validator", args, os.Environ())
	os.Exit(126)
}
