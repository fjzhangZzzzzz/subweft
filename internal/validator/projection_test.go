package validator

import (
	"reflect"
	"strings"
	"testing"
)

func TestGeoProjectionOnlyDefersTypedClassifications(t *testing.T) {
	remote := map[string]bool{"geosite": true, "mmdb": true}
	for line, want := range map[string]bool{
		"DOMAIN,geosite.example.invalid,DIRECT":                       false,
		"DOMAIN-SUFFIX,geoip.example.invalid,DIRECT":                  false,
		"GEOSITE,cn,DIRECT":                                           true,
		"GEOIP,CN,DIRECT":                                             true,
		"AND,((DOMAIN,example.invalid),(GEOSITE,cn)),DIRECT":          true,
		"AND,((DOMAIN,geosite.example.invalid),(NETWORK,TCP)),DIRECT": false,
	} {
		if got := remoteGeoRule(line, remote); got != want {
			t.Errorf("typed geographic rule dependency differs for %s", line)
		}
	}
	servers := []any{"1.1.1.1", "bogus://geoip.example.invalid", "https://geosite.example.invalid/dns-query"}
	config := map[string]any{"dns": map[string]any{"nameserver": servers, "nameserver-policy": map[string]any{"geosite:cn": []any{"1.1.1.1"}, "geoip.example.invalid": servers}, "fake-ip-filter": []any{"geosite:cn", "geosite.example.invalid"}}}
	deferRemoteGeo(config, remote)
	dns := config["dns"].(map[string]any)
	if !reflect.DeepEqual(dns["nameserver"], servers) {
		t.Fatal("ordinary DNS values were modified")
	}
	policy := dns["nameserver-policy"].(map[string]any)
	if _, ok := policy["geosite:cn"]; ok {
		t.Fatal("classification not deferred")
	}
	if !reflect.DeepEqual(policy["geoip.example.invalid"], servers) {
		t.Fatal("ordinary DNS policy removed")
	}
}
func TestGeoProjectionPreservesSiblingsAndPolicyResolvers(t *testing.T) {
	remote := map[string]bool{"geosite": true}
	line := "AND,((GEOSITE,cn),(NETWORK,invalid)),DIRECT"
	want := "AND,((DOMAIN,validation-only.invalid),(NETWORK,invalid)),DIRECT"
	if got := projectGeoRule(line, remote); got != want {
		t.Fatalf("sibling changed: %s", got)
	}
	servers := []any{"bogus://resolver.example.invalid"}
	config := map[string]any{"dns": map[string]any{"nameserver-policy": map[string]any{"geosite:cn": servers}}}
	deferRemoteGeo(config, remote)
	policies := config["dns"].(map[string]any)["nameserver-policy"].(map[string]any)
	if len(policies) != 1 {
		t.Fatal("policy lost")
	}
	for _, v := range policies {
		if !reflect.DeepEqual(v, servers) {
			t.Fatal("resolver lost")
		}
	}
	for _, format := range []string{"yaml", "text"} {
		data := []byte("GEOSITE,cn\nDOMAIN,DIRECT.example.invalid\nGEOIP,CN,no-resolve\n")
		if format == "yaml" {
			data = []byte("payload:\n - GEOSITE,cn\n - DOMAIN,DIRECT.example.invalid\n - GEOIP,CN,no-resolve\n")
		}
		projected, e := projectClassicalRules(data, format, remote)
		if e != nil || !strings.Contains(string(projected), "DOMAIN,validation-only.invalid") || !strings.Contains(string(projected), "DOMAIN,DIRECT.example.invalid") || !strings.Contains(string(projected), "GEOIP,CN,no-resolve") {
			t.Fatal("local payload projection changed unrelated conditions")
		}
	}
}
