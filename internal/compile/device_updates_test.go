package compile_test

import (
	"strings"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"go.yaml.in/yaml/v3"
)

func remoteFixture() (domain.Profile, domain.Snapshot) {
	p := domain.Profile{Meta: domain.Meta{ID: "profile-a"}, Name: "remote", Groups: []domain.Group{{Name: "Proxy", Type: "select", SourceIDs: []string{"s"}}, {Name: "Other", Type: "select", SourceIDs: []string{"s"}}}, Rules: []domain.Rule{{Type: "RULE-SET", Value: "r", Target: "Proxy", Enabled: true}, {Type: "MATCH", Target: "Proxy", Enabled: true}}}
	s := domain.Snapshot{Sources: []domain.Source{{Meta: domain.Meta{ID: "s"}, Name: "source", Kind: "http", URL: "https://example.invalid/sub?token=fixture-secret", Interval: 3600, Enabled: true, Update: domain.DeviceUpdate{Headers: map[string]string{"Authorization": "fixture-header"}}}}, Resources: []domain.Resource{{Meta: domain.Meta{ID: "r"}, Name: "rules", Kind: "http", Type: "ruleset", URL: "https://example.invalid/rules?key=fixture-rule", Interval: 86400, Behavior: "domain", Format: "yaml", Enabled: true}}}
	return p, s
}
func TestRemoteDefinitionsAreSharedAndRedactedWithoutCache(t *testing.T) {
	p, s := remoteFixture()
	r, e := compile.Compile(p, s, false)
	if e != nil {
		t.Fatal(e)
	}
	var c map[string]any
	if e = yaml.Unmarshal(r.Config, &c); e != nil {
		t.Fatal(e)
	}
	providers := c["proxy-providers"].(map[string]any)
	if len(providers) != 1 || len(r.Resources) != 0 || len(r.Assets) != 0 {
		t.Fatal("remote source duplicated or cached")
	}
	if len(c["proxies"].([]any)) != 0 {
		t.Fatal("synthetic proxy leaked into published config")
	}
	preview := string(compile.Redact(r.Config))
	for _, secret := range []string{"fixture-secret", "fixture-header", "fixture-rule"} {
		if strings.Contains(preview, secret) {
			t.Fatal("preview leaked upstream credentials")
		}
	}
	path := providers["s"].(map[string]any)["path"]
	p.ID = "profile-b"
	other, e := compile.Compile(p, s, false)
	if e != nil {
		t.Fatal(e)
	}
	_ = yaml.Unmarshal(other.Config, &c)
	if c["proxy-providers"].(map[string]any)["s"].(map[string]any)["path"] == path {
		t.Fatal("profiles share cache path")
	}
	p.ID = "profile-a"
	s.Sources[0].URL += "-new"
	other, e = compile.Compile(p, s, false)
	if e != nil {
		t.Fatal(e)
	}
	_ = yaml.Unmarshal(other.Config, &c)
	if c["proxy-providers"].(map[string]any)["s"].(map[string]any)["path"] == path {
		t.Fatal("changed address shares cache")
	}
}
func TestDeviceDownloadDependenciesAndInvalidDefinitions(t *testing.T) {
	for name, mutate := range map[string]func(*domain.Profile, *domain.Snapshot){
		"self cycle":        func(p *domain.Profile, s *domain.Snapshot) { p.SourceUpdates = map[string]string{"s": "Proxy"} },
		"other group cycle": func(p *domain.Profile, s *domain.Snapshot) { p.SourceUpdates = map[string]string{"s": "Other"} },
		"missing route":     func(p *domain.Profile, s *domain.Snapshot) { p.SourceUpdates = map[string]string{"s": "Absent"} },
		"invalid header":    func(p *domain.Profile, s *domain.Snapshot) { s.Sources[0].Update.Headers["X-Test"] = "bad\r\n" },
		"invalid filter":    func(p *domain.Profile, s *domain.Snapshot) { s.Sources[0].Update.Filter = "[" },
		"legacy override": func(p *domain.Profile, s *domain.Snapshot) {
			s.Sources[0].Overrides = []domain.NodeOverride{{Identity: "old"}}
		},
		"legacy proxy": func(p *domain.Profile, s *domain.Snapshot) { s.Sources[0].LegacyProxy = "http://example.invalid:8080" },
	} {
		t.Run(name, func(t *testing.T) {
			p, s := remoteFixture()
			mutate(&p, &s)
			if _, e := compile.Compile(p, s, false); e == nil {
				t.Fatal("invalid dependency or definition accepted")
			}
		})
	}
	p, s := remoteFixture()
	s.Nodes = []domain.ManualNode{{Meta: domain.Meta{ID: "bootstrap"}, Enabled: true, Node: domain.Node{Name: "Bootstrap", Type: "socks5", Server: "192.0.2.1", Port: 1080}}}
	p.SourceUpdates = map[string]string{"s": "Bootstrap"}
	p.RuleUpdates = map[string]string{"r": "Bootstrap"}
	r, e := compile.Compile(p, s, false)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(r.Config), "name: Bootstrap") {
		t.Fatal("independent bootstrap node omitted")
	}
}

