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
	more, e := source.Parse([]byte(testfixture.RemainingProtocolProviderYAML))
	if e != nil {
		t.Fatal("remaining protocols rejected", e)
	}
	nodes = append(nodes, more...)
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

func TestNativeManagedProviderRulesAndDNSBundle(t *testing.T) {
	name := "mihomo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary, _ := filepath.Abs(filepath.Join("..", "..", "output", "tools", name))
	if _, e := os.Stat(binary); e != nil {
		t.Skip("fetch locked validator first")
	}
	country, e := os.ReadFile(filepath.Join("..", "..", "output", "tools", "GeoIP2-Country-Test.mmdb"))
	if e != nil {
		t.Skip("fetch pinned MMDB test fixture first")
	}
	sha, _ := validator.ExpectedSHA(runtime.GOOS, runtime.GOARCH)
	checker, e := validator.New(binary, sha, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	nodes, e := source.Parse([]byte(testfixture.ProviderYAML))
	if e != nil {
		t.Fatal(e)
	}
	rules := []byte("payload:\n  - +.example.invalid\n")
	p := domain.Profile{Name: "native bundle", GeoIPID: "country", Groups: []domain.Group{{Name: "Proxy", Type: "select", AllSources: true}}, Rules: []domain.Rule{{Type: "AND", Conditions: []domain.Condition{{Type: "DOMAIN-SUFFIX", Value: "example.invalid"}, {Type: "NETWORK", Value: "TCP"}}, Target: "Proxy", Enabled: true}, {Type: "RULE-SET", Value: "rules", Target: "Proxy", Enabled: true}, {Type: "GEOIP", Value: "CN", Target: "DIRECT", Enabled: true}, {Type: "MATCH", Target: "Proxy", Enabled: true}}, Settings: domain.Settings{MixedPort: 7890, DNSEnabled: true, DNSMode: "fake-ip", Nameservers: []string{"https://dns.google/dns-query"}, DNS: map[string]any{"default-nameserver": []string{"1.1.1.1"}, "respect-rules": true, "proxy-server-nameserver": []string{"1.1.1.1"}, "fallback": []string{"https://dns.google/dns-query"}, "fallback-filter": map[string]any{"geoip": true, "geoip-code": "CN", "ipcidr": []string{"240.0.0.0/4"}, "domain": []string{"+.example.invalid"}}}, NameserverPolicy: []domain.DNSPolicy{{Pattern: "+.example.invalid", Nameservers: []string{"1.1.1.1"}}}}}
	snap := domain.Snapshot{Sources: []domain.Source{{Meta: domain.Meta{ID: "fixture"}, Name: "fixture", Enabled: true, Nodes: nodes, ClientProvider: &domain.ClientProvider{Enabled: true, Interval: 3600, HealthCheck: domain.HealthCheck{Enable: true, URL: "https://www.gstatic.com/generate_204", Interval: 300, Timeout: 5000, Lazy: true}}}}, Resources: []domain.Resource{{Meta: domain.Meta{ID: "rules"}, Name: "示例规则", Type: "ruleset", Behavior: "domain", Format: "yaml", Digest: compile.Digest(rules), Enabled: true}, {Meta: domain.Meta{ID: "country"}, Type: "mmdb", Digest: compile.Digest(country), Enabled: true}}}
	result, e := compile.Compile(p, snap, false)
	if e != nil {
		t.Fatal(e)
	}
	assets := result.Assets
	assets["rules"] = rules
	assets["country"] = country
	if e = checker.CheckBundle(context.Background(), result.Config, assets); e != nil {
		t.Fatal("native managed bundle rejected", e)
	}
	delete(assets, "rules")
	if e = checker.CheckBundle(context.Background(), result.Config, assets); e == nil {
		t.Fatal("missing published resource was accepted")
	}
}
