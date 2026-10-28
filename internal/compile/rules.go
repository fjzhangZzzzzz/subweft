package compile

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

func RuleCondition(c domain.Condition, depth int) (string, error) {
	if depth > 8 {
		return "", domain.Invalid("rule_too_deep", "conditions")
	}
	if c.Type == "AND" || c.Type == "OR" || c.Type == "NOT" {
		if c.Value != "" || c.NoResolve || len(c.Children) == 0 || len(c.Children) > 32 || c.Type == "NOT" && len(c.Children) != 1 {
			return "", domain.Invalid("invalid_logical_rule", "conditions")
		}
		parts := []string{}
		for _, child := range c.Children {
			v, e := RuleCondition(child, depth+1)
			if e != nil {
				return "", e
			}
			parts = append(parts, "("+v+")")
		}
		return c.Type + ",(" + strings.Join(parts, ",") + ")", nil
	}
	if len(c.Children) > 0 || len(c.Value) > 2048 || strings.ContainsAny(c.Value, ",\r\n\x00()") {
		return "", domain.Invalid("invalid_rule_value", "value")
	}
	switch c.Type {
	case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "GEOSITE", "GEOIP", "IP-ASN", "SRC-IP-ASN", "SRC-GEOIP", "PROCESS-NAME", "PROCESS-PATH", "PROCESS-NAME-WILDCARD", "PROCESS-PATH-WILDCARD", "IN-NAME", "IN-USER", "IN-TYPE", "NETWORK", "DSCP":
		if c.Value == "" {
			return "", domain.Invalid("invalid_rule_value", "value")
		}
	case "DOMAIN-REGEX", "PROCESS-NAME-REGEX", "PROCESS-PATH-REGEX":
		if _, e := regexp.Compile(c.Value); e != nil || c.Value == "" {
			return "", domain.Invalid("invalid_rule_value", "value")
		}
	case "IP-CIDR", "IP-CIDR6", "SRC-IP-CIDR", "SRC-IP-CIDR6":
		p, e := netip.ParsePrefix(c.Value)
		if e != nil || strings.HasSuffix(c.Type, "6") != p.Addr().Is6() {
			return "", domain.Invalid("invalid_cidr", "value")
		}
	case "DST-PORT", "SRC-PORT", "IN-PORT":
		for _, part := range strings.Split(c.Value, "/") {
			pair := strings.Split(part, "-")
			if len(pair) > 2 {
				return "", domain.Invalid("invalid_rule_value", "value")
			}
			last := 0
			for _, v := range pair {
				n, e := strconv.Atoi(v)
				if e != nil || n < 1 || n > 65535 || n < last {
					return "", domain.Invalid("invalid_rule_value", "value")
				}
				last = n
			}
		}
	case "MATCH":
		if c.Value != "" || c.NoResolve {
			return "", domain.Invalid("invalid_match", "value")
		}
		return "MATCH", nil
	default:
		return "", domain.Invalid("unsupported_rule_type", "type")
	}
	if c.NoResolve && !strings.Contains(c.Type, "IP") && c.Type != "GEOIP" {
		return "", domain.Invalid("no_resolve_not_applicable", "noResolve")
	}
	s := c.Type + "," + c.Value
	if c.NoResolve {
		s += ",no-resolve"
	}
	return s, nil
}
func renderRules(p domain.Profile, names map[string]bool, resources map[string]domain.Resource) ([]string, map[string]string, error) {
	out := []string{}
	used := map[string]string{}
	match := false
	for i, r := range p.Rules {
		if !r.Enabled {
			continue
		}
		field := fmt.Sprintf("rules.%d", i)
		if match {
			return nil, nil, domain.Invalid("match_must_be_last", field)
		}
		if !names[r.Target] {
			return nil, nil, domain.Invalid("rule_target_missing", field+".target")
		}
		var line string
		if r.Type == "RULE-SET" {
			resource, ok := resources[r.Value]
			if !ok || resource.Type != "ruleset" || !resource.Enabled || resource.Digest == "" {
				return nil, nil, domain.Invalid("ruleset_unavailable", field+".value")
			}
			used[r.Value] = resource.Digest
			line = "RULE-SET," + r.Value
		} else {
			var e error
			line, e = RuleCondition(domain.Condition{Type: r.Type, Value: r.Value, NoResolve: false, Children: r.Conditions}, 0)
			if e != nil {
				return nil, nil, domain.Field(field, e)
			}
		}
		if r.NoResolve && r.Type != "RULE-SET" {
			if _, e := RuleCondition(domain.Condition{Type: r.Type, Value: r.Value, NoResolve: true, Children: r.Conditions}, 0); e != nil {
				return nil, nil, domain.Field(field, e)
			}
		}
		line += "," + r.Target
		if r.NoResolve {
			line += ",no-resolve"
		}
		out = append(out, line)
		match = r.Type == "MATCH"
	}
	if !match {
		return nil, nil, domain.Invalid("match_required", "rules")
	}
	return out, used, nil
}