func TestDefaultUpdateFollowsRulesAndExplicitDirectSurvives(t *testing.T) {
	p, s := remoteFixture()
	for _, explicit := range []bool{false, true} {
		if explicit {
			p.SourceUpdates = map[string]string{"s": "DIRECT"}
			p.RuleUpdates = map[string]string{"r": "DIRECT"}
		}
		result, e := compile.Compile(p, s, false)
		if e != nil {
			t.Fatal(e)
		}
		var config map[string]any
		if e = yaml.Unmarshal(result.Config, &config); e != nil {
			t.Fatal(e)
		}
		for _, key := range []string{"proxy-providers", "rule-providers"} {
			for _, raw := range config[key].(map[string]any) {
				route, ok := raw.(map[string]any)["proxy"]
				if ok != explicit || explicit && route != "DIRECT" {
					t.Fatalf("%s proxy selection mismatch", key)
				}
			}
		}
		if _, ok := config["geox-url"]; ok {
			t.Fatal("core geographic defaults replaced")
		}
		if _, ok := config["geo-auto-update"]; ok {
			t.Fatal("implicit geodata update changed")
		}
	}
}

func TestImplicitDATGeodataPreservesCoreURLs(t *testing.T) {
	p, s := remoteFixture()
	p.GeoDataMode = true
	r, e := compile.Compile(p, s, false)
	if e != nil {
		t.Fatal(e)
	}
	var c map[string]any
	if e = yaml.Unmarshal(r.Config, &c); e != nil {
		t.Fatal(e)
	}
	if c["geodata-mode"] != true || c["geox-url"] != nil {
		t.Fatal("DAT mode or implicit URLs lost")
	}
}

func TestExplicitSubscriptionOrderSurvivesSnapshotOrder(t *testing.T) {
	p, s := remoteFixture()
	s.Sources = append(s.Sources, domain.Source{Meta: domain.Meta{ID: "second"}, Name: "second", Kind: "http", URL: "https://example.invalid/second", Interval: 3600, Enabled: true})
	p.Groups = []domain.Group{{Name: "Proxy", Type: "select", SourceIDs: []string{"second", "s"}, AllSources: true}}
	r, e := compile.Compile(p, s, false)
	if e != nil {
		t.Fatal(e)
	}
	var config map[string]any
	if e = yaml.Unmarshal(r.Config, &config); e != nil {
		t.Fatal(e)
	}
	groups := config["proxy-groups"].([]any)
	uses := groups[0].(map[string]any)["use"].([]any)
	if len(uses) != 2 || uses[0] != "second" || uses[1] != "s" {
		t.Fatalf("subscription order or deduplication lost: %v", uses)
	}
}
