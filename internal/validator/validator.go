package validator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

const Version = "v1.19.33"

type Checker interface {
	Check(context.Context, []byte) error
	Identity() string
}
type Native struct {
	Path    string
	SHA256  string
	WorkDir string
	sem     chan struct{}
}

func New(path, sha, workDir string) (*Native, error) {
	expected, supported := ExpectedSHA(runtime.GOOS, runtime.GOARCH)
	if !supported || sha != expected {
		return nil, errors.New("validator_not_in_artifact_lock")
	}
	if !filepath.IsAbs(path) || len(sha) != 64 {
		return nil, errors.New("validator_configuration_invalid")
	}
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() {
		return nil, errors.New("validator_file_invalid")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return nil, e
	}
	if hex.EncodeToString(h.Sum(nil)) != sha {
		return nil, errors.New("validator_digest_mismatch")
	}
	if e = os.MkdirAll(workDir, 0700); e != nil {
		return nil, e
	}
	return &Native{Path: path, SHA256: sha, WorkDir: workDir, sem: make(chan struct{}, 1)}, nil
}
func (n *Native) Identity() string { return "mihomo/" + Version + "/sha256:" + n.SHA256 }

type limitedOutput struct{ b bytes.Buffer }

func (l *limitedOutput) Write(b []byte) (int, error) {
	length := len(b)
	if l.b.Len() < 8192 {
		keep := min(len(b), 8192-l.b.Len())
		l.b.Write(b[:keep])
	}
	return length, nil
}
func (n *Native) Check(ctx context.Context, config []byte) error {
	select {
	case n.sem <- struct{}{}:
		defer func() { <-n.sem }()
	case <-ctx.Done():
		return domain.Invalid("validation_cancelled", "publish")
	}
	// Recheck the executable before each invocation; never accept a changed binary silently.
	f, e := os.Open(n.Path)
	if e != nil {
		return domain.Invalid("validator_unavailable", "publish")
	}
	h := sha256.New()
	_, e = io.Copy(h, f)
	f.Close()
	if e != nil || hex.EncodeToString(h.Sum(nil)) != n.SHA256 {
		return domain.Invalid("validator_digest_mismatch", "publish")
	}
	dir, e := os.MkdirTemp(n.WorkDir, "validation-")
	if e != nil {
		return domain.Invalid("validation_failed", "publish")
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "config.yaml")
	if e = os.WriteFile(file, config, 0600); e != nil {
		return domain.Invalid("validation_failed", "publish")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, n.Path, "-t", "-d", dir, "-f", file)
	hideWindow(cmd)
	cmd.Dir = dir
	// No inherited proxy or user configuration. The compiler emits no remote rule/providers.
	cmd.Env = []string{"HOME=" + dir, "USERPROFILE=" + dir, "APPDATA=" + dir, "LOCALAPPDATA=" + dir, "PATH=" + filepath.Dir(n.Path), "SystemRoot=" + os.Getenv("SystemRoot")}
	var output limitedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if e = cmd.Run(); e != nil {
		if ctx.Err() != nil {
			return domain.Invalid("validation_timeout", "publish")
		}
		return domain.Invalid("native_validation_failed", "publish")
	}
	if !strings.Contains(output.b.String(), "test is successful") {
		return domain.Invalid("validator_output_unrecognized", "publish")
	}
	return nil
}
