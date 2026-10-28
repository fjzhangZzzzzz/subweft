package source

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

func decode64(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		if b, e := enc.DecodeString(s); e == nil {
			return b, nil
		}
	}
	return nil, errors.New("invalid_base64")
}
func parseURIs(input string) ([]domain.Node, error) {
	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Buffer(make([]byte, 4096), MaxBytes)
	nodes := []domain.Node{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		n, e := parseURI(line)
		if e != nil {
			return nil, domain.Field("nodes."+strconv.Itoa(len(nodes)), e)
		}
		nodes = append(nodes, n)
		if len(nodes) > MaxNodes {
			return nil, domain.Invalid("invalid_node_count", "content")
		}
	}
	if scanner.Err() != nil {
		return nil, domain.Invalid("input_too_large", "content")
	}
	return nodes, nil
}
func parseURI(line string) (domain.Node, error) {
	invalid := func() (domain.Node, error) {
		return domain.Node{}, domain.Invalid("unsupported_or_invalid_uri", "content")
	}
	if strings.HasPrefix(line, "vmess://") {
		return parseVMess(strings.TrimPrefix(line, "vmess://"))
	}
	u, e := url.Parse(line)
	if e != nil || u.Hostname() == "" {
		return invalid()
	}
	query, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return invalid()
	}
	for _, values := range query {
		if len(values) != 1 {
			return invalid()
		}
	}
	n := domain.Node{Type: u.Scheme, Server: u.Hostname(), Name: u.Fragment, UDP: u.Scheme != "http" && u.Scheme != "https"}
	if n.Name == "" {
		n.Name = u.Host
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		case "socks5":
			port = "1080"
		default:
			return invalid()
		}
	}
	n.Port, e = strconv.Atoi(port)
	if e != nil || u.Path != "" && u.Path != "/" {
		return invalid()
	}
	switch u.Scheme {
	case "ss":
		if u.User == nil || len(query) > 0 {
			return invalid()
		}
		auth := ""
		if password, ok := u.User.Password(); ok {
			auth = u.User.Username() + ":" + password
		} else {
			b, e := decode64(u.User.Username())
			if e != nil {
				return invalid()
			}
			auth = string(b)
		}
		var ok bool
		n.Cipher, n.Password, ok = strings.Cut(auth, ":")
		if !ok {
			return invalid()
		}
	case "http", "https", "socks5":
		if len(query) > 0 {
			return invalid()
		}
		if u.User != nil {
			n.Username = u.User.Username()
			var ok bool
			n.Password, ok = u.User.Password()
			if !ok {
				return invalid()
			}
		}
		if u.Scheme == "https" {
			n.Type = "http"
			n.TLS = true
		}
	case "vless", "trojan":
		if u.User == nil {
			return invalid()
		}
		if u.Scheme == "vless" {
			if _, ok := u.User.Password(); ok {
				return invalid()
			}
			n.UUID = u.User.Username()
		} else {
			n.Password = u.User.Username()
			if p, ok := u.User.Password(); ok {
				n.Password += ":" + p
			}
		}
		allowed := map[string]bool{"type": true, "security": true, "sni": true, "fp": true, "alpn": true, "path": true, "host": true, "serviceName": true, "allowInsecure": true}
		if u.Scheme == "vless" {
			for _, key := range []string{"encryption", "flow", "pbk", "sid"} {
				allowed[key] = true
			}
		}
		for key := range query {
			if !allowed[key] {
				return invalid()
			}
		}
		n.Network = query.Get("type")
		if n.Network == "" {
			n.Network = "tcp"
		}
		security := query.Get("security")
		if u.Scheme == "trojan" {
			if security != "" && security != "tls" {
				return invalid()
			}
		} else {
			switch security {
			case "", "none":
			case "tls", "reality":
				n.TLS = true
			default:
				return invalid()
			}
			if encryption := query.Get("encryption"); encryption != "" && encryption != "none" {
				return invalid()
			}
			n.Flow = query.Get("flow")
		}
		if u.Scheme == "trojan" {
			n.SNI = query.Get("sni")
		} else {
			n.ServerName = query.Get("sni")
		}
		n.ClientFingerprint = query.Get("fp")
		if alpn := query.Get("alpn"); alpn != "" {
			values := strings.Split(alpn, ",")
			n.ALPN = &values
		}
		switch query.Get("allowInsecure") {
		case "", "0", "false":
		case "1", "true":
			n.SkipCertVerify = true
		default:
			return invalid()
		}
		if security == "reality" {
			n.Reality = &domain.RealityOptions{PublicKey: query.Get("pbk"), ShortID: query.Get("sid")}
		} else if query.Has("pbk") || query.Has("sid") {
			return invalid()
		}
		switch n.Network {
		case "ws":
			if query.Has("serviceName") {
				return invalid()
			}
			n.WS = &domain.WSOptions{Path: query.Get("path")}
			if query.Has("host") {
				n.WS.Headers = map[string]string{"Host": query.Get("host")}
			}
		case "grpc":
			if query.Has("path") || query.Has("host") {
				return invalid()
			}
			n.GRPC = &domain.GRPCOptions{ServiceName: query.Get("serviceName")}
		case "tcp":
			if query.Has("path") || query.Has("host") || query.Has("serviceName") {
				return invalid()
			}
		default:
			return invalid()
		}
	default:
		return invalid()
	}
	return n, nil
}

