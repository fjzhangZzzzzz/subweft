// Package configuration defines the single configuration editor accepted in ADR-0008.
// It intentionally has no upstream acquisition or manually maintained proxy nodes.
package configuration

type Draft struct {
	Revision int    `json:"revision"`
	Config   Config `json:"config"`
}

type Config struct {
	Subscriptions []Subscription `json:"subscriptions"`
	Groups        []Group        `json:"groups"`
	RuleSets      []RuleSet      `json:"ruleSets"`
	Rules         []Rule         `json:"rules"`
	Fallback      string         `json:"fallback"`
	Base          map[string]any `json:"base"`
}

type Subscription struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Pool        string `json:"pool"`
	Enabled     bool   `json:"enabled"`
	Interval    int    `json:"interval"`
	Prefix      string `json:"prefix"`
	HealthCheck bool   `json:"healthCheck"`
	Proxy       string `json:"proxy"`
}

type Group struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Pools     []string `json:"pools"`
	Builtins  []string `json:"builtins"`
	URL       string   `json:"url"`
	Interval  int      `json:"interval"`
	Tolerance int      `json:"tolerance"`
}

type RuleSet struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Entries  []Entry `json:"entries"`
	URL      string  `json:"url,omitempty"`
	Behavior string  `json:"behavior,omitempty"`
	Interval int     `json:"interval,omitempty"`
}

type Entry struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Rule struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Value     string `json:"value"`
	Target    string `json:"target"`
	NoResolve bool   `json:"noResolve"`
}
