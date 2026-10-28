package compile_test

import (
	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"strings"
	"testing"
)

func TestFilteredNodesAreAbsentFromDeliveredConfiguration(t *testing.T) {
	p, snap := fixture(t)
	snap.Sources[0].Nodes[0].Password = "synthetic-excluded-credential"
	p.Groups[0].Filter = "QUOTA"
	result, e := compile.Compile(p, snap, false)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(result.Config), "synthetic-excluded-credential") || strings.Contains(string(result.Config), "HQ-01") {
		t.Fatal("unselected node leaked into config")
	}
}
func TestTypeExclusionCannotBecomeImplicitDirectFallback(t *testing.T) {
	p, snap := fixture(t)
	p.Groups[0].Options = map[string]any{"exclude-type": "Shadowsocks"}
	if _, e := compile.Compile(p, snap, false); e == nil {
		t.Fatal("fully excluded group accepted")
	}
	p.Groups[0].Builtins = []string{"DIRECT"}
	result, e := compile.Compile(p, snap, false)
	if e != nil || len(result.Members["Proxy"]) != 1 || result.Members["Proxy"][0] != "DIRECT" {
		t.Fatal("explicit direct member not preserved", e)
	}
	p.Groups[0].Options["default-selected"] = "missing"
	if _, e := compile.Compile(p, snap, false); e == nil {
		t.Fatal("invalid default selection accepted")
	}
}
func TestDeviceURLRewriteNeverTouchesCredentials(t *testing.T) {
	input := []byte("proxies:\n  - name: x\n    type: ss\n    password: https://subweft.invalid/resources/private\nrule-providers:\n  r:\n    type: http\n    url: https://subweft.invalid/resources/r\n")
	output, e := compile.DeviceConfig(input, "https://example.invalid/device/resources/")
	if e != nil || !strings.Contains(string(output), "password: https://subweft.invalid/resources/private") || !strings.Contains(string(output), "url: https://example.invalid/device/resources/r") {
		t.Fatal("resource URL rewrite changed user data")
	}
}