// The widely used VMess JSON URI has numeric strings for port/aid. This
// format-specific conversion does not relax the provider YAML or API schema.
func parseVMess(encoded string) (domain.Node, error) {
	invalid := func() (domain.Node, error) {
		return domain.Node{}, domain.Invalid("unsupported_or_invalid_uri", "content")
	}
	b, e := decode64(encoded)
	if e != nil {
		return invalid()
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(b, &values) != nil || values == nil {
		return invalid()
	}
	allowed := map[string]bool{}
	for _, key := range []string{"v", "ps", "add", "port", "id", "aid", "scy", "net", "type", "host", "path", "tls", "sni", "alpn", "fp"} {
		allowed[key] = true
	}
	for key, raw := range values {
		if string(raw) == "null" || !allowed[key] {
			return invalid()
		}
	}
	text := func(key string) (string, bool) {
		raw, ok := values[key]
		if !ok {
			return "", true
		}
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return "", false
		}
		return s, true
	}
	get := func(key string) string { s, _ := text(key); return s }
	for _, key := range []string{"ps", "add", "id", "scy", "net", "type", "host", "path", "tls", "sni", "alpn", "fp"} {
		if _, ok := text(key); !ok {
			return invalid()
		}
	}
	number := func(key string, defaultValue int) (int, error) {
		raw, ok := values[key]
		if !ok {
			return defaultValue, nil
		}
		var v int
		if json.Unmarshal(raw, &v) == nil {
			return v, nil
		}
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return 0, errors.New("invalid_number")
		}
		return strconv.Atoi(s)
	}
	if v, ok := values["v"]; ok && string(v) != "2" && string(v) != "\"2\"" {
		return invalid()
	}
	port, e := number("port", 0)
	if e != nil {
		return invalid()
	}
	aid, e := number("aid", 0)
	if e != nil {
		return invalid()
	}
	n := domain.Node{Name: get("ps"), Type: "vmess", Server: get("add"), Port: port, UUID: get("id"), AlterID: aid, Cipher: get("scy"), UDP: true, Network: get("net"), ServerName: get("sni"), ClientFingerprint: get("fp")}
	if n.Name == "" {
		n.Name = n.Server
	}
	if n.Cipher == "" {
		n.Cipher = "auto"
	}
	if n.Network == "" {
		n.Network = "tcp"
	}
	if get("type") != "" && get("type") != "none" {
		return invalid()
	}
	switch get("tls") {
	case "", "none":
	case "tls":
		n.TLS = true
	default:
		return invalid()
	}
	if get("alpn") != "" {
		values := strings.Split(get("alpn"), ",")
		n.ALPN = &values
	}
	switch n.Network {
	case "ws":
		n.WS = &domain.WSOptions{Path: get("path")}
		if get("host") != "" {
			n.WS.Headers = map[string]string{"Host": get("host")}
		}
	case "grpc":
		if get("host") != "" {
			return invalid()
		}
		n.GRPC = &domain.GRPCOptions{ServiceName: get("path")}
	case "tcp":
		if get("host") != "" || get("path") != "" {
			return invalid()
		}
	default:
		return invalid()
	}
	return n, nil
}
