package migrate_test

import (
	"github.com/fjzhangZzzzzz/subweft/internal/migrate"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
	"strings"
	"testing"
)

func TestImportPreservesExplicitMembershipAndOrder(t *testing.T) {
	p, e := migrate.Parse(migrate.Input{Name: "Imported", Content: testfixture.MigrationConfig})
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Sources) != 2 || len(p.Nodes) != 1 || len(p.Resources) != 3 || p.Profile.GeoIPID == "" {
		t.Fatal("import lost inputs")
	}
	for _, g := range p.Profile.Groups {
		if g.AllSources || len(g.Tags) != 0 {
			t.Fatal("import inferred unrequested membership")
		}
	}
	if len(p.Profile.Groups[0].SourceIDs) != 2 || len(p.Profile.Groups[1].SourceIDs) != 1 {
		t.Fatal("explicit membership changed")
	}
	policies := p.Profile.Settings.NameserverPolicy
	if len(policies) != 2 || policies[0].Pattern != "+.example.invalid" || policies[1].Pattern != "*.invalid" {
		t.Fatal("ordered DNS policy changed")
	}
	if p.Profile.Rules[0].Type != "RULE-SET" || p.Profile.Rules[2].Type != "GEOIP" || p.Profile.Rules[3].Type != "MATCH" {
		t.Fatal("rule order changed")
	}
	if !p.Sources[0].Update.HealthCheck.Enable {
		t.Fatal("client provider health check lost")
	}
}
func TestImportRejectsUnknownFieldsTypesAndInvalidReferences(t *testing.T) {
	for _, input := range []string{strings.Replace(testfixture.MigrationConfig, "allow-lan: false", "allow-lan: 'false'", 1), strings.Replace(testfixture.MigrationConfig, "interval: 3600", "interval: '3600'", 1), strings.Replace(testfixture.MigrationConfig, "enable: true", "enable: 'true'", 1), strings.Replace(testfixture.MigrationConfig, "type: select", "type: select, mystery: true", 1), strings.Replace(testfixture.MigrationConfig, "use: [quota]", "use: [missing]", 1), testfixture.MigrationConfig + "unknown-setting: true\n"} {
		if _, e := migrate.Parse(migrate.Input{Name: "Imported", Content: input}); e == nil {
			t.Fatal("invalid input silently accepted")
		}
	}
}
func TestLogicalRulesRoundTrip(t *testing.T) {
	r, e := migrate.ParseRule("AND,((DOMAIN-SUFFIX,example.invalid),(NOT,((NETWORK,UDP)))),Proxy")
	if e != nil || r.Type != "AND" || len(r.Conditions) != 2 || r.Conditions[1].Type != "NOT" {
		t.Fatal("logical rule import failed", e)
	}
}
