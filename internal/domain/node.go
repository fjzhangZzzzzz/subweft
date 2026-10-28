package domain

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"strings"
)

// Node is the supported field set, not a passthrough for arbitrary core options.
// JSON uses the product contract; YAML uses the pinned Mihomo field names.
type Node struct {
	Name              string          `json:"name" yaml:"name"`
	Type              string          `json:"type" yaml:"type"`
	Server            string          `json:"server" yaml:"server"`
	Port              int             `json:"port" yaml:"port"`
	Cipher            string          `json:"cipher,omitempty" yaml:"cipher,omitempty"`
	Password          string          `json:"password,omitempty" yaml:"password,omitempty"`
	Username          string          `json:"username,omitempty" yaml:"username,omitempty"`
	UUID              string          `json:"uuid,omitempty" yaml:"uuid,omitempty"`
	AlterID           int             `json:"alterId,omitempty" yaml:"alterId,omitempty"`
	UDP               bool            `json:"udp,omitempty" yaml:"udp,omitempty"`
	TLS               bool            `json:"tls,omitempty" yaml:"tls,omitempty"`
	SkipCertVerify    bool            `json:"skipCertVerify,omitempty" yaml:"skip-cert-verify,omitempty"`
	ServerName        string          `json:"serverName,omitempty" yaml:"servername,omitempty"`
	SNI               string          `json:"sni,omitempty" yaml:"sni,omitempty"`
	ALPN              *[]string       `json:"alpn,omitempty" yaml:"alpn,omitempty"`
	ClientFingerprint string          `json:"clientFingerprint,omitempty" yaml:"client-fingerprint,omitempty"`
	Network           string          `json:"network,omitempty" yaml:"network,omitempty"`
	Flow              string          `json:"flow,omitempty" yaml:"flow,omitempty"`
	Reality           *RealityOptions `json:"realityOpts,omitempty" yaml:"reality-opts,omitempty"`
	WS                *WSOptions      `json:"wsOpts,omitempty" yaml:"ws-opts,omitempty"`
	GRPC              *GRPCOptions    `json:"grpcOpts,omitempty" yaml:"grpc-opts,omitempty"`
}
type RealityOptions struct {
	PublicKey string `json:"publicKey" yaml:"public-key"`
	ShortID   string `json:"shortId" yaml:"short-id"`
}
type WSOptions struct {
	Path                string            `json:"path" yaml:"path,omitempty"`
	Headers             map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	MaxEarlyData        int               `json:"maxEarlyData" yaml:"max-early-data,omitempty"`
	EarlyDataHeaderName string            `json:"earlyDataHeaderName" yaml:"early-data-header-name,omitempty"`
}
type GRPCOptions struct {
	ServiceName string `json:"serviceName" yaml:"grpc-service-name"`
}

func Protocols() []string { return []string{"ss", "vmess", "vless", "trojan", "http", "socks5"} }

// This same policy applies to imported YAML and management JSON, including
// explicitly supplied zero/false values that a value-only check would miss.
func NodeFields(protocol string, yaml bool) map[string]bool {
	fields := map[string]bool{"name": true, "type": true, "server": true, "port": true}
	add := func(jsonName, yamlName string) {
		if yaml {
			fields[yamlName] = true
		} else {
			fields[jsonName] = true
		}
	}
	switch protocol {
	case "ss":
		add("cipher", "cipher")
		add("password", "password")
		add("udp", "udp")
	case "vmess", "vless", "trojan":
		add("udp", "udp")
		add("network", "network")
		add("skipCertVerify", "skip-cert-verify")
		add("alpn", "alpn")
		add("clientFingerprint", "client-fingerprint")
		add("wsOpts", "ws-opts")
		add("grpcOpts", "grpc-opts")
		if protocol == "trojan" {
			add("password", "password")
			add("sni", "sni")
		} else {
			add("uuid", "uuid")
			add("tls", "tls")
			add("serverName", "servername")
		}
		if protocol == "vmess" {
			add("cipher", "cipher")
			add("alterId", "alterId")
		}
		if protocol == "vless" {
			add("flow", "flow")
			add("realityOpts", "reality-opts")
		}
	case "http", "socks5":
		add("username", "username")
		add("password", "password")
		add("tls", "tls")
		add("skipCertVerify", "skip-cert-verify")
		if protocol == "http" {
			add("sni", "sni")
		} else {
			add("udp", "udp")
		}
	}
	return fields
}

