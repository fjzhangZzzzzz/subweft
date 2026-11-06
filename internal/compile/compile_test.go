package compile_test

import (
	"strings"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
)

func fixture(t *testing.T) (domain.Profile, domain.Snapshot) {
	t.Helper()
	nodes, e := source.Parse([]byte(testfixture.ProviderYAML))
	if e != nil {
		t.Fatal(e)
	}
	return domain.Profile{Name: "test", Groups: []domain.Group{{Name: "Proxy", Type: "select", AllSources: true}}, Rules: []domain.Rule{{Type: "DOMAIN-SUFFIX", Value: "example.invalid", Target: "Proxy", Enabled: true}, {Type: "MATCH", Target: "Proxy", Enabled: true}}, Settings: domain.Settings{MixedPort: 7890}}, domain.Snapshot{Sources: []domain.Source{{Meta: domain.Meta{ID: "source"}, Name: "sample", Enabled: true, Nodes: nodes, Tags: []string{"quota"}}}}
}
func TestSelectionRulesAndRedaction(t *testing.T) {
	p, snap := fixture(t)
	p.Groups[0].Filter = "QUOTA"
	got, e := compile.Compile(p, snap, false)
	if e != nil {
		t.Fatal(e)
	}
	if len(got.Members["Proxy"]) != 1 || got.Members["Proxy"][0] != "[sample] QUOTA-01" {
		t.Fatal("filter not applied")
	}
	if strings.Contains(string(got.Config), "http://") {
		t.Fatal("upstream URL leaked")
	}
	if strings.Contains(string(compile.Redact(got.Config)), "fixture-proxy-password") {
		t.Fatal("preview leaked password")
	}
	if !strings.Contains(string(got.Config), "DOMAIN-SUFFIX,example.invalid,Proxy") {
		t.Fatal("rule missing")
	}
}
func TestInvalidReferencesAndRules(t *testing.T) {
	tests := map[string]func(*domain.Profile, *domain.Snapshot){"cycle": func(p *domain.Profile, s *domain.Snapshot) { p.Groups[0].Groups = []string{"Proxy"} }, "empty": func(p *domain.Profile, s *domain.Snapshot) { p.Groups[0].Filter = "never-matches" }, "missing target": func(p *domain.Profile, s *domain.Snapshot) { p.Rules[0].Target = "Absent" }, "invalid filter": func(p *domain.Profile, s *domain.Snapshot) { p.Groups[0].Filter = "[" }, "match order": func(p *domain.Profile, s *domain.Snapshot) { p.Rules[0], p.Rules[1] = p.Rules[1], p.Rules[0] }, "stale": func(p *domain.Profile, s *domain.Snapshot) { s.Sources[0].LastError = "fetch_failed" }, "missing source": func(p *domain.Profile, s *domain.Snapshot) { p.Groups[0].SourceIDs = []string{"missing"} }}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			p, snap := fixture(t)
			mutate(&p, &snap)
			if _, e := compile.Compile(p, snap, false); e == nil {
				t.Fatal("invalid profile accepted")
			}
		})
	}
}
