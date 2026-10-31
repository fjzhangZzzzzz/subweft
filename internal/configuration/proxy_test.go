package configuration

import (
	"go.yaml.in/yaml/v3"
	"testing"
)

func TestSubscriptionProxyRoundTrip(t *testing.T) {
	for _, proxy := range []string{"", "DIRECT", "REJECT", "route"} {
		t.Run("proxy="+proxy, func(t *testing.T) {
			c := Config{Fallback: "DIRECT", Subscriptions: []Subscription{{ID: "source", Name: "Source", URL: "https://example.com/sub", Pool: "quality", Enabled: true, Interval: 3600, Proxy: proxy}}, Groups: []Group{{ID: "route", Name: "Route", Type: "select", Builtins: []string{"DIRECT"}}}}
			data, err := Render(c, false)
			if err != nil {
				t.Fatal(err)
			}
			check := func(data []byte) {
				t.Helper()
				var root map[string]any
				if err := yaml.Unmarshal(data, &root); err != nil {
					t.Fatal(err)
				}
				provider := root["proxy-providers"].(map[string]any)["Source"].(map[string]any)
				value, present := provider["proxy"]
				if proxy == "" {
					if present {
						t.Fatal("unspecified proxy must be omitted")
					}
					return
				}
				expected := proxy
				if proxy == "route" {
					expected = "Route"
				}
				if value != expected {
					t.Fatal("explicit routing changed")
				}
			}
			check(data)
			imported, err := ImportPreview(string(data))
			if err != nil {
				t.Fatal(err)
			}
			data, err = Render(imported, false)
			if err != nil {
				t.Fatal(err)
			}
			check(data)
			c.Subscriptions[0].Proxy = "missing"
			if _, err := Render(c, false); err == nil {
				t.Fatal("unknown explicit proxy accepted")
			}
			c.Groups[0].Builtins = nil
			c.Groups[0].Pools = []string{"quality"}
			c.Subscriptions[0].Proxy = "route"
			if _, err := Render(c, false); err == nil {
				t.Fatal("explicit circular proxy dependency accepted")
			}
		})
	}
}