func (n *Node) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil || raw == nil || hasJSONNull(data) {
		return Invalid("invalid_node", "node")
	}
	var kind string
	if json.Unmarshal(raw["type"], &kind) != nil {
		return Invalid("unsupported_protocol", "type")
	}
	allowed := NodeFields(kind, false)
	supported := false
	for _, p := range Protocols() {
		if p == kind {
			supported = true
		}
	}
	if !supported {
		return Invalid("unsupported_protocol", "type")
	}
	for key, value := range raw {
		if !allowed[key] {
			return Invalid("unsupported_or_invalid_field", "node")
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return Invalid("invalid_field_type", "node")
		}
	}
	type plain Node
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value plain
	if decoder.Decode(&value) != nil {
		return Invalid("invalid_field_type", "node")
	}
	*n = Node(value)
	return nil
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func safeText(s string, limit int) bool {
	return len(s) <= limit && !strings.ContainsAny(s, "\r\n\x00")
}
func ValidateNode(n Node) error {
	if !ValidName(n.Name) {
		return Invalid("invalid_name", "name")
	}
	if n.Server == "" || len(n.Server) > 253 || strings.ContainsAny(n.Server, " /\r\n\x00") {
		return Invalid("invalid_server", "server")
	}
	if n.Port < 1 || n.Port > 65535 {
		return Invalid("invalid_port", "port")
	}
	// Reject nonzero unsupported fields even for callers constructing Go values.
	encoded, _ := json.Marshal(n)
	var present map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &present)
	allowed := NodeFields(n.Type, false)
	for field, value := range present {
		if field == "udp" && bytes.Equal(value, []byte("false")) {
			continue
		}
		if !allowed[field] {
			return Invalid("unsupported_or_invalid_field", "node")
		}
	}
	switch n.Type {
	case "ss":
		switch n.Cipher {
		case "aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305":
		default:
			return Invalid("unsupported_cipher", "cipher")
		}
		if n.Password == "" {
			return Invalid("invalid_password", "password")
		}
	case "vmess", "vless":
		if !uuidPattern.MatchString(n.UUID) {
			return Invalid("invalid_uuid", "uuid")
		}
		if n.Type == "vmess" {
			switch n.Cipher {
			case "auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero":
			default:
				return Invalid("unsupported_cipher", "cipher")
			}
			if n.AlterID < 0 || n.AlterID > 65535 {
				return Invalid("invalid_alter_id", "alterId")
			}
		}
	case "trojan":
		if n.Password == "" {
			return Invalid("invalid_password", "password")
		}
	case "http", "socks5":
		if (n.Username == "") != (n.Password == "") {
			return Invalid("credentials_pair_required", "username")
		}
	default:
		return Invalid("unsupported_protocol", "type")
	}
	if len(n.Password) > 2048 || len(n.Username) > 1024 || !safeText(n.Username, 1024) {
		return Invalid("invalid_credentials", "credentials")
	}
	tls := n.TLS || n.Type == "trojan"
	if !tls && (n.SkipCertVerify || n.ServerName != "" || n.SNI != "" || n.ALPN != nil || n.ClientFingerprint != "" || n.Reality != nil) {
		return Invalid("tls_required", "tls")
	}
	if !safeText(n.ServerName, 253) || !safeText(n.SNI, 253) {
		return Invalid("invalid_server_name", "serverName")
	}
	if n.ALPN != nil {
		if len(*n.ALPN) > 8 {
			return Invalid("invalid_alpn", "alpn")
		}
		for _, v := range *n.ALPN {
			if v == "" || !safeText(v, 255) {
				return Invalid("invalid_alpn", "alpn")
			}
		}
	}
	switch n.ClientFingerprint {
	case "", "chrome", "firefox", "safari", "ios", "android", "edge", "random", "randomized":
	default:
		return Invalid("unsupported_fingerprint", "clientFingerprint")
	}
	if n.Network != "" && n.Network != "tcp" && n.Network != "ws" && n.Network != "grpc" {
		return Invalid("unsupported_transport", "network")
	}
	if n.WS != nil && n.Network != "ws" || n.GRPC != nil && n.Network != "grpc" {
		return Invalid("transport_options_mismatch", "network")
	}
	if n.WS != nil {
		if !safeText(n.WS.Path, 2048) || (n.WS.Path != "" && !strings.HasPrefix(n.WS.Path, "/")) || n.WS.MaxEarlyData < 0 || n.WS.MaxEarlyData > 65536 || (!safeText(n.WS.EarlyDataHeaderName, 128) || n.WS.EarlyDataHeaderName != "" && !headerToken(n.WS.EarlyDataHeaderName)) {
			return Invalid("invalid_websocket", "wsOpts")
		}
		if len(n.WS.Headers) > 32 {
			return Invalid("too_many_headers", "wsOpts.headers")
		}
		seenHeaders := map[string]bool{}
		for key, value := range n.WS.Headers {
			canonical := strings.ToLower(key)
			if seenHeaders[canonical] {
				return Invalid("duplicate_header", "wsOpts.headers")
			}
			seenHeaders[canonical] = true
			if !headerToken(key) || !safeText(value, 2048) || http.CanonicalHeaderKey(key) == "" {
				return Invalid("invalid_header", "wsOpts.headers")
			}
		}
	}
	if n.GRPC != nil && !safeText(n.GRPC.ServiceName, 512) {
		return Invalid("invalid_grpc_service", "grpcOpts.serviceName")
	}
	if n.Flow != "" && (n.Flow != "xtls-rprx-vision" || !tls || (n.Network != "" && n.Network != "tcp")) {
		return Invalid("invalid_flow", "flow")
	}
	if n.Reality != nil {
		key, err := base64.RawURLEncoding.DecodeString(n.Reality.PublicKey)
		short, shortErr := hex.DecodeString(n.Reality.ShortID)
		if err != nil || len(key) != 32 || shortErr != nil || len(short) > 8 || n.ServerName == "" || n.ClientFingerprint == "" || n.SkipCertVerify {
			return Invalid("invalid_reality", "realityOpts")
		}
	}
	return nil
}
func headerToken(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if c > 127 || !strings.ContainsRune("!#$%&'*+-.^_`|~0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", c) {
			return false
		}
	}
	return true
}

