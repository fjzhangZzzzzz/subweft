package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type NodeOverride struct {
	Identity string `json:"identity"`
	Name     string `json:"name"`
	Excluded bool   `json:"excluded"`
	Notes    string `json:"notes"`
}

// Stable identity excludes mutable labels and credentials. Multiple equal identities
// are deliberately ambiguous; an override is never reassigned by name or position.
func NodeIdentity(n Node) string {
	core, _ := n.Core()
	m := map[string]any{}
	for _, k := range []string{"type", "server", "port", "cipher", "network", "tls", "servername", "sni", "public-key", "protocol", "obfs"} {
		if v, ok := core[k]; ok {
			m[k] = v
		}
	}
	if peers, ok := core["peers"]; ok {
		b, _ := json.Marshal(peers)
		var p []map[string]any
		_ = json.Unmarshal(b, &p)
		endpoints := []map[string]any{}
		for _, peer := range p {
			endpoints = append(endpoints, map[string]any{"server": peer["server"], "port": peer["port"], "public-key": peer["public-key"]})
		}
		m["peers"] = endpoints
	}
	b, _ := json.Marshal(m)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func SourceNodes(s Source) ([]Node, error) {
	counts := map[string]int{}
	for _, n := range s.Nodes {
		counts[NodeIdentity(n)]++
	}
	overrides := map[string]NodeOverride{}
	for _, o := range s.Overrides {
		if counts[o.Identity] > 1 {
			return nil, Invalid("ambiguous_node_identity", "overrides")
		}
		overrides[o.Identity] = o
	}
	out := []Node{}
	for _, n := range s.Nodes {
		o, ok := overrides[NodeIdentity(n)]
		if ok {
			if o.Excluded {
				continue
			}
			if o.Name != "" {
				n.Name = o.Name
			}
		}
		out = append(out, n)
	}
	return out, nil
}
