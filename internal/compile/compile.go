package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/fields"
	"go.yaml.in/yaml/v3"
)

type Result struct {
	Assets    map[string][]byte
	Resources map[string]string
	Config    []byte
	Members   map[string][]string
	Sources   map[string]string
	Stale     []string
}

func Digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Compile(p domain.Profile, snap domain.Snapshot, allowStale bool) (Result, error) {
	result := Result{Assets: map[string][]byte{}, Resources: map[string]string{}, Members: map[string][]string{}, Sources: map[string]string{}, Stale: []string{}}
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
		effective, e := domain.SourceNodes(s)
		if e != nil {
			return result, e
		}
		localNames := map[string]string{}
		for _, n := range effective {
			localNames[n.Name] = "[" + s.Name + "] " + n.Name
		}
		for _, n := range effective {
			n.Name = "[" + s.Name + "] " + n.Name
			if dialer, ok := n.Options["dialer-proxy"].(string); ok {
				if mapped, exists := localNames[dialer]; exists {
					cloned := map[string]any{}
					for k, v := range n.Options {
						cloned[k] = v
					}
					cloned["dialer-proxy"] = mapped
					n.Options = cloned
				}
			}
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
	fetchDependencies := map[string][]string{}
	proxyProviders := map[string]any{}
	externalNodes := map[string]bool{}
	nodeMap := map[string]domain.Node{}
	for _, n := range nodes {
		nodeMap[n.Name] = n
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
		uses := []string{}
		providerMembers := map[string]bool{}
		for _, src := range snap.Sources {
			if !src.Enabled || src.ClientProvider == nil || !src.ClientProvider.Enabled || !(g.AllSources || ids[src.ID] || intersects(g.Tags, src.Tags)) {
				continue
			}
			subset := []domain.Node{}
			for _, name := range sourceNodes[src.ID] {
				if seen[name] {
					subset = append(subset, nodeMap[name])
					providerMembers[name] = true
					externalNodes[name] = true
				}
			}
			if len(subset) == 0 {
				continue
			}
			if proxy := src.ClientProvider.Proxy; proxy != "" && !names[proxy] {
				return result, domain.Invalid("provider_proxy_missing", field)
			}
			id, provider, data, e := providerResource(src, subset, index)
			if e != nil {
				return result, e
			}
			if proxy := src.ClientProvider.Proxy; proxy != "" {
				for _, node := range subset {
					if node.Name == proxy {
						return result, domain.Invalid("provider_download_cycle", field)
					}
				}
				fetchDependencies[g.Name] = append(fetchDependencies[g.Name], proxy)
			}
			uses = append(uses, id)
			proxyProviders[id] = provider
			result.Assets[id] = data
			result.Resources[id] = Digest(data)
		}
		if len(uses) > 0 {
			explicit := []string{}
			for _, name := range members {
				if !providerMembers[name] {
					explicit = append(explicit, name)
				}
			}
			out["proxies"] = explicit
			out["use"] = uses
		}
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
		if e := fields.Validate("group:"+g.Type, g.Options); g.Options != nil && e != nil {
			return result, contractError(field+".options", e)
		}
		for key, value := range g.Options {
			out[key] = fields.Normalize(value)
		}
		outputGroups = append(outputGroups, out)
	}
	if e := validateDependencies(nodes, result.Members, fetchDependencies); e != nil {
		return result, e
	}
	resourceMap := map[string]domain.Resource{}
	for _, r := range snap.Resources {
		resourceMap[r.ID] = r
	}
	ruleTargets := map[string]bool{"DIRECT": true, "REJECT": true}
	for name := range groups {
		ruleTargets[name] = true
	}
	rules, resources, e := renderRules(p, ruleTargets, resourceMap)
	if e != nil {
		return result, e
	}
	for id, digest := range resources {
		result.Resources[id] = digest
	}
	providers := map[string]any{}
	for id, digest := range resources {
		r := resourceMap[id]
		if r.LastError != "" && !allowStale {
			return result, domain.Invalid("stale_cache_requires_ack", "allowStale")
		}
		_ = digest
		ext := "yaml"
		if r.Format == "text" {
			ext = "txt"
		}
		providers[id] = map[string]any{"type": "http", "behavior": r.Behavior, "format": r.Format, "url": ResourceURL(id), "path": "./rules/" + id + "." + ext, "interval": 86400}
	}
	config := map[string]any{"mixed-port": p.Settings.MixedPort, "allow-lan": p.Settings.AllowLAN, "ipv6": p.Settings.IPv6, "mode": "rule", "log-level": "info", "proxies": renderedNodes(nodes, externalNodes), "proxy-groups": outputGroups, "rules": rules}
	if len(proxyProviders) > 0 {
		config["proxy-providers"] = proxyProviders
	}
	if len(providers) > 0 {
		config["rule-providers"] = providers
	}
	geox := map[string]string{}
	for key, id := range map[string]string{"mmdb": p.GeoIPID, "geosite": p.GeoSiteID} {
		if id != "" {
			r, ok := resourceMap[id]
			if !ok || r.Type != key || !r.Enabled || r.Digest == "" {
				return result, domain.Invalid("geodata_unavailable", "settings")
			}
			result.Resources[id] = r.Digest
			geox[key] = ResourceURL(id)
		}
	}
	if len(geox) > 0 {
		config["geox-url"] = geox
		config["geo-auto-update"] = false
	}
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
	if p.Settings.Options != nil {
		if e := fields.Validate("config", p.Settings.Options); e != nil {
			return result, contractError("settings.options", e)
		}
		for k, v := range p.Settings.Options {
			config[k] = fields.Normalize(v)
		}
	}
	dns := config["dns"].(map[string]any)
	if p.Settings.DNS != nil {
		if e := fields.Validate("dns", p.Settings.DNS); e != nil {
			return result, contractError("settings.dns", e)
		}
		for k, v := range p.Settings.DNS {
			dns[k] = fields.Normalize(v)
		}
	}
	for key, policies := range map[string][]domain.DNSPolicy{"nameserver-policy": p.Settings.NameserverPolicy, "proxy-server-nameserver-policy": p.Settings.ProxyNameserverPolicy} {
		if len(policies) > 0 {
			ordered := &yaml.Node{Kind: yaml.MappingNode}
			seen := map[string]bool{}
			for _, policy := range policies {
				if policy.Pattern == "" || seen[policy.Pattern] || len(policy.Nameservers) == 0 {
					return result, domain.Invalid("invalid_dns_policy", "settings."+key)
				}
				seen[policy.Pattern] = true
				k := &yaml.Node{}
				_ = k.Encode(policy.Pattern)
				v := &yaml.Node{}
				_ = v.Encode(policy.Nameservers)
				ordered.Content = append(ordered.Content, k, v)
			}
			dns[key] = ordered
		}
	}
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
		for i, p := range proxies {
			if n, ok := p.(map[string]any); ok {
				kind, _ := n["type"].(string)
				proxies[i] = fields.Redact(kind, n)
			}
		}
	}
	for _, k := range []string{"secret", "authentication"} {
		if _, ok := v[k]; ok {
			v[k] = "[REDACTED]"
		}
	}
	b, _ := yaml.Marshal(v)
	return b
}

func contractError(prefix string, e error) error {
	if x, ok := e.(*fields.Error); ok {
		return domain.Invalid(x.Code, prefix+"."+x.Path)
	}
	return domain.Invalid("invalid_field_type", prefix)
}

const ResourcePrefix = "https://subweft.invalid/resources/"

func ResourceURL(id string) string { return ResourcePrefix + id }

func renderedNodes(nodes []domain.Node, external map[string]bool) []domain.Node {
	needed := map[string]bool{}
	for _, n := range nodes {
		if p, ok := n.Options["dialer-proxy"].(string); ok {
			needed[p] = true
		}
	}
	out := []domain.Node{}
	for _, n := range nodes {
		if !external[n.Name] || needed[n.Name] {
			out = append(out, n)
		}
	}
	return out
}
