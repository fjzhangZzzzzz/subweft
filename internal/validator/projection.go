package validator

import (
	"strings"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

// remoteValidationData exists only in the private validator directory. It is never
// returned by compilation or persisted as a release asset.
func remoteValidationData(kind string, provider map[string]any) ([]byte, error) {
	if kind == "proxy-providers" {
		return []byte("proxies:\n  - {name: validation-only, type: socks5, server: 192.0.2.1, port: 1080}\n"), nil
	}
	var entry string
	switch provider["behavior"] {
	case "domain":
		entry = "example.invalid"
	case "ipcidr":
		entry = "192.0.2.0/24"
	case "classical":
		entry = "DOMAIN,example.invalid"
	default:
		return nil, domain.Invalid("invalid_behavior", "publish")
	}
	switch provider["format"] {
	case "text":
		return []byte(entry + "\n"), nil
	case "yaml":
		return []byte("payload:\n  - " + entry + "\n"), nil
	default:
		return nil, domain.Invalid("invalid_format", "publish")
	}
}

// External geographic classifications cannot be checked offline. Defer only
// dependent rule/DNS values; local geographic assets are still checked natively.
func deferRemoteGeo(config map[string]any, remote map[string]bool) {
	dependent := func(value string) bool {
		if remote["geosite"] && strings.Contains(strings.ToLower(value), "geosite") {
			return true
		}
		if (remote["geoip"] || remote["mmdb"]) && strings.Contains(strings.ToLower(value), "geoip") {
			return true
		}
		return remote["asn"] && strings.Contains(strings.ToLower(value), "ip-asn")
	}
	if rules, ok := config["rules"].([]any); ok {
		filtered := []any{}
		for _, raw := range rules {
			s, _ := raw.(string)
			if !dependent(s) {
				filtered = append(filtered, raw)
			}
		}
		config["rules"] = filtered
	}
	var walk func(map[string]any)
	walk = func(m map[string]any) {
		for k, v := range m {
			if dependent(k) {
				delete(m, k)
				continue
			}
			switch value := v.(type) {
			case map[string]any:
				walk(value)
			case []any:
				out := []any{}
				for _, item := range value {
					if s, ok := item.(string); !ok || !dependent(s) {
						out = append(out, item)
					}
				}
				m[k] = out
			case string:
				if dependent(value) {
					delete(m, k)
				}
			}
		}
	}
	if dns, ok := config["dns"].(map[string]any); ok {
		walk(dns)
	}
	config["geo-auto-update"] = false
}
