package compile

import (
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

func validateDependencies(nodes []domain.Node, members map[string][]string, fetch map[string][]string) error {
	graph := map[string][]string{}
	for name, values := range members {
		graph[name] = append(append([]string{}, values...), fetch[name]...)
	}
	for _, n := range nodes {
		graph[n.Name] = nil
		if proxy, ok := n.Options["dialer-proxy"].(string); ok && proxy != "" {
			graph[n.Name] = []string{proxy}
		}
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var walk func(string) error
	walk = func(name string) error {
		if name == "DIRECT" || name == "REJECT" {
			return nil
		}
		edges, ok := graph[name]
		if !ok {
			return domain.Invalid("proxy_dependency_missing", "groups")
		}
		if visiting[name] {
			return domain.Invalid("proxy_dependency_cycle", "groups")
		}
		if visited[name] {
			return nil
		}
		visiting[name] = true
		for _, next := range edges {
			if e := walk(next); e != nil {
				return e
			}
		}
		visiting[name] = false
		visited[name] = true
		return nil
	}
	for name := range graph {
		if e := walk(name); e != nil {
			return e
		}
	}
	return nil
}
