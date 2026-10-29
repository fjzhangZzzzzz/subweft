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
	"github.com/fjzhangZzzzzz/subweft/internal/source"
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
				if !s.Enabled || (s.Kind != "http" && len(s.Nodes) == 0) {
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
		if s.Kind == "http" {
			b, _ := json.Marshal(s.Update)
			result.Sources[s.ID] = Digest(append([]byte(s.URL), b...))
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
	// Include explicit update egress nodes and their dialer dependencies.
	for _, route := range p.SourceUpdates {
		for _, n := range snap.Nodes {
			if n.Node.Name == route {
				selectedManual[n.ID] = true
			}
		}
	}
	for _, route := range p.RuleUpdates {
		for _, n := range snap.Nodes {
			if n.Node.Name == route {
				selectedManual[n.ID] = true
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, n := range snap.Nodes {
			if selectedManual[n.ID] {
				if route, ok := n.Node.Options["dialer-proxy"].(string); ok {
					for _, dep := range snap.Nodes {
						if dep.Node.Name == route && !selectedManual[dep.ID] {
							selectedManual[dep.ID] = true
							changed = true
						}
					}
				}
			}
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
		excluded, e := excludedTypes(g.Options["exclude-type"])
		if e != nil {
			return result, domain.Field(field, e)
		}
		var include, exclude *regexp.Regexp
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
		remoteUses := []string{}
		seen := map[string]bool{}
		add := func(name string, filter bool) {
			kind := nodeMap[name].Type
			if child, ok := groups[name]; ok {
				kind = child.Type
			}
			if name == "DIRECT" || name == "REJECT" {
				kind = name
			}
			if excluded[strings.ToLower(typeNames[kind])] {
				return
			}
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
				if s.Kind == "http" {
					remoteUses = append(remoteUses, s.ID)
				}
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
		if len(members) == 0 && len(remoteUses) == 0 {
			return result, domain.Invalid("empty_group", field)
		}
		if selected, ok := g.Options["default-selected"].(string); ok && selected != "" && !seen[selected] && len(remoteUses) == 0 {
			return result, domain.Invalid("default_member_missing", field+".options.default-selected")
		}
		result.Members[g.Name] = members
		out := map[string]any{"name": g.Name, "type": g.Type, "proxies": members}
		for _, id := range remoteUses {
			for _, src := range snap.Sources {
				if src.ID == id {
					provider, e := remoteProvider(src, p)
					if e != nil {
						return result, e
					}
					route := provider["proxy"].(string)
					if !names[route] {
						return result, domain.Invalid("provider_proxy_missing", "sourceUpdates."+id)
					}
					fetchDependencies[g.Name] = append(fetchDependencies[g.Name], route)
					proxyProviders[id] = provider
				}
			}
		}
		if len(remoteUses) > 0 {
			out["use"] = remoteUses
			if g.Filter != "" {
				out["filter"] = g.Filter
			}
			if g.Exclude != "" {
				out["exclude-filter"] = g.Exclude
			}
			if value, ok := g.Options["exclude-type"]; ok {
				out["exclude-type"] = value
			}
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
			if key != "exclude-type" {
				out[key] = fields.Normalize(value)
			}
		}
		outputGroups = append(outputGroups, out)
	}
	needed, e := validateDependencies(nodes, result.Members, fetchDependencies)
	if e != nil {
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
	providers := map[string]any{}
	for id, digest := range resources {
		r := resourceMap[id]
		address := ResourceURL(id)
		if r.Kind == "http" {
			address = r.URL
			if e := source.ValidateURL(address); e != nil {
				return result, domain.Field("rules", e)
			}
			if e := ValidateDeviceUpdate(domain.DeviceUpdate{Headers: r.Headers}); e != nil {
				return result, e
			}
		} else {
			result.Resources[id] = digest
		}
		route := p.RuleUpdates[id]
		if route == "" {
			route = "DIRECT"
		}
		if !names[route] {
			return result, domain.Invalid("provider_proxy_missing", "ruleUpdates."+id)
		}
		// Validate reachability of a rule update route without introducing a traffic dependency.
		routeNeeded, e := validateDependencies(nodes, mapWithRoot(result.Members, route), fetchDependencies)
		if e != nil {
			return result, e
		}
		for name := range routeNeeded {
			needed[name] = true
		}
		provider := map[string]any{"type": "http", "behavior": r.Behavior, "format": r.Format, "url": address, "interval": r.Interval, "proxy": route}
		if r.Kind == "http" {
			provider["header"] = deviceHeaders("", r.Headers)
		}
		provider["path"] = devicePath(p.ID, id, r.Format, []any{provider, digest})
		providers[id] = provider
	}

	config := map[string]any{"mixed-port": p.Settings.MixedPort, "allow-lan": p.Settings.AllowLAN, "ipv6": p.Settings.IPv6, "mode": "rule", "log-level": "info", "proxies": renderedNodes(nodes, externalNodes, needed), "proxy-groups": outputGroups, "rules": rules}
	if len(proxyProviders) > 0 {
		config["proxy-providers"] = proxyProviders
	}
	if len(providers) > 0 {
		config["rule-providers"] = providers
	}
	geox := map[string]string{}
	for key, id := range map[string]string{"mmdb": p.GeoIPID, "geosite": p.GeoSiteID, "asn": p.GeoASNID} {
		if id == "" {
			continue
		}
		r, ok := resourceMap[id]
		if !ok || !r.Enabled || (r.Kind != "http" && r.Digest == "") || !(r.Type == key || key == "mmdb" && r.Type == "geoip") {
			return result, domain.Invalid("geodata_unavailable", "settings")
		}
		actual := key
		if key == "mmdb" && r.Type == "geoip" {
			actual = "geoip"
			config["geodata-mode"] = true
		}
		if r.Kind == "http" {
			if e := source.ValidateURL(r.URL); e != nil {
				return result, e
			}
			geox[actual] = r.URL
		} else {
			result.Resources[id] = r.Digest
			geox[actual] = ResourceURL(id)
		}
	}
	if len(geox) > 0 {
		config["geox-url"] = geox
		config["geo-auto-update"] = true
		config["geo-update-interval"] = 24
		if p.GeoAutoUpdate != nil {
			config["geo-auto-update"] = *p.GeoAutoUpdate
		}
		if p.GeoUpdateInterval != 0 {
			config["geo-update-interval"] = p.GeoUpdateInterval
		}
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
	var root yaml.Node
	if yaml.Unmarshal(config, &root) != nil || len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return []byte("preview unavailable\n")
	}
	mapping := root.Content[0]
	for i := 0; i < len(mapping.Content); i += 2 {
		key, value := mapping.Content[i].Value, mapping.Content[i+1]
		switch key {
		case "proxies":
			for _, node := range value.Content {
				var v map[string]any
				if node.Decode(&v) != nil {
					return []byte("preview unavailable\n")
				}
				kind, _ := v["type"].(string)
				if node.Encode(fields.Redact(kind, v)) != nil {
					return []byte("preview unavailable\n")
				}
			}
		case "proxy-providers", "rule-providers":
			for j := 1; j < len(value.Content); j += 2 {
				m := value.Content[j]
				for k := 0; k+1 < len(m.Content); k += 2 {
					if m.Content[k].Value == "url" || m.Content[k].Value == "header" {
						_ = m.Content[k+1].Encode("[REDACTED]")
					}
				}
			}
		case "geox-url", "secret", "authentication":
			_ = value.Encode("[REDACTED]")
		}
	}
	b, e := yaml.Marshal(&root)
	if e != nil {
		return []byte("preview unavailable\n")
	}
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

func renderedNodes(nodes []domain.Node, external map[string]bool, used map[string]bool) []domain.Node {
	needed := map[string]bool{}
	for _, n := range nodes {
		if p, ok := n.Options["dialer-proxy"].(string); ok && used[n.Name] {
			needed[p] = true
		}
	}
	out := []domain.Node{}
	for _, n := range nodes {
		if used[n.Name] && (!external[n.Name] || needed[n.Name]) {
			out = append(out, n)
		}
	}
	return out
}

func mapWithRoot(members map[string][]string, route string) map[string][]string {
	out := map[string][]string{}
	for k, v := range members {
		out[k] = v
	}
	out["\x00rule-update"] = []string{route}
	return out
}
