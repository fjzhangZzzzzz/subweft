package domain

import "time"

// Resource caches are content-addressed encrypted assets; definitions stay small.
type Resource struct {
	Headers map[string]string `json:"headers,omitempty"`
	Meta
	Name        string    `json:"name"`
	Kind        string    `json:"kind"` // local or http
	Type        string    `json:"type"` // ruleset, mmdb, geoip, geosite
	Behavior    string    `json:"behavior,omitempty"`
	Format      string    `json:"format,omitempty"`
	URL         string    `json:"url,omitempty"`
	Enabled     bool      `json:"enabled"`
	Interval    int       `json:"interval"`
	Digest      string    `json:"digest,omitempty"`
	Count       int       `json:"count"`
	LastSuccess time.Time `json:"lastSuccess"`
	LastAttempt time.Time `json:"lastAttempt"`
	LastError   string    `json:"lastError,omitempty"`
}
type Condition struct {
	Type      string      `json:"type"`
	Value     string      `json:"value"`
	NoResolve bool        `json:"noResolve"`
	Children  []Condition `json:"children,omitempty"`
}
