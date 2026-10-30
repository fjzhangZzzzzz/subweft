package validator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"go.yaml.in/yaml/v3"
)

const Version = "v1.19.33"

type Checker interface {
	Check(context.Context, []byte) error
	Identity() string
}
type Native struct {
	Path     string
	SHA256   string
	WorkDir  string
	peak     atomic.Uint64
	corePeak atomic.Uint64
	sem      chan struct{}
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
	return n.check(ctx, config, assets, false)
}

// Definition validates the exact client configuration without fetching HTTP providers.
// GeoDB files are supplied by deployment and never downloaded by the application.
type Definition struct {
	Native *Native
	GeoDB  map[string][]byte
}

func (d Definition) Identity() string { return d.Native.Identity() + "/definition" }
func (d Definition) Check(ctx context.Context, config []byte) error {
	return d.Native.check(ctx, config, d.GeoDB, true)
}

func (n *Native) check(ctx context.Context, config []byte, assets map[string][]byte, definition bool) error {
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
	if definition {
		for name, data := range assets {
			if name != "Country.mmdb" && name != "GeoIP.dat" && name != "GeoSite.dat" && name != "ASN.mmdb" {
				return domain.Invalid("invalid_geodata", "check")
			}
			if os.WriteFile(filepath.Join(dir, name), data, 0600) != nil {
				return domain.Invalid("validation_failed", "check")
			}
		}
		geoIPFile := "Country.mmdb"
		if configMap["geodata-mode"] == true {
			geoIPFile = "GeoIP.dat"
		}
		required := map[string]bool{}
		var inspect func(any)
		inspect = func(value any) {
			switch v := value.(type) {
			case string:
				lower := strings.ToLower(v)
				if strings.HasPrefix(lower, "geoip,") || strings.HasPrefix(lower, "geoip:") {
					required[geoIPFile] = true
				}
				if strings.HasPrefix(lower, "geosite:") || strings.HasPrefix(lower, "geosite,") {
					required["GeoSite.dat"] = true
				}
			case []any:
				for _, item := range v {
					inspect(item)
				}
			case map[string]any:
				for key, item := range v {
					if key == "geoip" && item == true {
						required[geoIPFile] = true
					}
					if key == "geosite" {
						required["GeoSite.dat"] = true
					}
					inspect(key)
					inspect(item)
				}
			}
		}
		inspect(configMap)
		for filename := range required {
			if len(assets[filename]) == 0 {
				return domain.Invalid("geodb_required", "check")
			}
		}

	} else {
		// Absent geographic URLs are supplied by Mihomo itself, hence remain remote.
		remoteGeo := map[string]bool{"geosite": true, "asn": true}
		if configMap["geodata-mode"] == true {
			remoteGeo["geoip"] = true
		} else {
			remoteGeo["mmdb"] = true
		}
		if urls, ok := configMap["geox-url"].(map[string]any); ok {
			for key, raw := range urls {
				remote, ok := raw.(string)
				if !ok {
					return domain.Invalid("invalid_geodata", "publish")
				}
				if !strings.HasPrefix(remote, compile.ResourcePrefix) {
					if source.ValidateURL(remote) != nil {
						return domain.Invalid("invalid_url", "publish")
					}
					remoteGeo[key] = true
					continue
				}
				delete(remoteGeo, key)
				data, ok := assets[strings.TrimPrefix(remote, compile.ResourcePrefix)]
				if !ok {
					return domain.Invalid("validation_resource_missing", "publish")
				}
				filename := map[string]string{"mmdb": "Country.mmdb", "geoip": "GeoIP.dat", "geosite": "GeoSite.dat", "asn": "ASN.mmdb"}[key]
				if filename == "" || os.WriteFile(filepath.Join(dir, filename), data, 0600) != nil {
					return domain.Invalid("validation_failed", "publish")
				}
			}
			delete(configMap, "geox-url")
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
					if strings.HasPrefix(remote, compile.ResourcePrefix) {
						if !ok {
							return domain.Invalid("validation_resource_missing", "publish")
						}
					} else {
						if source.ValidateURL(remote) != nil {
							return domain.Invalid("invalid_url", "publish")
						}
						data, e = remoteValidationData(providerKey, provider)
						if e != nil {
							return e
						}
					}
					if providerKey == "rule-providers" && provider["behavior"] == "classical" && len(remoteGeo) > 0 {
						format, _ := provider["format"].(string)
						data, e = projectClassicalRules(data, format, remoteGeo)
						if e != nil {
							return domain.Invalid("invalid_resource", "publish")
						}
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

		if len(remoteGeo) > 0 {
			deferRemoteGeo(configMap, remoteGeo)
		}

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
	// No inherited proxy or user configuration. Definition checks leave HTTP providers
	// intact; Mihomo's test mode checks their definitions without fetching their content.
	if runtime.GOOS != "linux" {
		cmd.Env = []string{"HOME=" + dir, "USERPROFILE=" + dir, "APPDATA=" + dir, "LOCALAPPDATA=" + dir, "PATH=" + filepath.Dir(n.Path), "SystemRoot=" + os.Getenv("SystemRoot")}
	}
	var output limitedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	e = cmd.Run()
	if b, err := os.ReadFile(filepath.Join(dir, "validator-usage.json")); err == nil {
		var usage struct {
			RSS uint64 `json:"rss"`
		}
		if json.Unmarshal(b, &usage) == nil {
			for old := n.corePeak.Load(); usage.RSS > old; old = n.corePeak.Load() {
				if n.corePeak.CompareAndSwap(old, usage.RSS) {
					break
				}
			}
		}
	}
	if value := processPeakRSS(cmd.ProcessState); value > 0 {
		for old := n.peak.Load(); value > old; old = n.peak.Load() {
			if n.peak.CompareAndSwap(old, value) {
				break
			}
		}
	}
	if e != nil {
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

// PeakRSS reports the fixed core child RSS collected by the private launcher.
func (n *Native) PeakRSS() uint64    { return n.corePeak.Load() }
func (n *Native) JobPeakRSS() uint64 { return n.peak.Load() }
func PeakRSS(c Checker) uint64 {
	if v, ok := c.(interface{ PeakRSS() uint64 }); ok {
		return v.PeakRSS()
	}
	return 0
}

func JobPeakRSS(c Checker) uint64 {
	if v, ok := c.(interface{ JobPeakRSS() uint64 }); ok {
		return v.JobPeakRSS()
	}
	return 0
}
