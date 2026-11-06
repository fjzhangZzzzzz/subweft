package validator

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefinitionProjectionBoundaries(t *testing.T) {
	for _, input := range []string{
		"rules: [\"GEOIP,CN,DIRECT,no-resolve\"]",
		"geodata-mode: true\nrules: [\"GEOIP,CN,DIRECT\"]",
		"rules: [\"GEOSITE,cn,DIRECT\"]",
		"dns: {enable: true, nameserver-policy: {'geosite:cn': [1.1.1.1]}, fake-ip-filter: ['geosite:cn'], fallback-filter: {geoip: true, geosite: [cn]}}",
	} {
		projected, items, err := definitionProjection([]byte(input), nil)
		if err != nil || len(items) == 0 || len(projected) == 0 {
			t.Fatal("valid projection rejected", err)
		}
		for _, item := range items {
			if item.Status != "unverified" {
				t.Fatal("missing data marked passed")
			}
		}
	}
	for _, input := range []string{
		"rules: [\"GEOIP,CN,DIRECT,bad\"]", "rules: [\"GEOSITE,,DIRECT\"]",
		"rules: [\"GEOSITE,cn,DIRECT,no-resolve\"]",
		"dns: {fallback-filter: {geoip: 'true'}}", "dns: {fallback-filter: {geosite: cn}}",
		"dns: {nameserver-policy: {'geosite:': [1.1.1.1]}}",
		"dns: {fake-ip-filter: ['geosite:']}",
	} {
		if _, _, err := definitionProjection([]byte(input), nil); err == nil {
			t.Fatal("invalid geographic input accepted")
		}
	}
}
func TestDefinitionNativePartialChecks(t *testing.T) {
	name := "mihomo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path, _ := filepath.Abs(filepath.Join("..", "..", "output", "tools", name))
	if _, err := os.Stat(path); err != nil {
		t.Skip("locked native artifact unavailable")
	}
	sha, _ := ExpectedSHA(runtime.GOOS, runtime.GOARCH)
	n, err := New(path, sha, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := Definition{Native: n}
	original := []byte("rules: [\"GEOIP,CN,DIRECT,no-resolve\", \"MATCH,DIRECT\"]")
	report, err := d.CheckReport(context.Background(), original)
	if err != nil || len(report.Items) != 1 || report.Items[0].Status != "unverified" {
		t.Fatal("partial native check failed", err)
	}
	if !strings.Contains(string(original), "GEOIP,CN") {
		t.Fatal("original mutated")
	}
	for _, input := range []string{
		"rules: [\"GEOIP,CN,missing\"]",
		"rules: [\"GEOIP,CN,DIRECT\", \"IP-CIDR,invalid,DIRECT\"]",
		"dns: {enable: true, nameserver: [1.1.1.1], nameserver-policy: {'geosite:cn': ['bogus://resolver.invalid']}}",
	} {
		if err := d.Check(context.Background(), []byte(input)); err == nil {
			t.Fatal("native error hidden")
		}
	}
	for _, input := range []string{
		"geodata-mode: true\nrules: [\"GEOIP,CN,DIRECT\"]",
		"rules: [\"GEOSITE,cn,DIRECT\"]",
		"dns: {enable: true, nameserver: [1.1.1.1], nameserver-policy: {'geosite:cn': [1.1.1.1]}, fake-ip-filter: ['geosite:cn'], fallback-filter: {geoip: true, geosite: [cn]}}",
	} {
		report, err := d.CheckReport(context.Background(), []byte(input))
		if err != nil || len(report.Items) == 0 {
			t.Fatal("native geographic coverage failed", err)
		}
	}
	broken := Definition{Native: n, GeoDB: map[string][]byte{"Country.mmdb": []byte("broken")}}
	if broken.Identity() == d.Identity() {
		t.Fatal("database identity omitted")
	}
	if err := broken.Check(context.Background(), original); err == nil {
		t.Fatal("corrupt supplied data silently downgraded")
	}
}
