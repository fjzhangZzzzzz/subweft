package validator

import (
	"fmt"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"go.yaml.in/yaml/v3"
	"regexp"
	"sort"
	"strings"
)

type Item struct {
	Field    string `json:"field"`
	Status   string `json:"status"`
	Resource string `json:"resource"`
}
type Report struct {
	Items    []Item `json:"items"`
	Identity string `json:"identity"`
	Digest   string `json:"digest"`
}

// Only the explicitly understood geographic positions may be projected. All
// other fields remain intact for the pinned native checker. No fetching occurs.
func definitionProjection(data []byte, assets map[string][]byte) ([]byte, []Item, error) {
	var m map[string]any
	if yaml.Unmarshal(data, &m) != nil || m == nil {
		return nil, nil, domain.Invalid("invalid_config", "check")
	}
	items := []Item{}
	fail := func() ([]byte, []Item, error) {
		return nil, nil, domain.Invalid("invalid_geographic_configuration", "check")
	}
	ip := "Country.mmdb"
	if mode, exists := m["geodata-mode"]; exists {
		b, ok := mode.(bool)
		if !ok {
			return fail()
		}
		if b {
			ip = "GeoIP.dat"
		}
	}
	dependency := func(field, resource string) bool {
		missing := len(assets[resource]) == 0
		status := "passed"
		if missing {
			status = "unverified"
		}
		items = append(items, Item{field, status, resource})
		return missing
	}
	code := regexp.MustCompile(`^[A-Za-z0-9_-]+(@[A-Za-z0-9_-]+)?$`)
	classification := func(v string) bool { return code.MatchString(v) }
	if rules, ok := m["rules"].([]any); ok {
		for i, raw := range rules {
			line, ok := raw.(string)
			if !ok {
				return fail()
			}
			parts := strings.Split(line, ",")
			if parts[0] != "GEOIP" && parts[0] != "GEOSITE" {
				// Unsupported nested geographic expressions must not evade dependency tracking.
				if strings.Contains(line, "GEOIP,") || strings.Contains(line, "GEOSITE,") || strings.Contains(line, "IP-ASN,") {
					return fail()
				}
				continue
			}
			if len(parts) < 3 || len(parts) > 4 || !classification(parts[1]) || parts[2] == "" {
				return fail()
			}
			if len(parts) == 4 && (parts[0] != "GEOIP" || parts[3] != "no-resolve") {
				return fail()
			}
			resource := ip
			if parts[0] == "GEOSITE" {
				resource = "GeoSite.dat"
			}
			if dependency(fmt.Sprintf("rules[%d]", i), resource) {
				// IP-CIDR retains the optional no-resolve and target checks.
				parts[0] = "IP-CIDR"
				parts[1] = "192.0.2.0/24"
				rules[i] = strings.Join(parts, ",")
			}
		}
	}
	if dns, ok := m["dns"].(map[string]any); ok {
		for _, key := range []string{"nameserver-policy", "proxy-server-nameserver-policy"} {
			if raw, exists := dns[key]; exists {
				policies, ok := raw.(map[string]any)
				if !ok {
					return fail()
				}
				replacements := map[string]any{}
				for pattern, value := range policies {
					if !strings.HasPrefix(pattern, "geosite:") {
						continue
					}
					for _, name := range strings.Split(strings.TrimPrefix(pattern, "geosite:"), ",") {
						if !classification(name) {
							return fail()
						}
					}
					if dependency("dns."+key, "GeoSite.dat") {
						name := fmt.Sprintf("validation-%d.invalid", len(replacements))
						for {
							if _, exists := policies[name]; !exists {
								break
							}
							name = "x-" + name
						}
						replacements[name] = value
						delete(policies, pattern)
					}
				}
				for k, v := range replacements {
					policies[k] = v
				}
			}
		}
		if raw, exists := dns["fake-ip-filter"]; exists {
			values, ok := raw.([]any)
			if !ok {
				return fail()
			}
			for i, raw := range values {
				value, ok := raw.(string)
				if !ok {
					return fail()
				}
				if strings.HasPrefix(value, "geosite:") {
					if !classification(strings.TrimPrefix(value, "geosite:")) {
						return fail()
					}
					if dependency("dns.fake-ip-filter", "GeoSite.dat") {
						values[i] = "validation-only.invalid"
					}
				} else if strings.Contains(value, "GEOSITE,") || strings.Contains(value, "GEOIP,") {
					return fail()
				}
			}
		}
		// Mihomo defaults fallback-filter.geoip to true. Materialize that default
		// only in the private projection, including when the filter is omitted.
		fallback := map[string]any{}
		if raw, exists := dns["fallback-filter"]; exists {
			var ok bool
			fallback, ok = raw.(map[string]any)
			if !ok {
				return fail()
			}
		}
		enabled := true
		if raw, exists := fallback["geoip"]; exists {
			var ok bool
			enabled, ok = raw.(bool)
			if !ok {
				return fail()
			}
		}
		if raw, exists := fallback["geoip-code"]; exists {
			v, ok := raw.(string)
			if !ok || !classification(v) {
				return fail()
			}
		}
		if enabled && dns["enable"] == true {
			if dependency("dns.fallback-filter.geoip", ip) {
				fallback["geoip"] = false
				dns["fallback-filter"] = fallback
			}
		}
		if raw, exists := fallback["geosite"]; exists {
			values, ok := raw.([]any)
			if !ok {
				return fail()
			}
			for _, raw := range values {
				v, ok := raw.(string)
				if !ok || !classification(v) {
					return fail()
				}
			}
			if len(values) > 0 && dependency("dns.fallback-filter.geosite", "GeoSite.dat") {
				delete(fallback, "geosite")
			}
		}
	}
	if raw, exists := m["geo-auto-update"]; exists {
		if _, ok := raw.(bool); !ok {
			return fail()
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Field == items[j].Field {
			return items[i].Resource < items[j].Resource
		}
		return items[i].Field < items[j].Field
	})
	// Updates belong to the client and must never run inside the checker.
	m["geo-auto-update"] = false
	out, err := yaml.Marshal(m)
	return out, items, err
}
