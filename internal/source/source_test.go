package source_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
)

func TestStrictParsing(t *testing.T) {
	nodes, e := source.Parse([]byte(testfixture.ProviderYAML))
	if e != nil || len(nodes) != 2 {
		t.Fatal("valid fixture not parsed", e)
	}
	cases := map[string]string{"unknown field": strings.Replace(testfixture.ProviderYAML, "udp: true", "mystery: true", 1), "unknown protocol": strings.Replace(testfixture.ProviderYAML, "type: ss", "type: vmess", 1), "empty": "proxies: []", "extra root": testfixture.ProviderYAML + "dns: {}\n", "second document": testfixture.ProviderYAML + "---\nproxies: []", "alias": "proxies: &p [*p]", "duplicate name": strings.Replace(testfixture.ProviderYAML, "QUOTA-01", "HQ-01", 1)}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, e := source.Parse([]byte(input)); e == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
	uri := "ss://YWVzLTEyOC1nY206Zml4dHVyZQ@192.0.2.1:443#sample"
	if parsed, e := source.Parse([]byte(uri)); e != nil || len(parsed) != 1 || parsed[0].Password != "fixture" {
		t.Fatal("SIP002 URI not parsed", e)
	}
}
func TestFetchPolicyAndRedirectRevalidation(t *testing.T) {
	ctx := context.Background()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(testfixture.ProviderYAML)) }))
	defer target.Close()
	if _, e := source.NewFetcher(nil).Fetch(ctx, target.URL); e == nil {
		t.Fatal("loopback SSRF accepted")
	}
	allowed := source.NewFetcher([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	if _, e := allowed.Fetch(ctx, target.URL); e != nil {
		t.Fatal("explicit loopback fixture rejected", e)
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "http://[::1]:9/private", 302) }))
	defer redirect.Close()
	if _, e := allowed.Fetch(ctx, redirect.URL); e == nil {
		t.Fatal("redirect to non-allowed loopback accepted")
	}
	for _, url := range []string{"file:///etc/passwd", "http://user:password@example.com", "https://example.com/#fragment"} {
		if source.ValidateURL(url) == nil {
			t.Fatal("unsafe URL accepted")
		}
	}
}

func TestScalarTypesAreNotCoerced(t *testing.T) {
	for _, input := range []string{strings.Replace(testfixture.ProviderYAML, "port: 443", "port: 443.5", 1), strings.Replace(testfixture.ProviderYAML, "password: fixture-proxy-password", "password: 12345", 1), strings.Replace(testfixture.ProviderYAML, "udp: true", "udp: 'true'", 1)} {
		if _, err := source.Parse([]byte(input)); err == nil {
			t.Fatal("invalid scalar type was coerced")
		}
	}
}
