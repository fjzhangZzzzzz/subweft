package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strings"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"go.yaml.in/yaml/v3"
)

type Result struct {
	Config  []byte
	Members map[string][]string
	Sources map[string]string
	Stale   []string
}

func Digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Compile(p domain.Profile, snap domain.Snapshot, allowStale bool) (Result, error) {
	result := Result{Members: map[string][]string{}, Sources: map[string]string{}, Stale: []string{}}
	if e := domain.ValidateProfile(p); e != nil {
		return result, e
	}
	if len(p.Groups) == 0 {
		return result, domain.Invalid("no_groups", "groups")
	}
	nodes := []domain.Node{}
	sourceNodes := map[string][]string{}
	manualNames := map[string]string{}
	names := map[string]bool{"DIRECT": true, "REJECT": true}
	groups := map[string]domain.Group{}
	for i, g := range p.Groups {
		if !domain.ValidName(g.Name) || names[g.Name] {
			return result, domain.Invalid("duplicate_or_invalid_name", fmt.Sprintf("groups.%d.name", i))
		}
		names[g.Name] = true
		groups[g.Name] = g
	}
	addNode := func(n domain.Node) error {
		if names[n.Name] {
			return domain.Invalid("duplicate_or_reserved_name", "nodes")
		}
		if e := domain.ValidateNode(n); e != nil {
			return e
		}
		names[n.Name] = true
		nodes = append(nodes, n)
		return nil
	}
	// Only include selected sources; unrelated unavailable sources do not block a profile.
	selected := map[string]bool{}
	for _, g := range p.Groups {
		for _, id := range g.SourceIDs {
			selected[id] = true
		}
		for _, s := range snap.Sources {
			if s.Enabled && (g.AllSources || intersects(g.Tags, s.Tags)) {
				selected[s.ID] = true
			}
		}
	}
	for id := range selected {
		found := false
		for _, s := range snap.Sources {
			if s.ID == id {
				found = true
				if !s.Enabled || len(s.Nodes) == 0 {
					return result, domain.Invalid("source_unavailable", "groups")
				}
			}
		}
		if !found {
			return result, domain.Invalid("source_missing", "groups")
		}
	}
	for _, s := range snap.Sources {
		if !selected[s.ID] {
			continue
		}
		if s.LastError != "" {
			result.Stale = append(result.Stale, s.Name)
		}
		b, _ := json.Marshal(s.Nodes)
		result.Sources[s.ID] = Digest(b)
		for _, n := range s.Nodes {
			n.Name = "[" + s.Name + "] " + n.Name
			if e := addNode(n); e != nil {
				return result, e
			}
			sourceNodes[s.ID] = append(sourceNodes[s.ID], n.Name)
		}
	}
	if len(result.Stale) > 0 && !allowStale {
		return result, domain.Invalid("stale_cache_requires_ack", "allowStale")
	}
	selectedManual := map[string]bool{}
	for _, g := range p.Groups {
		for _, id := range g.NodeIDs {
			selectedManual[id] = true
		}
	}
	for _, n := range snap.Nodes {
		if !selectedManual[n.ID] {
			continue
		}
		if !n.Enabled {
			return result, domain.Invalid("node_unavailable", "groups")
		}
		if e := addNode(n.Node); e != nil {
			return result, e
		}
		manualNames[n.ID] = n.Node.Name
	}
	for id := range selectedManual {
		if manualNames[id] == "" {
			return result, domain.Invalid("node_missing", "groups")
		}
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if visiting[name] {
			return domain.Invalid("group_cycle", "groups")
		}
		if visited[name] {
			return nil
		}
		g, ok := groups[name]
		if !ok {
			return domain.Invalid("group_missing", "groups")
		}
		visiting[name] = true
		for _, child := range g.Groups {
			if e := visit(child); e != nil {
				return e
			}
		}
		visiting[name] = false
		visited[name] = true
		return nil
	}
	for name := range groups {
		if e := visit(name); e != nil {
			return result, e
		}
	}
	outputGroups := []map[string]any{}
	for index, g := range p.Groups {
		field := fmt.Sprintf("groups.%d", index)
		switch g.Type {
		case "select", "url-test", "fallback", "load-balance":
		default:
			return result, domain.Invalid("unsupported_group_type", field+".type")
		}
		if len(g.Filter) > 512 || len(g.Exclude) > 512 {
			return result, domain.Invalid("filter_too_long", field+".filter")
		}
		var include, exclude *regexp.Regexp
		var e error
		if g.Filter != "" {
			include, e = regexp.Compile(g.Filter)
			if e != nil {
				return result, domain.Invalid("invalid_filter", field+".filter")
			}
		}
		if g.Exclude != "" {
			exclude, e = regexp.Compile(g.Exclude)
			if e != nil {
				return result, domain.Invalid("invalid_filter", field+".exclude")
			}
		}
		members := []string{}
		seen := map[string]bool{}
		add := func(name string, filter bool) {
			if filter && ((include != nil && !include.MatchString(name)) || (exclude != nil && exclude.MatchString(name))) {
				return
			}
			if !seen[name] {
				members = append(members, name)
				seen[name] = true
			}
		}
		ids := map[string]bool{}
		for _, id := range g.SourceIDs {
			ids[id] = true
		}
		for _, s := range snap.Sources {
			if s.Enabled && (g.AllSources || ids[s.ID] || intersects(g.Tags, s.Tags)) {
				for _, n := range sourceNodes[s.ID] {
					add(n, true)
				}
			}
		}
		for _, id := range g.NodeIDs {
			add(manualNames[id], true)
		}
		for _, name := range g.Groups {
			add(name, false)
		}
		for _, name := range g.Builtins {
			if name != "DIRECT" && name != "REJECT" {
				return result, domain.Invalid("invalid_builtin", field+".builtins")
			}
			add(name, false)
		}
		if len(members) == 0 {
			return result, domain.Invalid("empty_group", field)
		}
		result.Members[g.Name] = members
		out := map[string]any{"name": g.Name, "type": g.Type, "proxies": members}
		if g.Type != "select" {
			u, e := url.Parse(g.URL)
			if e != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
				return result, domain.Invalid("invalid_test_url", field+".url")
			}
			if g.Interval < 30 || g.Interval > 86400 {
				return result, domain.Invalid("invalid_interval", field+".interval")
			}
			out["url"] = g.URL
			out["interval"] = g.Interval
			out["lazy"] = true
		}
		if g.Type == "url-test" {
			if g.Tolerance < 0 || g.Tolerance > 10000 {
				return result, domain.Invalid("invalid_tolerance", field+".tolerance")
			}
			out["tolerance"] = g.Tolerance
		}
		if g.Type == "load-balance" {
			switch g.Strategy {
			case "consistent-hashing", "round-robin", "sticky-sessions":
				out["strategy"] = g.Strategy
			default:
				return result, domain.Invalid("invalid_strategy", field+".strategy")
			}
		}
		outputGroups = append(outputGroups, out)
	}
	rules := []string{}
	match := false
	for i, r := range p.Rules {
		if !r.Enabled {
			continue
		}
		field := fmt.Sprintf("rules.%d", i)
		if match {
			return result, domain.Invalid("match_must_be_last", field)
		}
		if !names[r.Target] || (!hasGroup(groups, r.Target) && r.Target != "DIRECT" && r.Target != "REJECT") {
			return result, domain.Invalid("rule_target_missing", field+".target")
		}
		if strings.ContainsAny(r.Value, ",\r\n\x00") || len(r.Value) > 512 {
			return result, domain.Invalid("invalid_rule_value", field+".value")
		}
		switch r.Type {
		case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD":
			if r.Value == "" || strings.ContainsAny(r.Value, " /:") {
				return result, domain.Invalid("invalid_rule_value", field+".value")
			}
			if r.NoResolve {
				return result, domain.Invalid("no_resolve_not_applicable", field+".noResolve")
			}
		case "IP-CIDR", "IP-CIDR6":
			prefix, e := netip.ParsePrefix(r.Value)
			if e != nil || (r.Type == "IP-CIDR6") != prefix.Addr().Is6() {
				return result, domain.Invalid("invalid_cidr", field+".value")
			}
		case "MATCH":
			if r.Value != "" || r.NoResolve {
				return result, domain.Invalid("invalid_match", field)
			}
			match = true
		default:
			return result, domain.Invalid("unsupported_rule_type", field+".type")
		}
		line := r.Type + ","
		if r.Type != "MATCH" {
			line += r.Value + ","
		}
		line += r.Target
		if r.NoResolve {
			line += ",no-resolve"
		}
		rules = append(rules, line)
	}
	if !match {
		return result, domain.Invalid("match_required", "rules")
	}
	config := map[string]any{"mixed-port": p.Settings.MixedPort, "allow-lan": p.Settings.AllowLAN, "ipv6": p.Settings.IPv6, "mode": "rule", "log-level": "info", "proxies": nodes, "proxy-groups": outputGroups, "rules": rules}
	if p.Settings.DNSEnabled {
		if p.Settings.DNSMode != "fake-ip" && p.Settings.DNSMode != "redir-host" {
			return result, domain.Invalid("invalid_dns_mode", "settings.dnsMode")
		}
		if len(p.Settings.Nameservers) == 0 || len(p.Settings.Nameservers) > 16 {
			return result, domain.Invalid("nameservers_required", "settings.nameservers")
		}
		for _, ns := range p.Settings.Nameservers {
			if len(ns) > 2048 || strings.ContainsAny(ns, "\r\n\x00") {
				return result, domain.Invalid("invalid_nameserver", "settings.nameservers")
			}
		}
		config["dns"] = map[string]any{"enable": true, "enhanced-mode": p.Settings.DNSMode, "nameserver": p.Settings.Nameservers}
	} else {
		config["dns"] = map[string]any{"enable": false}
	}
	var e error
	result.Config, e = yaml.Marshal(config)
	return result, e
}
func intersects(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
func hasGroup(g map[string]domain.Group, name string) bool { _, ok := g[name]; return ok }
func Redact(config []byte) []byte {
	var v map[string]any
	if yaml.Unmarshal(config, &v) != nil {
		return []byte("preview unavailable\n")
	}
	if proxies, ok := v["proxies"].([]any); ok {
		for _, p := range proxies {
			if n, ok := p.(map[string]any); ok {
				n["password"] = "[REDACTED]"
			}
		}
	}
	b, _ := yaml.Marshal(v)
	return b
}
