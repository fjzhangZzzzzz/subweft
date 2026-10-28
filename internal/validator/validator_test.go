package validator_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
	"github.com/fjzhangZzzzzz/subweft/internal/validator"
)

func TestNativePinnedMihomoAcceptsAndRejects(t *testing.T) {
	name := "mihomo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary, e := filepath.Abs(filepath.Join("..", "..", "output", "tools", name))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(binary); e != nil {
		t.Skip("run python tools/fetch_validator.py to execute native validation")
	}
	sha, ok := validator.ExpectedSHA(runtime.GOOS, runtime.GOARCH)
	if !ok {
		t.Skip("platform not in artifact lock")
	}
	checker, e := validator.New(binary, sha, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	nodes, e := source.Parse([]byte(testfixture.ProviderYAML))
	if e != nil {
		t.Fatal(e)
	}
	p := domain.Profile{Name: "native", Groups: []domain.Group{{Name: "Proxy", Type: "select", AllSources: true}}, Rules: []domain.Rule{{Type: "MATCH", Target: "Proxy", Enabled: true}}, Settings: domain.Settings{MixedPort: 7890}}
	result, e := compile.Compile(p, domain.Snapshot{Sources: []domain.Source{{Meta: domain.Meta{ID: "sample"}, Name: "sample", Enabled: true, Nodes: nodes}}}, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = checker.Check(context.Background(), result.Config); e != nil {
		t.Fatal("native validation rejected valid configuration", e)
	}
	if e = checker.Check(context.Background(), []byte("invalid: [\n")); e == nil {
		t.Fatal("native validation accepted invalid configuration")
	}
	if _, e = validator.New(binary, "0000000000000000000000000000000000000000000000000000000000000000", t.TempDir()); e == nil {
		t.Fatal("unlocked validator accepted")
	}
}

func TestNativeProtocolAndTransportSamples(t *testing.T) {
	name := "mihomo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary, e := filepath.Abs(filepath.Join("..", "..", "output", "tools", name))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(binary); e != nil {
		t.Skip("fetch locked validator first")
	}
	sha, ok := validator.ExpectedSHA(runtime.GOOS, runtime.GOARCH)
	if !ok {
		t.Skip("platform not locked")
	}
	checker, e := validator.New(binary, sha, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	nodes, e := source.Parse([]byte(testfixture.ProtocolProviderYAML))
	if e != nil {
		t.Fatal(e)
	}
	for _, cipher := range []string{"auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero"} {
		nodes = append(nodes, domain.Node{Name: "VMess-" + cipher, Type: "vmess", Server: "192.0.2.1", Port: 443, UUID: "00000000-0000-4000-8000-000000000001", Cipher: cipher, Network: "tcp"})
	}
	for _, fingerprint := range []string{"chrome", "firefox", "safari", "ios", "android", "edge", "random", "randomized"} {
		nodes = append(nodes, domain.Node{Name: "Fingerprint-" + fingerprint, Type: "vless", Server: "192.0.2.1", Port: 443, UUID: "00000000-0000-4000-8000-000000000001", TLS: true, ServerName: "example.invalid", ClientFingerprint: fingerprint})
	}
	for _, node := range nodes {
		t.Run(node.Name, func(t *testing.T) {
			p := domain.Profile{Name: "native", Groups: []domain.Group{{Name: "Proxy", Type: "select", AllSources: true}}, Rules: []domain.Rule{{Type: "MATCH", Target: "Proxy", Enabled: true}}, Settings: domain.Settings{MixedPort: 7890}}
			result, e := compile.Compile(p, domain.Snapshot{Sources: []domain.Source{{Meta: domain.Meta{ID: "fixture"}, Name: "fixture", Enabled: true, Nodes: []domain.Node{node}}}}, false)
			if e != nil {
				t.Fatal(e)
			}
			if e = checker.Check(context.Background(), result.Config); e != nil {
				t.Fatal("native protocol sample rejected", e)
			}
		})
	}
}
