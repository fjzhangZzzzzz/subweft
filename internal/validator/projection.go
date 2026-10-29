package validator

import (
	"fmt"
	"strings"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/migrate"
	"go.yaml.in/yaml/v3"
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
	if rules, ok := config["rules"].([]any); ok {
		for i, raw := range rules {
			line, ok := raw.(string)
			if ok {
				rules[i] = projectGeoRule(line, remote)
			}
		}
	}
	if dns, ok := config["dns"].(map[string]any); ok {
		for _, key := range []string{"nameserver-policy", "proxy-server-nameserver-policy"} {
			if policies, ok := dns[key].(map[string]any); ok {
				replacements := map[string]any{}
				for pattern := range policies {
					if remote["geosite"] && strings.HasPrefix(pattern, "geosite:") {
						name := fmt.Sprintf("validation-only-%d.invalid", len(replacements))
						for policies[name] != nil {
							name = "x-" + name
						}
						replacements[name] = policies[pattern]
						delete(policies, pattern)
					}
				}
				for name, value := range replacements {
					policies[name] = value
				}
			}
		}
		if values, ok := dns["fake-ip-filter"].([]any); ok {
			for i, v := range values {
				pattern, ok := v.(string)
				if ok && remote["geosite"] && strings.HasPrefix(pattern, "geosite:") {
					values[i] = "validation-only.invalid"
				}
			}
		}
		if fallback, ok := dns["fallback-filter"].(map[string]any); ok {
			if remote["geosite"] {
				delete(fallback, "geosite")
			}
			if remote["geoip"] || remote["mmdb"] {
				fallback["geoip"] = false
			}
		}
	}
	config["geo-auto-update"] = false
}

// Replace only the unavailable classification, preserving targets and sibling
// conditions for native validation. The original release is never modified.
func projectGeoRule(line string, remote map[string]bool) string {
	r, e := migrate.ParseRule(line)
	if e != nil || !remoteGeoRule(line, remote) {
		return line
	}
	var project func(domain.Condition) domain.Condition
	project = func(c domain.Condition) domain.Condition {
		if remoteGeoLeaf(c.Type, remote) {
			return domain.Condition{Type: "DOMAIN", Value: "validation-only.invalid"}
		}
		for i := range c.Children {
			c.Children[i] = project(c.Children[i])
		}
		return c
	}
	c := project(domain.Condition{Type: r.Type, Value: r.Value, Children: r.Conditions, NoResolve: r.NoResolve})
	noResolve := c.NoResolve
	c.NoResolve = false
	value, e := compile.RuleCondition(c, 0)
	if e != nil {
		return line
	}
	value += "," + r.Target
	if noResolve {
		value += ",no-resolve"
	}
	return value
}

func remoteGeoLeaf(kind string, remote map[string]bool) bool {
	switch kind {
	case "GEOSITE":
		return remote["geosite"]
	case "GEOIP", "SRC-GEOIP":
		return remote["geoip"] || remote["mmdb"]
	case "IP-ASN", "SRC-IP-ASN":
		return remote["asn"]
	}
	return false
}

func projectClassicalRules(data []byte, format string, remote map[string]bool) ([]byte, error) {
	project := func(line string) string {
		// Classical payloads have no target. Preserve trailing no-resolve.
		line = strings.TrimSpace(line)
		noResolve := strings.HasSuffix(line, ",no-resolve")
		if noResolve {
			line = strings.TrimSuffix(line, ",no-resolve")
		}
		withTarget := line + ",DIRECT"
		if noResolve {
			withTarget += ",no-resolve"
		}
		result := projectGeoRule(withTarget, remote)
		if strings.HasSuffix(result, ",no-resolve") {
			return strings.TrimSuffix(strings.TrimSuffix(result, ",no-resolve"), ",DIRECT") + ",no-resolve"
		}
		return strings.TrimSuffix(result, ",DIRECT")
	}
	if format == "text" {
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
				lines[i] = project(line)
			}
		}
		return []byte(strings.Join(lines, "\n")), nil
	}
	var payload map[string]any
	if e := yaml.Unmarshal(data, &payload); e != nil {
		return nil, e
	}
	if lines, ok := payload["payload"].([]any); ok {
		for i, raw := range lines {
			if line, ok := raw.(string); ok {
				lines[i] = project(line)
			}
		}
	}
	return yaml.Marshal(payload)
}
func remoteGeoRule(line string, remote map[string]bool) bool {
	r, e := migrate.ParseRule(line)
	if e != nil {
		return false
	}
	var dependent func(domain.Condition) bool
	dependent = func(c domain.Condition) bool {
		if remoteGeoLeaf(c.Type, remote) {
			return true
		}
		for _, child := range c.Children {
			if dependent(child) {
				return true
			}
		}
		return false
	}
	return dependent(domain.Condition{Type: r.Type, Value: r.Value, Children: r.Conditions})
}
