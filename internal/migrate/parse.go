// Package migrate converts explicit imports into reviewable drafts, never active releases.
package migrate

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/fields"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"go.yaml.in/yaml/v3"
)

type Input struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}
type ResourceDraft struct {
	Resource domain.Resource `json:"resource"`
	Data     []byte          `json:"data,omitempty"`
}
type Plan struct {
	Sources   []domain.Source     `json:"sources"`
	Nodes     []domain.ManualNode `json:"nodes"`
	Resources []ResourceDraft     `json:"resources"`
	Profile   domain.Profile      `json:"profile"`
	Warnings  []string            `json:"warnings"`
}

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func text(v any) string           { s, _ := v.(string); return s }
func flag(v any) bool             { b, _ := v.(bool); return b }
func integer(v any, fallback int) int {
	if v == nil {
		return fallback
	}
	b, _ := json.Marshal(v)
	var n int
	if json.Unmarshal(b, &n) != nil {
		return -1
	}
	return n
}
func list(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	a, ok := v.([]any)
	if !ok {
		return nil, domain.Invalid("invalid_field_type", "import")
	}
	out := []string{}
	for _, v := range a {
		s, ok := v.(string)
		if !ok {
			return nil, domain.Invalid("invalid_field_type", "import")
		}
		out = append(out, s)
	}
	return out, nil
}
func read(data []byte) (map[string]any, *yaml.Node, error) {
	if len(data) > 8<<20 {
		return nil, nil, domain.Invalid("input_too_large", "import")
	}
	var document yaml.Node
	d := yaml.NewDecoder(bytes.NewReader(data))
	if d.Decode(&document) != nil || len(document.Content) != 1 {
		return nil, nil, domain.Invalid("invalid_yaml", "import")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, nil, domain.Invalid("multiple_documents", "import")
	}
	value, e := fields.YAMLValue(document.Content[0])
	if e != nil {
		return nil, nil, domain.Invalid("invalid_field_type", "import")
	}
	root := object(value)
	if root == nil {
		return nil, nil, domain.Invalid("invalid_config", "import")
	}
	return root, document.Content[0], nil
}
func known(m map[string]any, names ...string) error {
	allowed := map[string]bool{}
	for _, k := range names {
		allowed[k] = true
	}
	for k := range m {
		if !allowed[k] {
			return domain.Invalid("unsupported_or_invalid_field", k)
		}
	}
	return nil
}
func Parse(in Input) (Plan, error) {
	plan := Plan{Sources: []domain.Source{}, Nodes: []domain.ManualNode{}, Resources: []ResourceDraft{}, Warnings: []string{}}
	if !domain.ValidName(in.Name) {
		return plan, domain.Invalid("invalid_name", "name")
	}
	root, ast, e := read([]byte(in.Content))
	if e != nil {
		return plan, e
	}
	for _, key := range []string{"proxy-providers", "rule-providers", "dns", "geox-url"} {
		if v, ok := root[key]; ok && object(v) == nil {
			return plan, domain.Invalid("invalid_field_type", key)
		}
	}
	p := domain.Profile{Meta: domain.Meta{ID: domain.NewID()}, Name: in.Name, Groups: []domain.Group{}, Rules: []domain.Rule{}, Settings: domain.Settings{DNSMode: "redir-host", Options: map[string]any{}, DNS: map[string]any{}}}
	providerIDs := map[string]string{}
	nodeIDs := map[string]string{}
	resourceIDs := map[string]string{}
	addSource := func(name string, raw map[string]any, enabled bool) error {
		if !domain.ValidName(name) {
			return domain.Invalid("invalid_name", "sources")
		}
		if e := validateProvider(raw); e != nil {
			return domain.Invalid("unsupported_or_invalid_field", "sources")
		}
		if e := known(raw, "name", "type", "url", "path", "interval", "proxy", "health-check", "override", "payload"); e != nil {
			return domain.Field("sources", e)
		}
		if prefix := object(raw["override"]); prefix != nil {
			if e := known(prefix, "additional-prefix"); e != nil {
				return e
			}
			if got := text(prefix["additional-prefix"]); got != "" && got != "["+name+"] " {
				return domain.Invalid("nonstandard_prefix_requires_review", "sources")
			}
		}
		s := domain.Source{Meta: domain.Meta{ID: domain.NewID()}, Name: name, Enabled: enabled, Kind: "http", URL: text(raw["url"]), Interval: integer(raw["interval"], 3600), Tags: []string{"imported"}, Fetch: domain.FetchOptions{Timeout: 20, Retries: 1}, ClientProvider: &domain.ClientProvider{Enabled: true, Interval: integer(raw["interval"], 3600), Proxy: text(raw["proxy"]), HealthCheck: domain.HealthCheck{Interval: 300, Timeout: 5000, Lazy: true}}}
		if h := object(raw["health-check"]); h != nil {
			if e := known(h, "enable", "url", "interval", "timeout", "lazy", "expected-status"); e != nil {
				return e
			}
			s.ClientProvider.HealthCheck = domain.HealthCheck{Enable: flag(h["enable"]), URL: text(h["url"]), Interval: integer(h["interval"], 300), Timeout: integer(h["timeout"], 5000), Lazy: true, ExpectedStatus: text(h["expected-status"])}
			if lazy, exists := h["lazy"]; exists {
				s.ClientProvider.HealthCheck.Lazy = flag(lazy)
			}
		}
		if text(raw["type"]) == "inline" {
			data, e := yaml.Marshal(map[string]any{"proxies": fields.Normalize(raw["payload"])})
			if e != nil {
				return e
			}
			s.Kind = "local"
			s.URL = ""
			s.Content = string(data)
			s.Nodes, e = source.Parse(data)
			if e != nil {
				return e
			}
		} else if text(raw["type"]) != "http" {
			return domain.Invalid("unsupported_provider_type", "sources")
		}
		if s.Kind == "http" {
			var e error
			s.URL, s.Fetch.Headers, e = source.NormalizeCredentials(s.URL, s.Fetch.Headers)
			if e != nil {
				return e
			}
		}
		if e := domain.ValidateSource(s); e != nil {
			return e
		}
		if s.Kind == "http" {
			if e := source.ValidateURL(s.URL); e != nil {
				return e
			}
		}
		if id := providerIDs[name]; id != "" {
			s.ID = id
			for i, old := range plan.Sources {
				if old.ID == id {
					plan.Sources[i] = s
					return nil
				}
			}
		}
		providerIDs[name] = s.ID
		plan.Sources = append(plan.Sources, s)
		return nil
	}
	for name, value := range object(root["proxy-providers"]) {
		if object(value) == nil {
			return plan, domain.Invalid("invalid_provider", "sources")
		}
		if e = addSource(name, object(value), true); e != nil {
			return plan, e
		}
	}
	addNode := func(raw any, enabled bool) error {
		n, e := domain.NodeFromCore(object(raw))
		if e != nil {
			return e
		}
		if e = domain.ValidateNode(n); e != nil {
			return e
		}
		if id := nodeIDs[n.Name]; id != "" {
			for i, old := range plan.Nodes {
				if old.ID == id {
					plan.Nodes[i] = domain.ManualNode{Meta: old.Meta, Node: n, Enabled: enabled}
					return nil
				}
			}
		}
		v := domain.ManualNode{Meta: domain.Meta{ID: domain.NewID()}, Node: n, Enabled: enabled}
		nodeIDs[n.Name] = v.ID
		plan.Nodes = append(plan.Nodes, v)
		return nil
	}
	if value, ok := root["proxies"]; ok {
		a, ok := value.([]any)
		if !ok {
			return plan, domain.Invalid("invalid_field_type", "proxies")
		}
		for _, raw := range a {
			if e = addNode(raw, true); e != nil {
				return plan, e
			}
		}
	}
	for name, value := range object(root["rule-providers"]) {
		raw := object(value)
		if raw == nil {
			return plan, domain.Invalid("invalid_ruleset", "resources")
		}
		if e = known(raw, "type", "url", "path", "interval", "behavior", "format", "payload", "proxy", "size-limit"); e != nil {
			return plan, e
		}
		if text(raw["proxy"]) != "" || raw["size-limit"] != nil {
			return plan, domain.Invalid("resource_fetch_settings_require_review", "resources")
		}
		r := domain.Resource{Meta: domain.Meta{ID: domain.NewID()}, Name: name, Type: "ruleset", Kind: text(raw["type"]), URL: text(raw["url"]), Enabled: true, Interval: integer(raw["interval"], 86400), Behavior: text(raw["behavior"]), Format: text(raw["format"])}
		if r.Format == "" {
			r.Format = "yaml"
		}
		draft := ResourceDraft{Resource: r}
		if r.Kind == "inline" {
			draft.Resource.Kind = "local"
			draft.Data, e = yaml.Marshal(map[string]any{"payload": fields.Normalize(raw["payload"])})
			if e != nil {
				return plan, e
			}
			if r.Format != "yaml" {
				return plan, domain.Invalid("inline_format_requires_yaml", "resources")
			}
		} else if r.Kind != "http" {
			return plan, domain.Invalid("unsupported_resource_type", "resources")
		}
		resourceIDs[name] = r.ID
		plan.Resources = append(plan.Resources, draft)
	}
	groups, ok := root["proxy-groups"].([]any)
	if !ok {
		return plan, domain.Invalid("groups_required", "groups")
	}
	groupNames := map[string]bool{}
	for _, value := range groups {
		raw := object(value)
		name := text(raw["name"])
		if !domain.ValidName(name) || groupNames[name] {
			return plan, domain.Invalid("duplicate_or_invalid_name", "groups")
		}
		groupNames[name] = true
	}
	for _, value := range groups {
		raw := object(value)
		if e := validateGroup(raw); e != nil {
			return plan, domain.Invalid("unsupported_or_invalid_field", "groups")
		}
		g := domain.Group{Name: text(raw["name"]), Type: text(raw["type"]), Filter: text(raw["filter"]), Exclude: text(raw["exclude-filter"]), URL: text(raw["url"]), Interval: integer(raw["interval"], 300), Tolerance: integer(raw["tolerance"], 0), Strategy: text(raw["strategy"]), Options: map[string]any{}}
		if g.Strategy == "" {
			g.Strategy = "consistent-hashing"
		}
		uses, e := list(raw["use"])
		if e != nil {
			return plan, e
		}
		for _, name := range uses {
			if providerIDs[name] == "" {
				return plan, domain.Invalid("source_missing", "groups")
			}
			g.SourceIDs = append(g.SourceIDs, providerIDs[name])
		}
		members, e := list(raw["proxies"])
		if e != nil {
			return plan, e
		}
		for _, name := range members {
			switch {
			case name == "DIRECT" || name == "REJECT":
				g.Builtins = append(g.Builtins, name)
			case groupNames[name]:
				g.Groups = append(g.Groups, name)
			case nodeIDs[name] != "":
				g.NodeIDs = append(g.NodeIDs, nodeIDs[name])
			default:
				return plan, domain.Invalid("group_member_missing", "groups")
			}
		}
		owned := map[string]bool{}
		for _, k := range []string{"name", "type", "use", "proxies", "filter", "exclude-filter", "url", "interval", "tolerance", "strategy"} {
			owned[k] = true
		}
		for k, v := range raw {
			if !owned[k] {
				g.Options[k] = v
			}
		}
		if e = fields.Validate("group:"+g.Type, g.Options); e != nil {
			return plan, domain.Invalid("unsupported_or_invalid_field", "groups")
		}
		p.Groups = append(p.Groups, g)
	}
	ruleLines, e := list(root["rules"])
	if e != nil {
		return plan, e
	}
	for _, line := range ruleLines {
		r, e := ParseRule(line)
		if e != nil {
			return plan, e
		}
		if r.Type == "RULE-SET" {
			if resourceIDs[r.Value] == "" {
				return plan, domain.Invalid("ruleset_missing", "rules")
			}
			r.Value = resourceIDs[r.Value]
		}
		p.Rules = append(p.Rules, r)
	}
	special := map[string]bool{"proxy-providers": true, "proxy-groups": true, "proxies": true, "rule-providers": true, "rules": true, "dns": true, "geox-url": true, "geodata-mode": true}
	for k, v := range root {
		if special[k] {
			continue
		}
		switch k {
		case "mixed-port":
			p.Settings.MixedPort = integer(v, 0)
		case "allow-lan":
			p.Settings.AllowLAN = flag(v)
		case "ipv6":
			p.Settings.IPv6 = flag(v)
		default:
			p.Settings.Options[k] = v
		}
	}
	globalValues := map[string]any{}
	for k, v := range root {
		if !special[k] {
			globalValues[k] = v
		}
	}
	if e = fields.Validate("config", globalValues); e != nil {
		return plan, domain.Invalid("unsupported_or_invalid_field", "settings")
	}
	if dns := object(root["dns"]); dns != nil {
		if e = fields.Validate("dns", dns); e != nil {
			return plan, domain.Invalid("unsupported_or_invalid_field", "dns")
		}
		for k, v := range dns {
			switch k {
			case "enable":
				p.Settings.DNSEnabled = flag(v)
			case "enhanced-mode":
				p.Settings.DNSMode = text(v)
			case "nameserver":
				p.Settings.Nameservers, e = list(v)
				if e != nil {
					return plan, e
				}
			case "nameserver-policy", "proxy-server-nameserver-policy":
			default:
				p.Settings.DNS[k] = v
			}
		}
		p.Settings.NameserverPolicy, e = dnsPolicy(ast, "nameserver-policy")
		if e != nil {
			return plan, e
		}
		p.Settings.ProxyNameserverPolicy, e = dnsPolicy(ast, "proxy-server-nameserver-policy")
		if e != nil {
			return plan, e
		}
	}
	urls := object(root["geox-url"])
	if e := known(urls, "mmdb", "geoip", "geosite", "asn"); e != nil {
		return plan, e
	}
	for _, value := range urls {
		if _, ok := value.(string); !ok {
			return plan, domain.Invalid("invalid_field_type", "geox-url")
		}
	}
	if value, exists := root["geodata-mode"]; exists {
		if _, ok := value.(bool); !ok {
			return plan, domain.Invalid("invalid_field_type", "geodata-mode")
		}
	}
	needsCountry := false
	needsSite := false
	needsASN := false
	for _, r := range p.Rules {
		needsCountry = needsCountry || hasType(r, "GEOIP") || hasType(r, "SRC-GEOIP")
		needsSite = needsSite || hasType(r, "GEOSITE")
		needsASN = needsASN || hasType(r, "IP-ASN") || hasType(r, "SRC-IP-ASN")
	}
	if filter := object(p.Settings.DNS["fallback-filter"]); filter != nil {
		needsCountry = needsCountry || flag(filter["geoip"])
		sites, _ := list(filter["geosite"])
		needsSite = needsSite || len(sites) > 0
	}
	for _, policy := range append(p.Settings.NameserverPolicy, p.Settings.ProxyNameserverPolicy...) {
		needsSite = needsSite || strings.HasPrefix(policy.Pattern, "geosite:")
	}
	if filters, _ := list(p.Settings.DNS["fake-ip-filter"]); len(filters) > 0 {
		for _, value := range filters {
			needsSite = needsSite || strings.HasPrefix(value, "geosite:")
		}
	}
	countryKind := "mmdb"
	if flag(root["geodata-mode"]) {
		countryKind = "geoip"
	}
	for kind, needed := range map[string]bool{"mmdb": needsCountry && countryKind == "mmdb", "geoip": needsCountry && countryKind == "geoip", "geosite": needsSite, "asn": needsASN} {
		if !needed && urls[kind] == nil {
			continue
		}
		address := text(urls[kind])
		if address == "" {
			filename := map[string]string{"mmdb": "geoip.metadb", "geoip": "geoip.dat", "geosite": "geosite.dat", "asn": "GeoLite2-ASN.mmdb"}[kind]
			address = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/" + filename
		}
		r := domain.Resource{Meta: domain.Meta{ID: domain.NewID()}, Name: in.Name + " " + kind, Kind: "http", Type: kind, URL: address, Enabled: true, Interval: 86400}
		plan.Resources = append(plan.Resources, ResourceDraft{Resource: r})
		if kind == countryKind {
			p.GeoIPID = r.ID
		} else if kind == "geosite" {
			p.GeoSiteID = r.ID
		} else if kind == "asn" {
			p.GeoASNID = r.ID
		}
	}
	domain.NormalizeProfile(&p)
	if e = domain.ValidateProfile(p); e != nil {
		return plan, e
	}
	plan.Profile = p
	plan.Warnings = append(plan.Warnings, "import_creates_draft_only", "client_cache_paths_are_service_managed", "refresh_http_sources_and_resources_before_publish")
	if needsCountry || needsSite {
		plan.Warnings = append(plan.Warnings, "review_managed_geodata_source_or_upload_existing_client_data")
	}
	return plan, nil
}
func hasType(r domain.Rule, kind string) bool {
	if r.Type == kind {
		return true
	}
	var walk func([]domain.Condition) bool
	walk = func(cs []domain.Condition) bool {
		for _, c := range cs {
			if c.Type == kind || walk(c.Children) {
				return true
			}
		}
		return false
	}
	return walk(r.Conditions)
}
func dnsPolicy(root *yaml.Node, key string) ([]domain.DNSPolicy, error) {
	out := []domain.DNSPolicy{}
	var dns *yaml.Node
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "dns" {
			dns = root.Content[i+1]
		}
	}
	if dns == nil {
		return out, nil
	}
	for i := 0; i < len(dns.Content); i += 2 {
		if dns.Content[i].Value != key {
			continue
		}
		m := dns.Content[i+1]
		for j := 0; j < len(m.Content); j += 2 {
			v, e := fields.YAMLValue(m.Content[j+1])
			if e != nil {
				return nil, e
			}
			servers := []string{}
			if s, ok := v.(string); ok {
				servers = []string{s}
			} else {
				servers, e = list(v)
				if e != nil {
					return nil, e
				}
			}
			out = append(out, domain.DNSPolicy{Pattern: m.Content[j].Value, Nameservers: servers})
		}
	}
	return out, nil
}

