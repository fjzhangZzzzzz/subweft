package domain

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/fjzhangZzzzzz/subweft/internal/fields"
	"go.yaml.in/yaml/v3"
	"net/http"
	"reflect"
	"regexp"
	"strings"
)

// Node is the supported field set, not a passthrough for arbitrary core options.
// JSON uses the product contract; YAML uses the pinned Mihomo field names.
type Node struct {
	Options           map[string]any  `json:"options,omitempty" yaml:"-"`
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

func Protocols() []string { return fields.Protocols() }
func NodeFields(protocol string, yamlNames bool) map[string]bool {
	out := map[string]bool{}
	s := fields.Schema(protocol)
	if s == nil {
		return out
	}
	for k := range s.Properties {
		out[k] = true
	}
	if !yamlNames {
		for i := 0; i < reflect.TypeFor[Node]().NumField(); i++ {
			f := reflect.TypeFor[Node]().Field(i)
			j := strings.Split(f.Tag.Get("json"), ",")[0]
			y := strings.Split(f.Tag.Get("yaml"), ",")[0]
			if y != "-" && out[y] {
				delete(out, y)
				out[j] = true
			}
		}
		out["options"] = true
	}
	return out
}
func fieldError(e error) error {
	if e == nil {
		return nil
	}
	if x, ok := e.(*fields.Error); ok {
		return Invalid(x.Code, x.Path)
	}
	return Invalid("invalid_field_type", "node")
}
func (n *Node) UnmarshalJSON(data []byte) error {
	v, e := fields.DecodeJSON(data)
	if e != nil {
		return fieldError(e)
	}
	raw, ok := v.(map[string]any)
	if !ok {
		return Invalid("invalid_node", "node")
	}
	core := map[string]any{}
	typ := reflect.TypeFor[Node]()
	known := map[string]reflect.StructField{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		known[strings.Split(f.Tag.Get("json"), ",")[0]] = f
	}
	for key, v := range raw {
		if key == "options" {
			m, ok := v.(map[string]any)
			if !ok {
				return Invalid("invalid_field_type", "options")
			}
			for k, x := range m {
				if _, exists := core[k]; exists {
					return Invalid("duplicate_field", k)
				}
				core[k] = x
			}
			continue
		}
		f, ok := known[key]
		if !ok {
			return Invalid("unsupported_or_invalid_field", "node")
		}
		y := strings.Split(f.Tag.Get("yaml"), ",")[0]
		if _, exists := core[y]; exists {
			return Invalid("duplicate_field", y)
		}
		// Existing JSON names stay readable for development-preview databases.
		var value any
		bytes, _ := json.Marshal(v)
		ptr := reflect.New(f.Type)
		dec := json.NewDecoder(strings.NewReader(string(bytes)))
		dec.DisallowUnknownFields()
		if dec.Decode(ptr.Interface()) != nil {
			return Invalid("invalid_field_type", key)
		}
		value = ptr.Elem().Interface()
		encoded, _ := yaml.Marshal(value)
		var converted any
		if yaml.Unmarshal(encoded, &converted) != nil {
			return Invalid("invalid_field_type", key)
		}
		core[y] = converted
	}
	if core["type"] == "vmess" {
		if _, ok := core["alterId"]; !ok {
			core["alterId"] = 0
		}
	}
	value, e := nodeFromCore(core, false)
	if e != nil {
		return e
	}
	*n = value
	return nil
}
func NodeFromCore(core map[string]any) (Node, error) { return nodeFromCore(core, true) }
func nodeFromCore(core map[string]any, complete bool) (Node, error) {
	if core["type"] == "vmess" {
		if _, ok := core["alterId"]; !ok {
			core["alterId"] = 0
		}
		if _, ok := core["cipher"]; !ok {
			core["cipher"] = "auto"
		}
	}
	n := Node{Options: map[string]any{}}
	kind, _ := core["type"].(string)
	if e := fields.ValidateInput(kind, core, complete); e != nil {
		return n, fieldError(e)
	}
	value := reflect.ValueOf(&n).Elem()
	typ := value.Type()
	for key, v := range core {
		found := false
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			y := strings.Split(f.Tag.Get("yaml"), ",")[0]
			if y != key || y == "-" {
				continue
			}
			part := value.Field(i)
			if part.Kind() == reflect.Pointer || part.Kind() == reflect.Map || part.Kind() == reflect.Slice {
				b, _ := yaml.Marshal(fields.Normalize(v))
				ptr := reflect.New(f.Type)
				decoder := yaml.NewDecoder(strings.NewReader(string(b)))
				decoder.KnownFields(true)
				if decoder.Decode(ptr.Interface()) == nil {
					part.Set(ptr.Elem())
					found = true
				}
				break
			}
			b, _ := json.Marshal(v)
			if json.Unmarshal(b, part.Addr().Interface()) != nil {
				return n, Invalid("invalid_field_type", key)
			}
			found = true
			break
		}
		if !found {
			n.Options[key] = fields.Normalize(v)
		}
	}
	return n, nil
}
func (n *Node) UnmarshalYAML(value *yaml.Node) error {
	v, e := fields.YAMLValue(value)
	if e != nil {
		return fieldError(e)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return Invalid("invalid_node", "node")
	}
	out, e := NodeFromCore(m)
	if e == nil {
		*n = out
	}
	return e
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func safeText(s string, limit int) bool {
	return len(s) <= limit && !strings.ContainsAny(s, "\r\n\x00")
}
func ValidateNode(n Node) error {
	if !ValidName(n.Name) {
		return Invalid("invalid_name", "name")
	}
	if n.Type != "wireguard" && (n.Server == "" || len(n.Server) > 253 || strings.ContainsAny(n.Server, " /\r\n\x00")) {
		return Invalid("invalid_server", "server")
	}
	if n.Type != "wireguard" && !(n.Port == 0 && (n.Type == "hysteria" || n.Type == "hysteria2")) && (n.Port < 1 || n.Port > 65535) {
		return Invalid("invalid_port", "port")
	}
	core, e := n.Core()
	if e != nil {
		return e
	}
	if e = fields.Validate(n.Type, core); e != nil {
		return fieldError(e)
	}
	// Hydrate legacy semantic views while the complete nested fields remain in Options.
	hydrate := func(key string, dst any) {
		if v, ok := core[key]; ok {
			b, _ := yaml.Marshal(v)
			_ = yaml.Unmarshal(b, dst)
		}
	}
	if n.WS == nil {
		hydrate("ws-opts", &n.WS)
	}
	if n.GRPC == nil {
		hydrate("grpc-opts", &n.GRPC)
	}
	if n.Reality == nil {
		hydrate("reality-opts", &n.Reality)
	}
	if n.ALPN == nil {
		hydrate("alpn", &n.ALPN)
	}
	switch n.Type {
	case "ss":
		if n.Cipher == "" {
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
	case "ssr":
		if n.Password == "" || n.Cipher == "" {
			return Invalid("invalid_credentials", "password")
		}
	case "hysteria", "hysteria2", "snell", "anytls", "wireguard", "tuic":
		if n.Type == "tuic" {
			token, _ := core["token"].(string)
			if token == "" && (!uuidPattern.MatchString(n.UUID) || n.Password == "") {
				return Invalid("credentials_required", "uuid")
			}
		}
	default:
		return Invalid("unsupported_protocol", "type")
	}
	if len(n.Password) > 2048 || len(n.Username) > 1024 || !safeText(n.Username, 1024) {
		return Invalid("invalid_credentials", "credentials")
	}
	tls := n.TLS || strings.Contains("|trojan|anytls|hysteria|hysteria2|tuic|", "|"+n.Type+"|")
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
	if n.Network != "" && n.Network != "tcp" && n.Network != "ws" && n.Network != "grpc" && n.Network != "h2" && n.Network != "http" && n.Network != "xhttp" && n.Network != "mkcp" && n.Network != "mekya" && n.Network != "tlsmirror" {
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
		if err != nil || len(key) != 32 || shortErr != nil || len(short) > 8 || n.ServerName == "" && n.SNI == "" || n.ClientFingerprint == "" || n.SkipCertVerify {
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
func (n Node) Core() (map[string]any, error) {
	out := map[string]any{}
	value := reflect.ValueOf(n)
	typ := value.Type()
	for i := 0; i < value.NumField(); i++ {
		f := typ.Field(i)
		tag := strings.Split(f.Tag.Get("yaml"), ",")
		if tag[0] == "-" {
			continue
		}
		part := value.Field(i)
		if len(tag) > 1 && tag[1] == "omitempty" && part.IsZero() && !(f.Name == "AlterID" && n.Type == "vmess") {
			continue
		}
		if (f.Name == "Server" || f.Name == "Port") && part.IsZero() && (n.Type == "wireguard" || f.Name == "Port" && (n.Type == "hysteria" || n.Type == "hysteria2")) {
			continue
		}
		if part.Kind() != reflect.Pointer && part.Kind() != reflect.Map && part.Kind() != reflect.Slice && part.Kind() != reflect.Struct {
			out[tag[0]] = part.Interface()
			continue
		}
		b, _ := yaml.Marshal(part.Interface())
		var scalar any
		if e := yaml.Unmarshal(b, &scalar); e != nil {
			return nil, Invalid("invalid_field_type", tag[0])
		}
		out[tag[0]] = scalar
	}
	for k, v := range n.Options {
		if _, ok := out[k]; ok {
			return nil, Invalid("duplicate_field", k)
		}
		out[k] = fields.Normalize(v)
	}
	return out, nil
}
func (n Node) MarshalYAML() (any, error) { return n.Core() }