// Metadata is safe for broad list responses. Editing credentials is explicit.
func NodeMetadata(n Node) Node {
	return Node{Name: n.Name, Type: n.Type, Server: n.Server, Port: n.Port, UDP: n.UDP}
}

func hasJSONNull(data []byte) bool {
	var v any
	if json.Unmarshal(data, &v) != nil {
		return true
	}
	var walk func(any) bool
	walk = func(x any) bool {
		switch a := x.(type) {
		case nil:
			return true
		case []any:
			for _, y := range a {
				if walk(y) {
					return true
				}
			}
		case map[string]any:
			for _, y := range a {
				if walk(y) {
					return true
				}
			}
		}
		return false
	}
	return walk(v)
}

// Preserve an explicitly empty ALPN list and required VMess zero values.
func (n Node) MarshalYAML() (any, error) {
	out := map[string]any{}
	value := reflect.ValueOf(n)
	kind := value.Type()
	for i := 0; i < value.NumField(); i++ {
		field := kind.Field(i)
		tag := strings.Split(field.Tag.Get("yaml"), ",")
		part := value.Field(i)
		if len(tag) > 1 && tag[1] == "omitempty" && part.IsZero() && !(field.Name == "AlterID" && n.Type == "vmess") {
			continue
		}
		out[tag[0]] = part.Interface()
	}
	return out, nil
}
