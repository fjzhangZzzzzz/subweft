//go:build linux

package validator

import (
	"encoding/json"
	"os"
	"os/exec"

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
		_, _ = os.Stderr.WriteString("validator_child_workdir_failed\n")
		os.Exit(126)
	}
	if unix.Setrlimit(unix.RLIMIT_NPROC, &unix.Rlimit{Cur: 96, Max: 96}) != nil {
		_, _ = os.Stderr.WriteString("validator_child_process_limit_failed\n")
		os.Exit(126)
	}
	cmd := exec.Command("/validator", os.Args[2:]...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	usage, _ := json.Marshal(struct {
		RSS uint64 `json:"rss"`
	}{processPeakRSS(cmd.ProcessState)})
	_ = os.WriteFile("/work/validator-usage.json", usage, 0600)
	if err != nil {
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() > 0 {
			os.Exit(cmd.ProcessState.ExitCode())
		}
		_, _ = os.Stderr.WriteString("validator_child_exec_failed\n")
		os.Exit(126)
	}
	os.Exit(0)
}
