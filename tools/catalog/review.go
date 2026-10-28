package main

// Review overlays describe product-owned fields and custom YAML scalar types.
// They must be reviewed when the pinned validator changes.
func choice(values ...string) *Field { return &Field{Kind: "string", Enum: values} }
func number(min, max float64) *Field { return &Field{Kind: "integer", Min: &min, Max: &max} }
func review(payload map[string]any) {
	protocols := payload["protocols"].(map[string]*Field)
	plugins := payload["pluginTypes"].(map[string]*Field)
	pluginNames := map[string]string{"obfs": "simpleObfsOption", "v2ray-plugin": "v2rayObfsOption", "gost-plugin": "gostObfsOption", "shadow-tls": "shadowTLSOption", "restls": "restlsOption", "jls": "jlsOption", "kcptun": "kcpTunOption"}
	pluginSchemas := map[string]*Field{}
	for k, v := range pluginNames {
		pluginSchemas[k] = plugins[v]
	}
	payload["plugins"] = pluginSchemas
	delete(payload, "pluginTypes")
	for kind, root := range protocols {
		root.Properties["type"] = &Field{Kind: "string", Required: true, Enum: []string{kind}}
		root.Properties["ip-version"] = choice("dual", "ipv4", "ipv6", "ipv4-prefer", "ipv6-prefer")
		if p := root.Properties["port"]; p != nil {
			p.Min = number(1, 65535).Min
			p.Max = number(1, 65535).Max
		}
	}
	protocols["ss"].Properties["plugin"] = choice("obfs", "v2ray-plugin", "gost-plugin", "shadow-tls", "restls", "jls", "kcptun")
	protocols["snell"].Properties["obfs-opts"] = plugins["simpleObfsOption"]
	dns := payload["dns"].(*Field)
	dns.Properties["enhanced-mode"] = choice("fake-ip", "redir-host")
	dns.Properties["fake-ip-filter-mode"] = choice("blacklist", "whitelist", "rule")
	dns.Properties["cache-algorithm"] = choice("lru", "arc")
	stringList := &Field{Kind: "union", Alternatives: []*Field{{Kind: "string"}, {Kind: "array", Items: &Field{Kind: "string"}}}}
	for _, name := range []string{"nameserver-policy", "proxy-server-nameserver-policy"} {
		dns.Properties[name] = &Field{Kind: "map", Values: stringList}
	}
	config := payload["config"].(*Field)
	allowed := map[string]*Field{}
	for _, k := range []string{"mixed-port", "port", "socks-port", "redir-port", "tproxy-port", "allow-lan", "bind-address", "ipv6", "mode", "log-level", "external-controller", "external-controller-cors", "secret", "authentication", "skip-auth-prefixes", "lan-allowed-ips", "lan-disallowed-ips", "find-process-mode", "global-client-fingerprint", "global-ua", "tcp-concurrent", "unified-delay", "keep-alive-idle", "keep-alive-interval", "disable-keep-alive", "interface-name", "routing-mark", "inbound-tfo", "inbound-mptcp", "profile", "hosts", "sniffer"} {
		if f := config.Properties[k]; f != nil {
			allowed[k] = f
		}
	}
	allowed["mode"] = choice("rule", "global", "direct")
	allowed["log-level"] = choice("debug", "info", "warning", "error", "silent")
	allowed["find-process-mode"] = choice("off", "strict", "always")
	allowed["hosts"] = &Field{Kind: "map", Values: stringList}
	payload["config"] = &Field{Kind: "object", Properties: allowed}
	// Membership, filtering and fallbacks are owned by the product compiler.
	for _, root := range payload["groups"].(map[string]*Field) {
		for _, k := range []string{"type", "name", "proxies", "use", "include-all", "include-all-proxies", "include-all-providers", "filter", "exclude-filter", "empty-fallback", "url", "interval", "strategy", "tolerance"} {
			delete(root.Properties, k)
		}
	}
	var mark func(*Field, string)
	mark = func(f *Field, key string) {
		if f == nil {
			return
		}
		if key == "pre-shared-key" || key == "key" || key == "primary-key" || key == "header-protection-key" || key == "client-metadata" || key == "ws-headers" || key == "secret" || key == "authentication" {
			f.Secret = true
		}
		for k, p := range f.Properties {
			mark(p, k)
		}
		mark(f.Items, key)
		mark(f.Values, key)
	}
	for _, root := range protocols {
		mark(root, "")
	}
	for _, root := range pluginSchemas {
		mark(root, "")
	}
	mark(payload["config"].(*Field), "")
}