// Imported logical expressions are split only at balanced top-level separators.
func splitExpression(s string) ([]string, error) {
	depth := 0
	last := 0
	parts := []string{}
	for i, r := range s {
		switch r {
		case '(':
			depth++
			if depth > 16 {
				return nil, domain.Invalid("rule_too_deep", "rules")
			}
		case ')':
			depth--
			if depth < 0 {
				return nil, domain.Invalid("invalid_logical_rule", "rules")
			}
		case ',':
			if depth == 0 {
				parts = append(parts, s[last:i])
				last = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, domain.Invalid("invalid_logical_rule", "rules")
	}
	parts = append(parts, s[last:])
	return parts, nil
}
func parseCondition(s string) (domain.Condition, error) {
	parts, e := splitExpression(s)
	if e != nil {
		return domain.Condition{}, e
	}
	if len(parts) < 2 {
		return domain.Condition{}, domain.Invalid("invalid_rule_value", "rules")
	}
	c := domain.Condition{Type: parts[0], Value: parts[1]}
	if c.Type == "AND" || c.Type == "OR" || c.Type == "NOT" {
		if len(parts) != 2 || !strings.HasPrefix(c.Value, "(") || !strings.HasSuffix(c.Value, ")") {
			return c, domain.Invalid("invalid_logical_rule", "rules")
		}
		children, e := splitExpression(c.Value[1 : len(c.Value)-1])
		if e != nil {
			return c, e
		}
		c.Value = ""
		for _, child := range children {
			if !strings.HasPrefix(child, "(") || !strings.HasSuffix(child, ")") {
				return c, domain.Invalid("invalid_logical_rule", "rules")
			}
			parsed, e := parseCondition(child[1 : len(child)-1])
			if e != nil {
				return c, e
			}
			c.Children = append(c.Children, parsed)
		}
	} else if len(parts) == 3 && parts[2] == "no-resolve" {
		c.NoResolve = true
	} else if len(parts) != 2 {
		return c, domain.Invalid("invalid_rule_value", "rules")
	}
	_, e = compile.RuleCondition(c, 0)
	return c, e
}
func ParseRule(line string) (domain.Rule, error) {
	parts, e := splitExpression(line)
	if e != nil {
		return domain.Rule{}, e
	}
	r := domain.Rule{Enabled: true}
	if len(parts) > 0 && parts[len(parts)-1] == "no-resolve" {
		r.NoResolve = true
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 2 && parts[0] == "MATCH" {
		r.Type = "MATCH"
		r.Target = parts[1]
		return r, nil
	}
	if len(parts) < 3 {
		return r, domain.Invalid("invalid_rule_value", "rules")
	}
	r.Type = parts[0]
	r.Target = parts[len(parts)-1]
	if r.Type == "RULE-SET" && len(parts) == 3 {
		r.Value = parts[1]
		return r, nil
	}
	c, e := parseCondition(strings.Join(parts[:len(parts)-1], ","))
	if e != nil {
		return r, e
	}
	r.Value = c.Value
	r.Conditions = c.Children
	return r, nil
}
