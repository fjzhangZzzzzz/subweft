package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"go.yaml.in/yaml/v3"
	"os"
	"testing"
)

func dnsPreset(t *testing.T) map[string]any {
	t.Helper()
	b, e := os.ReadFile("../../web/src/dns-preset.json")
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]any
	if e = json.Unmarshal(b, &d); e != nil {
		t.Fatal(e)
	}
	return d
}
func TestDNSEditPreservesImportedFieldsAndRelease(t *testing.T) {
	ctx := context.Background()
	s, _, _ := setup(t, &checker{})
	d, _, e := s.Read(ctx)
	if e != nil {
		t.Fatal(e)
	}
	// Imported advanced values remain opaque and read-only unless exposed by the form.
	dns := d.Draft.Config.Base["dns"].(map[string]any)
	dns["use-system-hosts"] = false
	dns["nameserver-policy"] = map[string]any{"example.com": "1.1.1.1"}
	dns["fallback-filter"] = map[string]any{"geosite": []any{"cn"}}
	if e = storage.Save(ctx, s.Store, "configuration", &d.Meta, &d, ""); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Check(ctx, d.Draft.Revision); e != nil {
		t.Fatal(e)
	}
	first, e := s.Publish(ctx, d.Draft.Revision, "before DNS edit")
	if e != nil {
		t.Fatal(e)
	}
	edit := Draft{Revision: d.Draft.Revision, Config: Clone(d.Draft.Config)}
	dns = edit.Config.Base["dns"].(map[string]any)
	dns["enable"] = false
	saved, e := s.Save(ctx, edit)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Publish(ctx, saved.Revision, ""); e == nil {
		t.Fatal("DNS edit reused stale check")
	}
	current, check, e := s.Read(ctx)
	if e != nil || check != nil || current.Active != first.Number {
		t.Fatal("DNS save changed active release or retained check")
	}
	if !same(current.Releases[0].Config, d.Draft.Config) {
		t.Fatal("immutable release changed")
	}
	actual := current.Draft.Config.Base["dns"].(map[string]any)
	if actual["use-system-hosts"] != false || !same(actual["nameserver-policy"], dns["nameserver-policy"]) {
		t.Fatal("imported values lost")
	}
	for _, mutate := range []func(map[string]any){
		func(b map[string]any) { b["mixed-port"] = 9999 },
		func(b map[string]any) { b["dns"].(map[string]any)["listen"] = "0.0.0.0:53" },
		func(b map[string]any) { b["dns"].(map[string]any)["use-system-hosts"] = true },
		func(b map[string]any) {
			b["dns"].(map[string]any)["fallback-filter"].(map[string]any)["geosite"] = []any{"gfw"}
		},
	} {
		bad := Draft{Revision: saved.Revision, Config: Clone(saved.Config)}
		mutate(bad.Config.Base)
		if _, e = s.Save(ctx, bad); e == nil {
			t.Fatal("read-only base mutation accepted")
		}
	}
	if _, e = s.Save(ctx, edit); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale DNS revision accepted")
	}
	if _, e = s.Check(ctx, saved.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Publish(ctx, saved.Revision, "DNS disabled"); e != nil {
		t.Fatal(e)
	}
	restored, e := s.Restore(ctx, saved.Revision, first.Number)
	if e != nil || !same(restored.Config.Base, d.Draft.Config.Base) {
		t.Fatal("DNS restore failed", e)
	}
}
func TestDNSPresetRenderImportAndValidation(t *testing.T) {
	c := Default()
	c.Base["dns"] = dnsPreset(t)
	b, e := Render(c, false)
	if e != nil {
		t.Fatal(e)
	}
	var rendered map[string]any
	if e = yaml.Unmarshal(b, &rendered); e != nil {
		t.Fatal(e)
	}
	if !same(rendered["dns"], c.Base["dns"]) {
		t.Fatal("preset fields changed during render")
	}
	imported, e := ImportPreview(string(b))
	if e != nil || !same(imported.Base["dns"], c.Base["dns"]) {
		t.Fatal("DNS import lost values", e)
	}
	for _, mutate := range []func(map[string]any){
		func(d map[string]any) { d["enable"] = "true" },
		func(d map[string]any) { d["nameserver"] = "1.1.1.1" },
		func(d map[string]any) { d["nameserver"] = []any{true} },
		func(d map[string]any) { d["nameserver"] = []any{""} },
		func(d map[string]any) { d["nameserver-policy"] = map[string]any{"example.com": 123} },
		func(d map[string]any) { d["enhanced-mode"] = "unknown" },
		func(d map[string]any) { d["fake-ip-range"] = "invalid" },
		func(d map[string]any) { d["fallback-filter"].(map[string]any)["geoip"] = "true" },
		func(d map[string]any) { d["fallback-filter"].(map[string]any)["ipcidr"] = []any{"invalid"} },
		func(d map[string]any) { d["fallback-filter"].(map[string]any)["geoip-code"] = "CHINA" },
	} {
		bad := Clone(c)
		mutate(bad.Base["dns"].(map[string]any))
		if e := ValidateFields(bad); e == nil {
			t.Fatal("invalid DNS accepted")
		}
	}
	// A no-op or unrelated edit must not insert default values into imported DNS.
	c.Base["dns"] = map[string]any{"enable": false}
	b, e = Render(c, false)
	if e != nil {
		t.Fatal(e)
	}
	imported, e = ImportPreview(string(b))
	if e != nil || !same(imported.Base["dns"], c.Base["dns"]) {
		t.Fatal("absent DNS fields gained defaults", e)
	}
}
