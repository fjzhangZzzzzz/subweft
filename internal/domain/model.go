package domain

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrConflict = errors.New("conflict")
var ErrNotFound = errors.New("not_found")

type Error struct {
	Code  string `json:"code"`
	Field string `json:"field,omitempty"`
}

func (e *Error) Error() string         { return e.Code }
func Invalid(code, field string) error { return &Error{Code: code, Field: field} }

type Meta struct {
	ID       string `json:"id"`
	Revision int    `json:"revision"`
}
type Node struct {
	Name     string `json:"name" yaml:"name"`
	Type     string `json:"type" yaml:"type"`
	Server   string `json:"server" yaml:"server"`
	Port     int    `json:"port" yaml:"port"`
	Cipher   string `json:"cipher" yaml:"cipher"`
	Password string `json:"password" yaml:"password"`
	UDP      bool   `json:"udp" yaml:"udp"`
}
type ManualNode struct {
	Meta
	Node    Node `json:"node"`
	Enabled bool `json:"enabled"`
}
type Source struct {
	Meta
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	URL         string    `json:"url,omitempty"`
	Content     string    `json:"content,omitempty"`
	Enabled     bool      `json:"enabled"`
	Tags        []string  `json:"tags"`
	Interval    int       `json:"interval"`
	Nodes       []Node    `json:"nodes,omitempty"`
	LastSuccess time.Time `json:"lastSuccess"`
	LastAttempt time.Time `json:"lastAttempt"`
	LastError   string    `json:"lastError,omitempty"`
}
type Group struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	SourceIDs  []string `json:"sourceIds"`
	Tags       []string `json:"tags"`
	AllSources bool     `json:"allSources"`
	NodeIDs    []string `json:"nodeIds"`
	Groups     []string `json:"groups"`
	Builtins   []string `json:"builtins"`
	Filter     string   `json:"filter,omitempty"`
	Exclude    string   `json:"exclude,omitempty"`
	URL        string   `json:"url,omitempty"`
	Interval   int      `json:"interval"`
	Tolerance  int      `json:"tolerance"`
	Strategy   string   `json:"strategy,omitempty"`
}
type Rule struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Target    string `json:"target"`
	NoResolve bool   `json:"noResolve"`
	Enabled   bool   `json:"enabled"`
}
type Settings struct {
	MixedPort   int      `json:"mixedPort"`
	AllowLAN    bool     `json:"allowLan"`
	IPv6        bool     `json:"ipv6"`
	DNSEnabled  bool     `json:"dnsEnabled"`
	DNSMode     string   `json:"dnsMode"`
	Nameservers []string `json:"nameservers"`
}
type Profile struct {
	Meta
	Name     string   `json:"name"`
	Groups   []Group  `json:"groups"`
	Rules    []Rule   `json:"rules"`
	Settings Settings `json:"settings"`
}
type Device struct {
	Meta
	Name      string     `json:"name"`
	ProfileID string     `json:"profileId"`
	Owner     string     `json:"owner"`
	State     string     `json:"state"`
	Token     string     `json:"token,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}
type Release struct {
	Meta
	ProfileID       string            `json:"profileId"`
	ProfileRevision int               `json:"profileRevision"`
	CreatedAt       time.Time         `json:"createdAt"`
	Config          []byte            `json:"config,omitempty"`
	Digest          string            `json:"digest"`
	Sources         map[string]string `json:"sources"`
	Validator       string            `json:"validator"`
}
type Snapshot struct {
	Generation int64
	Sources    []Source
	Nodes      []ManualNode
	Profiles   []Profile
	Devices    []Device
	Releases   []Release
	Active     map[string]string
}

func RandomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func NewID() string { return RandomToken()[:22] }
func ValidName(name string) bool {
	return name != "" && len(name) <= 160 && name == strings.TrimSpace(name) && !strings.ContainsAny(name, ",\r\n\x00")
}
func ValidateNode(n Node) error {
	if !ValidName(n.Name) {
		return Invalid("invalid_name", "name")
	}
	if n.Type != "ss" {
		return Invalid("unsupported_protocol", "type")
	}
	if n.Server == "" || len(n.Server) > 253 || strings.ContainsAny(n.Server, " /\r\n\x00") {
		return Invalid("invalid_server", "server")
	}
	if n.Port < 1 || n.Port > 65535 {
		return Invalid("invalid_port", "port")
	}
	switch n.Cipher {
	case "aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305":
	default:
		return Invalid("unsupported_cipher", "cipher")
	}
	if n.Password == "" || len(n.Password) > 2048 {
		return Invalid("invalid_password", "password")
	}
	return nil
}
func ValidateSource(s Source) error {
	if !ValidName(s.Name) {
		return Invalid("invalid_name", "name")
	}
	if s.Kind != "http" && s.Kind != "local" {
		return Invalid("invalid_source_kind", "kind")
	}
	if s.Interval < 60 || s.Interval > 604800 {
		return Invalid("invalid_interval", "interval")
	}
	if len(s.Tags) > 32 {
		return Invalid("too_many_tags", "tags")
	}
	for _, t := range s.Tags {
		if !ValidName(t) {
			return Invalid("invalid_tag", "tags")
		}
	}
	if s.Kind == "http" && (len(s.URL) > 8192 || s.URL == "") {
		return Invalid("invalid_url", "url")
	}
	if len(s.Content) > 2<<20 {
		return Invalid("input_too_large", "content")
	}
	return nil
}
func ValidateProfile(p Profile) error {
	if !ValidName(p.Name) {
		return Invalid("invalid_name", "name")
	}
	if len(p.Groups) > 100 || len(p.Rules) > 5000 {
		return Invalid("too_many_items", "profile")
	}
	if p.Settings.MixedPort < 1 || p.Settings.MixedPort > 65535 {
		return Invalid("invalid_port", "mixedPort")
	}
	return nil
}
func Field(prefix string, err error) error {
	var e *Error
	if errors.As(err, &e) {
		return Invalid(e.Code, fmt.Sprintf("%s.%s", prefix, e.Field))
	}
	return err
}
