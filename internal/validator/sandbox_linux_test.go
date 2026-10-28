//go:build linux

package validator

import (
	"context"
	"encoding/json"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSandboxBlocksHostFilesAndLoopback(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(t.TempDir(), "host-only")
	if e := os.WriteFile(secret, []byte("synthetic-host-secret"), 0600); e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	input, _ := json.Marshal(map[string]string{"secret": secret, "address": listener.Addr().String()})
	if e = os.WriteFile(filepath.Join(dir, "probe.json"), input, 0600); e != nil {
		t.Fatal(e)
	}
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd, e := isolatedCommand(ctx, binary, dir, []string{"-test.run=^TestSandboxChild$"})
	if e != nil {
		t.Fatal("sandbox dependency unavailable")
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		// This is our synthetic filesystem/network probe, not validator output or user configuration.
		t.Logf("synthetic isolation probe: %s", output)
		t.Fatal("sandbox probe failed")
	}
	if _, e = os.Stat(filepath.Join(dir, "probe-passed")); e != nil {
		t.Fatal("sandbox child did not finish")
	}
}
func TestSandboxChild(t *testing.T) {
	cwd, _ := os.Getwd()
	if cwd != "/work" {
		t.Skip("subprocess helper")
	}
	var limit unix.Rlimit
	if unix.Getrlimit(unix.RLIMIT_NPROC, &limit) != nil || limit.Cur != 96 || limit.Max != 96 {
		t.Fatal("sandbox process-count limit not applied")
	}
	b, e := os.ReadFile("/work/probe.json")
	if e != nil {
		t.Fatal("probe input unavailable")
	}
	var in map[string]string
	_ = json.Unmarshal(b, &in)
	if _, e = os.ReadFile(in["secret"]); e == nil {
		t.Fatal("host filesystem exposed")
	}
	conn, e := net.DialTimeout("tcp", in["address"], 300*time.Millisecond)
	if e == nil {
		conn.Close()
		t.Fatal("host network exposed")
	}
	if e = os.WriteFile("/work/probe-passed", []byte("ok"), 0600); e != nil {
		t.Fatal("private work directory not writable")
	}
}
