package configuration

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"go.yaml.in/yaml/v3"
)

type Problem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func validName(s string) bool {
	return s != "" && len(s) <= 240 && strings.TrimSpace(s) == s && !strings.ContainsAny(s, ",\r\n") && s != "DIRECT" && s != "REJECT" && s != "GLOBAL" && !strings.ContainsFunc(s, unicode.IsControl)
}
func validURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && len(s) <= 8192 && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.Fragment == "" && !strings.ContainsAny(s, "\r\n")
}
func domainValue(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	for _, p := range strings.Split(s, ".") {
		if len(p) == 0 || len(p) > 63 || p[0] == '-' || p[len(p)-1] == '-' {
			return false
		}
		for _, r := range p {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}
func EntryValid(t, v string) bool {
	switch t {
	case "domain", "suffix":
		return domainValue(v)
	case "ip":
		_, e := netip.ParsePrefix(v)
		return e == nil
	}
	return false
}
func interval(n int) bool { return n >= 60 && n <= 604800 }

// ValidateFields permits incomplete relationships in the draft, never malformed values.
func ValidateFields(c Config) error {
	if len(c.Subscriptions) > 100 || len(c.Groups) > 100 || len(c.RuleSets) > 300 || len(c.Rules) > 5000 {
		return domain.Invalid("configuration_limit", "config")
	}
	ids := map[string]bool{}
	names := map[string]bool{}
	object := func(id, name, kind string) error {
		if !identifier.MatchString(id) || ids[id] {
			return domain.Invalid("invalid_or_duplicate_id", kind)
		}
		ids[id] = true
		if !validName(name) || names[kind+name] {
			return domain.Invalid("invalid_or_duplicate_name", kind+".name")
		}
		names[kind+name] = true
		return nil
	}
	for _, s := range c.Subscriptions {
		if e := object(s.ID, s.Name, "subscriptions"); e != nil {
			return e
		}
		if !validURL(s.URL) {
			return domain.Invalid("invalid_url", "subscriptions.url")
		}
		if s.Pool != "quality" && s.Pool != "quota" {
			return domain.Invalid("invalid_pool", "subscriptions.pool")
		}
		if !interval(s.Interval) || len(s.Prefix) > 240 || strings.ContainsFunc(s.Prefix, unicode.IsControl) {
			return domain.Invalid("invalid_provider_options", "subscriptions")
		}
	}
	for _, g := range c.Groups {
		if e := object(g.ID, g.Name, "groups"); e != nil {
			return e
		}
		if g.Type != "select" && g.Type != "url-test" {
			return domain.Invalid("unsupported_group_type", "groups.type")
		}
		if len(g.Pools) > 2 || len(g.Builtins) > 2 {
			return domain.Invalid("invalid_members", "groups")
		}
		seen := map[string]bool{}
		for _, p := range g.Pools {
			if (p != "quality" && p != "quota") || seen[p] {
				return domain.Invalid("invalid_pool", "groups.pools")
			}
			seen[p] = true
		}
		for _, b := range g.Builtins {
			if (b != "DIRECT" && b != "REJECT") || seen[b] {
				return domain.Invalid("invalid_builtin", "groups.builtins")
			}
			seen[b] = true
		}
		if g.Type == "url-test" && (!validURL(g.URL) || g.Interval < 30 || g.Interval > 86400 || g.Tolerance < 0 || g.Tolerance > 60000) {
			return domain.Invalid("invalid_test_options", "groups")
		}
	}
	total := 0
	for _, s := range c.RuleSets {
		if e := object(s.ID, s.Name, "ruleSets"); e != nil {
			return e
		}
		if s.Kind == "http" {
			if len(s.Entries) > 0 || !validURL(s.URL) || !interval(s.Interval) || (s.Behavior != "domain" && s.Behavior != "ipcidr") {
				return domain.Invalid("invalid_remote_ruleset", "ruleSets")
			}
		} else if s.Kind == "local" {
			if s.URL != "" || s.Behavior != "" || s.Interval != 0 {
				return domain.Invalid("unexpected_remote_fields", "ruleSets")
			}
			seen := map[string]bool{}
			for _, e := range s.Entries {
				total++
				key := e.Type + "|" + strings.ToLower(e.Value)
				if !identifier.MatchString(e.ID) || ids[e.ID] || !EntryValid(e.Type, e.Value) || seen[key] {
					return domain.Invalid("invalid_or_duplicate_entry", "ruleSets.entries")
				}
				ids[e.ID] = true
				seen[key] = true
			}
		} else {
			return domain.Invalid("unsupported_ruleset_kind", "ruleSets.kind")
		}
	}
	if total > 50000 {
		return domain.Invalid("entry_limit", "ruleSets.entries")
	}
	for _, r := range c.Rules {
		if !identifier.MatchString(r.ID) || ids[r.ID] {
			return domain.Invalid("invalid_or_duplicate_id", "rules")
		}
		ids[r.ID] = true
		valid := false
		switch r.Type {
		case "RULE-SET":
			valid = len(r.Value) <= 80
		case "DOMAIN", "DOMAIN-SUFFIX":
			valid = domainValue(r.Value)
		case "DOMAIN-KEYWORD":
			valid = r.Value != "" && len(r.Value) <= 253 && !strings.ContainsAny(r.Value, ", \r\n\t")
		case "IP-CIDR":
			valid = EntryValid("ip", r.Value)
		case "GEOIP":
			valid = regexp.MustCompile(`^[A-Z]{2}$`).MatchString(r.Value)
		}
		if !valid {
			return domain.Invalid("invalid_rule", "rules.value")
		}
		if r.NoResolve && r.Type != "RULE-SET" && r.Type != "IP-CIDR" && r.Type != "GEOIP" {
			return domain.Invalid("invalid_no_resolve", "rules.noResolve")
		}
	}
	return nil
}

func Problems(c Config) []Problem {
	out := []Problem{}
	add := func(field, message string) { out = append(out, Problem{field, message}) }
	targets := map[string]string{"DIRECT": "DIRECT", "REJECT": "REJECT"}
	sets := map[string]RuleSet{}
	generated := map[string]bool{}
	for _, g := range c.Groups {
		targets[g.ID] = g.Name
		members := len(g.Builtins)
		for _, s := range c.Subscriptions {
			if s.Enabled && slices.Contains(g.Pools, s.Pool) {
				members++
			}
		}
		if members == 0 {
			add("groups."+g.ID, g.Name+" 没有启用的来源或成员")
		}
	}
	for _, s := range c.Subscriptions {
		if s.Enabled && s.Proxy != "" && targets[s.Proxy] == "" {
			add("subscriptions."+s.ID, s.Name+" 的拉取代理不存在")
		}
	}
	// A provider must never require a group that depends on that provider to fetch itself.
	var visit func(string, map[string]bool) bool
	visit = func(id string, path map[string]bool) bool {
		if path[id] {
			return true
		}
		path[id] = true
		defer delete(path, id)
		for _, g := range c.Groups {
			if g.ID == id {
				for _, s := range c.Subscriptions {
					if s.Enabled && slices.Contains(g.Pools, s.Pool) && s.Proxy != "" && s.Proxy != "DIRECT" && s.Proxy != "REJECT" && visit(s.Proxy, path) {
						return true
					}
				}
			}
		}
		return false
	}
	for _, g := range c.Groups {
		if visit(g.ID, map[string]bool{}) {
			add("groups."+g.ID, g.Name+" 的订阅拉取代理存在循环依赖")
		}
	}
	for _, s := range c.RuleSets {
		sets[s.ID] = s
		for _, n := range providerNames(s) {
			if generated[n] {
				add("ruleSets."+s.ID, "生成的规则集名称重复："+n)
			}
			generated[n] = true
		}
	}
	for _, r := range c.Rules {
		if targets[r.Target] == "" {
			add("rules."+r.ID, "目标策略不存在")
		}
		if r.Type == "RULE-SET" {
			s, ok := sets[r.Value]
			if !ok || len(providerNames(s)) == 0 {
				add("rules."+r.ID, "引用的规则集不存在或没有条目")
			}
		}
	}
	if targets[c.Fallback] == "" {
		add("fallback", "请选择兜底目标策略")
	}
	return out
}
func providerNames(s RuleSet) []string {
	if s.Kind == "http" {
		return []string{s.Name}
	}
	d, ip := false, false
	for _, e := range s.Entries {
		if e.Type == "ip" {
			ip = true
		} else {
			d = true
		}
	}
	out := []string{}
	if d {
		out = append(out, s.Name+"-domain")
	}
	if ip {
		out = append(out, s.Name+"-ipcidr")
	}
	return out
}
func Clone(c Config) Config {
	b, _ := json.Marshal(c)
	var out Config
	_ = json.Unmarshal(b, &out)
	return out
}
func Default() Config {
	return Config{Subscriptions: []Subscription{}, Groups: []Group{}, RuleSets: []RuleSet{}, Rules: []Rule{}, Fallback: "DIRECT", Base: map[string]any{"mixed-port": 7890, "allow-lan": false, "mode": "rule", "dns": map[string]any{"enable": true, "enhanced-mode": "fake-ip", "nameserver": []any{"https://dns.alidns.com/dns-query"}}}}
}

// RenderDiff marks changed secrets without exposing them or a reusable fingerprint.
func RenderDiff(c, other Config, side string) ([]byte, error) {
	b, err := Render(c, true)
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err = yaml.Unmarshal(b, &document); err != nil {
		return nil, err
	}
	mark := func(section, name string) {
		providers, _ := document[section].(map[string]any)
		provider, _ := providers[name].(map[string]any)
		if provider != nil {
			provider["url"] = "[地址已修改·" + side + "·内容已隐藏]"
		}
	}
	for _, s := range c.Subscriptions {
		for _, o := range other.Subscriptions {
			if s.ID == o.ID && s.URL != o.URL {
				mark("proxy-providers", s.Name)
			}
		}
	}
	for _, s := range c.RuleSets {
		for _, o := range other.RuleSets {
			if s.ID == o.ID && s.URL != o.URL {
				mark("rule-providers", s.Name)
			}
		}
	}
	return yaml.Marshal(document)
}

// Render preserves routing order and uses human-readable provider keys. It does no I/O.
func Render(c Config, redact bool) ([]byte, error) {
	if e := ValidateFields(c); e != nil {
		return nil, e
	}
	if len(Problems(c)) > 0 {
		return nil, domain.Invalid("configuration_incomplete", "config")
	}
	c = Clone(c)
	m := c.Base
	if m == nil {
		m = map[string]any{}
	}
	targets := map[string]string{"DIRECT": "DIRECT", "REJECT": "REJECT"}
	for _, g := range c.Groups {
		targets[g.ID] = g.Name
	}
	providers := map[string]any{}
	groups := []any{}
	ruleProviders := map[string]any{}
	sets := map[string]RuleSet{}
	for _, s := range c.Subscriptions {
		if !s.Enabled {
			continue
		}
		u := s.URL
		if redact {
			u = "[订阅地址已隐藏]"
		}
		providers[s.Name] = map[string]any{"type": "http", "url": u, "path": "./proxy_providers/" + s.ID + ".yaml", "interval": s.Interval, "override": map[string]any{"additional-prefix": s.Prefix}, "health-check": map[string]any{"enable": s.HealthCheck, "url": "https://www.gstatic.com/generate_204", "interval": 300}}
		if s.Proxy != "" {
			providers[s.Name].(map[string]any)["proxy"] = targets[s.Proxy]
		}
	}
	for _, g := range c.Groups {
		item := map[string]any{"name": g.Name, "type": g.Type}
		use := []string{}
		for _, s := range c.Subscriptions {
			if s.Enabled && slices.Contains(g.Pools, s.Pool) {
				use = append(use, s.Name)
			}
		}
		if len(use) > 0 {
			item["use"] = use
		}
		if len(g.Builtins) > 0 {
			item["proxies"] = g.Builtins
		}
		if g.Type == "url-test" {
			item["url"] = g.URL
			item["interval"] = g.Interval
			item["tolerance"] = g.Tolerance
		}
		groups = append(groups, item)
	}
	for _, s := range c.RuleSets {
		sets[s.ID] = s
		if s.Kind == "http" {
			u := s.URL
			if redact {
				u = "[远程规则集地址已隐藏]"
			}
			ruleProviders[s.Name] = map[string]any{"type": "http", "url": u, "path": "./rule_providers/" + s.ID + ".yaml", "interval": s.Interval, "behavior": s.Behavior, "format": "yaml"}
			continue
		}
		for _, behavior := range []string{"domain", "ipcidr"} {
			payload := []string{}
			for _, e := range s.Entries {
				if (behavior == "ipcidr") != (e.Type == "ip") {
					continue
				}
				v := e.Value
				if e.Type == "suffix" {
					v = "+." + v
				}
				payload = append(payload, v)
			}
			if len(payload) > 0 {
				ruleProviders[s.Name+"-"+behavior] = map[string]any{"type": "inline", "behavior": behavior, "payload": payload}
			}
		}
	}
	rules := []string{}
	for _, r := range c.Rules {
		if r.Type == "RULE-SET" {
			s := sets[r.Value]
			for _, n := range providerNames(s) {
				line := "RULE-SET," + n + "," + targets[r.Target]
				ip := s.Kind == "http" && s.Behavior == "ipcidr" || s.Kind == "local" && n == s.Name+"-ipcidr"
				if r.NoResolve && ip {
					line += ",no-resolve"
				}
				rules = append(rules, line)
			}
		} else {
			line := fmt.Sprintf("%s,%s,%s", r.Type, r.Value, targets[r.Target])
			if r.NoResolve {
				line += ",no-resolve"
			}
			rules = append(rules, line)
		}
	}
	rules = append(rules, "MATCH,"+targets[c.Fallback])
	m["proxy-providers"] = providers
	m["proxy-groups"] = groups
	m["rule-providers"] = ruleProviders
	m["rules"] = rules
	return yaml.Marshal(m)
}
