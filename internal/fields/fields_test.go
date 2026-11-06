package fields_test

import (
	"encoding/json"
	"github.com/fjzhangZzzzzz/subweft/internal/fields"
	"strings"
	"testing"
)

func TestPinnedContractUsesRenderableTypesAndMarksSecrets(t *testing.T) {
	var catalog map[string]any
	if e := json.Unmarshal(fields.Catalog, &catalog); e != nil {
		t.Fatal(e)
	}
	allowed := map[string]bool{"string": true, "integer": true, "number": true, "boolean": true, "object": true, "array": true, "map": true, "union": true, "any": true}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if kind, ok := x["kind"].(string); ok && !allowed[kind] {
				t.Fatal("unrenderable schema kind")
			}
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(catalog)
	v := map[string]any{"type": "ssr", "password": "synthetic-password", "protocol-param": "synthetic-protocol-secret", "obfs-param": "synthetic-obfs-secret"}
	redacted, _ := json.Marshal(fields.Redact("ssr", v))
	for _, secret := range []string{"synthetic-password", "synthetic-protocol-secret", "synthetic-obfs-secret"} {
		if strings.Contains(string(redacted), secret) {
			t.Fatal("extended protocol secret exposed")
		}
	}
}
func TestHTTPHeadersRejectCaseDuplicatesAndLineBreaks(t *testing.T) {
	for _, headers := range []map[string]any{{"Host": "example.invalid", "host": "other.invalid"}, {"Authorization": "value\r\nInjected: value"}} {
		v := map[string]any{"type": "http", "name": "fixture", "server": "192.0.2.1", "port": 443, "headers": headers}
		if fields.Validate("http", v) == nil {
			t.Fatal("unsafe HTTP headers accepted")
		}
	}
}
