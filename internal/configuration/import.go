package configuration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"go.yaml.in/yaml/v3"
)

// ImportPreview accepts only the explicitly supported configuration subset.
// Local files, manual proxies and unknown fields are rejected, never discarded.
func ImportPreview(data string) (Config, error) {
	c := Default()
	if len(data) > 4<<20 {
		return c, domain.Invalid("import_too_large", "import")
	}
	var root map[string]any
	decoder := yaml.NewDecoder(strings.NewReader(data))
	if decoder.Decode(&root) != nil || root == nil {
		return c, domain.Invalid("invalid_yaml", "import")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return c, domain.Invalid("multiple_yaml_documents", "import")
	}
	allowed := map[string]bool{}
	for _, k := range []string{"mixed-port", "port", "socks-port", "redir-port", "tproxy-port", "allow-lan", "bind-address", "ipv6", "mode", "log-level", "unified-delay", "tcp-concurrent", "find-process-mode", "global-client-fingerprint", "dns", "tun", "sniffer", "profile", "routing-mark", "interface-name", "geodata-mode", "geodata-loader", "geo-auto-update", "geo-update-interval", "keep-alive-interval", "keep-alive-idle", "disable-keep-alive", "hosts", "proxy-providers", "proxy-groups", "rule-providers", "rules"} {
		allowed[k] = true
	}
	for k, v := range root {
		if v == nil {
			return c, domain.Invalid("invalid_import_type", k)
		}
		if !allowed[k] {
			return c, domain.Invalid("unsupported_import_field", k)
		}
		if k != "proxy-providers" && k != "proxy-groups" && k != "rule-providers" && k != "rules" {
			c.Base[k] = v
		}
	}
	if e := typedFields(root, map[string]string{
		"mixed-port": "int", "port": "int", "socks-port": "int", "redir-port": "int", "tproxy-port": "int",
		"allow-lan": "bool", "ipv6": "bool", "unified-delay": "bool", "tcp-concurrent": "bool", "geodata-mode": "bool", "geo-auto-update": "bool", "disable-keep-alive": "bool",
		"bind-address": "string", "mode": "string", "log-level": "string", "find-process-mode": "string", "global-client-fingerprint": "string", "geodata-loader": "string", "interface-name": "string",
		"routing-mark": "int", "geo-update-interval": "int", "keep-alive-interval": "int", "keep-alive-idle": "int",
		"dns": "map", "tun": "map", "sniffer": "map", "profile": "map", "hosts": "map",
	}); e != nil {
		return c, e
	}
	if dns, ok := root["dns"].(map[string]any); ok {
		if e := typedFields(dns, map[string]string{"enable": "bool", "enhanced-mode": "string", "nameserver": "list"}); e != nil {
			return c, e
		}
	}
	if mode, ok := c.Base["mode"].(string); !ok || mode != "rule" {
		return c, domain.Invalid("rule_mode_required", "mode")
	}
	// No deployment-owned download URLs or file paths may be hidden in the base.
	raw, _ := json.Marshal(c.Base)
	for _, key := range []string{`"geox-url"`, `"external-ui"`, `"external-controller"`, `"certificate"`, `"private-key"`, `"path"`} {
		if strings.Contains(string(raw), key) {
			return c, domain.Invalid("unsupported_import_field", "base")
		}
	}
	providers, ok := root["proxy-providers"].(map[string]any)
	if root["proxy-providers"] != nil && !ok {
		return c, domain.Invalid("invalid_import", "proxy-providers")
	}
	sourceIDs := map[string]string{}
	for name, raw := range providers {
		p, ok := raw.(map[string]any)
		if !ok {
			return c, domain.Invalid("invalid_import", "proxy-providers")
		}
		if e := keys(p, "type", "url", "interval", "path", "proxy", "health-check", "override"); e != nil {
			return c, e
		}
		if e := typedFields(p, map[string]string{"type": "string", "url": "string", "path": "string", "proxy": "string", "interval": "int"}); e != nil {
			return c, e
		}
		if p["type"] != "http" {
			return c, domain.Invalid("http_provider_required", "proxy-providers")
		}
		s := Subscription{ID: domain.NewID(), Name: name, Pool: "quality", Enabled: true, Interval: 3600}
		s.URL, _ = p["url"].(string)
		if x, ok := p["interval"]; ok {
			n, ok := x.(int)
			if !ok {
				return c, domain.Invalid("invalid_import", "interval")
			}
			s.Interval = n
		}
		if x, ok := p["proxy"].(string); ok {
			s.Proxy = x
		}
		if o, exists := p["override"]; exists {
			v, ok := o.(map[string]any)
			if !ok {
				return c, domain.Invalid("invalid_import", "override")
			}
			if e := keys(v, "additional-prefix"); e != nil {
				return c, e
			}
			if e := typedFields(v, map[string]string{"additional-prefix": "string"}); e != nil {
				return c, e
			}
			s.Prefix, _ = v["additional-prefix"].(string)
		}
		if h, exists := p["health-check"]; exists {
			v, ok := h.(map[string]any)
			if !ok {
				return c, domain.Invalid("invalid_import", "health-check")
			}
			if e := keys(v, "enable", "url", "interval"); e != nil {
				return c, e
			}
			if e := typedFields(v, map[string]string{"enable": "bool", "url": "string", "interval": "int"}); e != nil {
				return c, e
			}
			s.HealthCheck, _ = v["enable"].(bool)
			if v["url"] != nil && v["url"] != "https://www.gstatic.com/generate_204" {
				return c, domain.Invalid("unsupported_health_check_url", "health-check.url")
			}
			if v["interval"] != nil && v["interval"] != 300 {
				return c, domain.Invalid("unsupported_health_check_interval", "health-check.interval")
			}
		}
		sourceIDs[name] = s.ID
		c.Subscriptions = append(c.Subscriptions, s)
	}
	// Infer at most two pools from identical subscription membership across groups.
	sort.Slice(c.Subscriptions, func(i, j int) bool { return c.Subscriptions[i].Name < c.Subscriptions[j].Name })
	groupUses := map[string][]string{}
	groups, ok := root["proxy-groups"].([]any)
	if root["proxy-groups"] != nil && !ok {
		return c, domain.Invalid("invalid_import", "proxy-groups")
	}
	targets := map[string]string{"DIRECT": "DIRECT", "REJECT": "REJECT"}
	for _, raw := range groups {
		g, ok := raw.(map[string]any)
		if !ok {
			return c, domain.Invalid("invalid_import", "proxy-groups")
		}
		if e := keys(g, "name", "type", "use", "proxies", "url", "interval", "tolerance"); e != nil {
			return c, e
		}
		if e := typedFields(g, map[string]string{"name": "string", "type": "string", "url": "string", "interval": "int", "tolerance": "int"}); e != nil {
			return c, e
		}
		item := Group{ID: domain.NewID(), Pools: []string{}, Builtins: []string{}, URL: "https://www.gstatic.com/generate_204", Interval: 300, Tolerance: 50}
		item.Name, _ = g["name"].(string)
		item.Type, _ = g["type"].(string)
		if use, ok := g["use"].([]any); ok {
			for _, v := range use {
				n, ok := v.(string)
				if !ok || sourceIDs[n] == "" {
					return c, domain.Invalid("unknown_provider", "proxy-groups.use")
				}
			}
			for _, v := range use {
				groupUses[item.ID] = append(groupUses[item.ID], v.(string))
			}
		} else if g["use"] != nil {
			return c, domain.Invalid("invalid_import", "use")
		}
		if bs, ok := g["proxies"].([]any); ok {
			for _, v := range bs {
				n, ok := v.(string)
				if !ok || (n != "DIRECT" && n != "REJECT") {
					return c, domain.Invalid("manual_proxy_or_nested_group_unsupported", "proxies")
				}
				item.Builtins = append(item.Builtins, n)
			}
		} else if g["proxies"] != nil {
			return c, domain.Invalid("invalid_import", "proxies")
		}
		if v, ok := g["url"].(string); ok {
			item.URL = v
		}
		for k, dst := range map[string]*int{"interval": &item.Interval, "tolerance": &item.Tolerance} {
			if v, exists := g[k]; exists {
				n, ok := v.(int)
				if !ok {
					return c, domain.Invalid("invalid_import", k)
				}
				*dst = n
			}
		}
		targets[item.Name] = item.ID
		c.Groups = append(c.Groups, item)
	}
	patterns := map[string]string{}
	for i, source := range c.Subscriptions {
		signature := ""
		for _, g := range c.Groups {
			found := false
			for _, name := range groupUses[g.ID] {
				if name == source.Name {
					found = true
				}
			}
			if found {
				signature += "1"
			} else {
				signature += "0"
			}
		}
		pool, ok := patterns[signature]
		if !ok {
			if len(patterns) >= 2 {
				return c, domain.Invalid("pool_mapping_required", "proxy-groups.use")
			}
			pool = "quality"
			if len(patterns) == 1 {
				pool = "quota"
			}
			patterns[signature] = pool
		}
		c.Subscriptions[i].Pool = pool
	}
	for i, g := range c.Groups {
		for _, pool := range []string{"quality", "quota"} {
			included := false
			for _, s := range c.Subscriptions {
				if s.Pool != pool {
					continue
				}
				for _, name := range groupUses[g.ID] {
					if name == s.Name {
						included = true
					}
				}
			}
			if included {
				c.Groups[i].Pools = append(c.Groups[i].Pools, pool)
			}
		}
	}
	for i := range c.Subscriptions {
		p := c.Subscriptions[i].Proxy
		if p == "" {
			continue
		}
		if targets[p] == "" {
			return c, domain.Invalid("unknown_proxy", "proxy")
		}
		c.Subscriptions[i].Proxy = targets[p]
	}
	setIDs := map[string]string{}
	sets, ok := root["rule-providers"].(map[string]any)
	if root["rule-providers"] != nil && !ok {
		return c, domain.Invalid("invalid_import", "rule-providers")
	}
	for name, raw := range sets {
		p, ok := raw.(map[string]any)
		if !ok {
			return c, domain.Invalid("invalid_import", "rule-providers")
		}
		if e := keys(p, "type", "behavior", "format", "url", "path", "interval", "payload"); e != nil {
			return c, e
		}
		if e := typedFields(p, map[string]string{"type": "string", "behavior": "string", "format": "string", "url": "string", "path": "string", "interval": "int"}); e != nil {
			return c, e
		}
		behavior, _ := p["behavior"].(string)
		if behavior != "domain" && behavior != "ipcidr" {
			return c, domain.Invalid("unsupported_behavior", "behavior")
		}
		if p["format"] != nil && p["format"] != "yaml" {
			return c, domain.Invalid("yaml_required", "format")
		}
		item := RuleSet{ID: domain.NewID(), Name: name, Entries: []Entry{}}
		switch p["type"] {
		case "http":
			item.Kind = "http"
			item.URL, _ = p["url"].(string)
			item.Behavior = behavior
			item.Interval = 86400
			if n, ok := p["interval"].(int); ok {
				item.Interval = n
			}
			if p["payload"] != nil {
				return c, domain.Invalid("unsupported_payload", "payload")
			}
		case "inline":
			item.Kind = "local"
			if p["url"] != nil || p["path"] != nil || p["interval"] != nil {
				return c, domain.Invalid("unexpected_remote_fields", "rule-providers")
			}
			entries, ok := p["payload"].([]any)
			if !ok {
				return c, domain.Invalid("invalid_payload", "payload")
			}
			for _, v := range entries {
				text, ok := v.(string)
				if !ok {
					return c, domain.Invalid("invalid_payload", "payload")
				}
				kind := "domain"
				if behavior == "ipcidr" {
					kind = "ip"
				} else if strings.HasPrefix(text, "+.") {
					kind = "suffix"
					text = strings.TrimPrefix(text, "+.")
				}
				item.Entries = append(item.Entries, Entry{ID: domain.NewID(), Type: kind, Value: text})
			}
		default:
			return c, domain.Invalid("unsupported_rule_provider", "rule-providers")
		}
		setIDs[name] = item.ID
		c.RuleSets = append(c.RuleSets, item)
	}
	rules, ok := root["rules"].([]any)
	if !ok {
		return c, domain.Invalid("rules_required", "rules")
	}
	fallback := false
	for i, v := range rules {
		text, ok := v.(string)
		if !ok {
			return c, domain.Invalid("invalid_rule", "rules")
		}
		p := strings.Split(text, ",")
		if len(p) == 2 && p[0] == "MATCH" {
			if i != len(rules)-1 || targets[p[1]] == "" {
				return c, domain.Invalid("invalid_fallback", "rules")
			}
			c.Fallback = targets[p[1]]
			fallback = true
			continue
		}
		if len(p) < 3 || len(p) > 4 || targets[p[2]] == "" || len(p) == 4 && p[3] != "no-resolve" {
			return c, domain.Invalid("invalid_rule", "rules")
		}
		r := Rule{ID: domain.NewID(), Type: p[0], Value: p[1], Target: targets[p[2]], NoResolve: len(p) == 4}
		if r.Type == "RULE-SET" {
			if setIDs[r.Value] == "" {
				return c, domain.Invalid("unknown_ruleset", "rules")
			}
			r.Value = setIDs[r.Value]
		}
		c.Rules = append(c.Rules, r)
	}
	if !fallback {
		return c, domain.Invalid("fallback_required", "rules")
	}
	return c, ValidateFields(c)
}
func keys(m map[string]any, allowed ...string) error {
	for k := range m {
		found := false
		for _, v := range allowed {
			if k == v {
				found = true
			}
		}
		if !found {
			return domain.Invalid("unsupported_import_field", k)
		}
	}
	return nil
}
func (s *Service) Import(ctx context.Context, revision int, content string) (Draft, error) {
	c, e := ImportPreview(content)
	if e != nil {
		return Draft{}, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.document(ctx)
	if e != nil {
		return Draft{}, e
	}
	if d.Draft.Revision != revision {
		return Draft{}, domain.ErrConflict
	}
	if d.Active != 0 || len(d.Draft.Config.Subscriptions)+len(d.Draft.Config.Groups)+len(d.Draft.Config.RuleSets)+len(d.Draft.Config.Rules) > 0 {
		return Draft{}, domain.Invalid("initial_import_only", "import")
	}
	d.Draft = Draft{Revision: revision + 1, Config: c}
	e = storage.Save(ctx, s.Store, "configuration", &d.Meta, &d, "")
	if e == nil {
		s.checked = nil
	}
	return d.Draft, e
}
func ImportSummary(c Config) string {
	return fmt.Sprintf("%d 个订阅、%d 个策略组、%d 个规则集、%d 条分流规则。基础配置随草稿保存；导入不会发布。", len(c.Subscriptions), len(c.Groups), len(c.RuleSets), len(c.Rules))
}

// Optional fields must retain their declared types; null is not an omitted field.
func typedFields(m map[string]any, fields map[string]string) error {
	for k, kind := range fields {
		v, exists := m[k]
		if !exists {
			continue
		}
		valid := false
		switch kind {
		case "string":
			_, valid = v.(string)
		case "int":
			_, valid = v.(int)
		case "bool":
			_, valid = v.(bool)
		case "map":
			_, valid = v.(map[string]any)
		case "list":
			_, valid = v.([]any)
		}
		if !valid {
			return domain.Invalid("invalid_import_type", k)
		}
	}
	return nil
}
