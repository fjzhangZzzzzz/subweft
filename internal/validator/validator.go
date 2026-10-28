package validator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"go.yaml.in/yaml/v3"
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
	workDir, e = filepath.Abs(workDir)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(workDir, 0700); e != nil {
		return nil, e
	}
	if e = checkSandbox(path, workDir); e != nil {
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
	return n.CheckBundle(ctx, config, nil)
}
func (n *Native) CheckBundle(ctx context.Context, config []byte, assets map[string][]byte) error {
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
	var configMap map[string]any
	if yaml.Unmarshal(config, &configMap) != nil {
		return domain.Invalid("invalid_config", "publish")
	}
	for _, providerKey := range []string{"rule-providers", "proxy-providers"} {
		if providers, ok := configMap[providerKey].(map[string]any); ok {
			for _, raw := range providers {
				provider, ok := raw.(map[string]any)
				if !ok {
					return domain.Invalid("invalid_provider", "publish")
				}
				remote, _ := provider["url"].(string)
				id := strings.TrimPrefix(remote, compile.ResourcePrefix)
				data, ok := assets[id]
				if !ok || !strings.HasPrefix(remote, compile.ResourcePrefix) {
					return domain.Invalid("validation_resource_missing", "publish")
				}
				filename, _ := provider["path"].(string)
				target := filepath.Join(dir, filepath.FromSlash(filename))
				if !strings.HasPrefix(target, dir+string(os.PathSeparator)) {
					return domain.Invalid("invalid_resource_path", "publish")
				}
				if os.MkdirAll(filepath.Dir(target), 0700) != nil || os.WriteFile(target, data, 0600) != nil {
					return domain.Invalid("validation_failed", "publish")
				}
				provider["type"] = "file"
				delete(provider, "url")
				delete(provider, "interval")
			}
		}
	}

	if urls, ok := configMap["geox-url"].(map[string]any); ok {
		for key, raw := range urls {
			remote, ok := raw.(string)
			if !ok {
				return domain.Invalid("invalid_geodata", "publish")
			}
			data, ok := assets[strings.TrimPrefix(remote, compile.ResourcePrefix)]
			if !ok {
				return domain.Invalid("validation_resource_missing", "publish")
			}
			filename := map[string]string{"mmdb": "Country.mmdb", "geoip": "GeoIP.dat", "geosite": "GeoSite.dat"}[key]
			if filename == "" || os.WriteFile(filepath.Join(dir, filename), data, 0600) != nil {
				return domain.Invalid("validation_failed", "publish")
			}
		}
		delete(configMap, "geox-url")
	}
	config, e = yaml.Marshal(configMap)
	if e != nil {
		return domain.Invalid("validation_failed", "publish")
	}
	file := filepath.Join(dir, "config.yaml")
	if e = os.WriteFile(file, config, 0600); e != nil {
		return domain.Invalid("validation_failed", "publish")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd, e := sandboxCommand(ctx, n.Path, dir, file)
	if e != nil {
		return domain.Invalid("validator_sandbox_unavailable", "publish")
	}
	hideWindow(cmd)
	cmd.Dir = dir
	// No inherited proxy or user configuration. The compiler emits no remote rule/providers.
	if runtime.GOOS != "linux" {
		cmd.Env = []string{"HOME=" + dir, "USERPROFILE=" + dir, "APPDATA=" + dir, "LOCALAPPDATA=" + dir, "PATH=" + filepath.Dir(n.Path), "SystemRoot=" + os.Getenv("SystemRoot")}
	}
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
