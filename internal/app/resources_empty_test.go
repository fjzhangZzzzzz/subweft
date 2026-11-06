package app

import "testing"

func TestEmptyRuleSetRequiresExplicitLocalAllowance(t *testing.T) {
	for _, tc := range []struct{ format, content string }{{"yaml", "payload: []\n"}, {"text", ""}} {
		t.Run(tc.format, func(t *testing.T) {
			data, count, err := normalizeRules([]byte(tc.content), "domain", tc.format, true)
			if err != nil || count != 0 || len(data) == 0 {
				t.Fatalf("empty local ruleset: %v", err)
			}
			if _, _, err := normalizeRules([]byte(tc.content), "domain", tc.format, false); err == nil {
				t.Fatal("empty fetched ruleset accepted")
			}
		})
	}
}
