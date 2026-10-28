package source

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"go.yaml.in/yaml/v3"
)

const MaxBytes = 2 << 20
const MaxNodes = 5000

// Every DNS answer is checked, then the chosen IP is dialed directly. TLS still verifies the URL host.
type Fetcher struct{ Client *http.Client }

func NewFetcher(allowed []netip.Prefix) *Fetcher {
	allowedIP := func(ip netip.Addr) bool {
		ip = ip.Unmap()
		for _, p := range allowed {
			if p.Contains(ip) {
				return true
			}
		}
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			return false
		}
		for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "64:ff9b::/96", "64:ff9b:1::/48"} {
			if netip.MustParsePrefix(cidr).Contains(ip) {
				return false
			}
		}
		return true
	}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, MaxIdleConns: 4, MaxConnsPerHost: 2, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 10 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, errors.New("blocked_destination")
		}
		ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if e != nil || len(ips) == 0 {
			return nil, errors.New("dns_failed")
		}
		for _, ip := range ips {
			if !allowedIP(ip) {
				return nil, errors.New("blocked_destination")
			}
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	return &Fetcher{Client: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too_many_redirects")
		}
		if e := ValidateURL(req.URL.String()); e != nil {
			return e
		}
		if req.URL.Scheme != via[0].URL.Scheme {
			return errors.New("scheme_redirect_rejected")
		}
		return nil
	}}}
}
func ValidateURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return domain.Invalid("invalid_url", "url")
	}
	if p := u.Port(); p != "" {
		v, e := strconv.Atoi(p)
		if e != nil || v < 1 || v > 65535 {
			return domain.Invalid("invalid_url", "url")
		}
	}
	return nil
}
func (f *Fetcher) Fetch(ctx context.Context, raw string) ([]byte, error) {
	if e := ValidateURL(raw); e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		return nil, domain.Invalid("invalid_url", "url")
	}
	req.Header.Set("User-Agent", "Subweft/0.1-dev")
	resp, e := f.Client.Do(req)
	if e != nil {
		return nil, domain.Invalid("fetch_failed", "url")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, domain.Invalid("upstream_status", "url")
	}
	if resp.ContentLength > MaxBytes {
		return nil, domain.Invalid("input_too_large", "url")
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if e != nil {
		return nil, domain.Invalid("fetch_failed", "url")
	}
	if len(b) > MaxBytes {
		return nil, domain.Invalid("input_too_large", "url")
	}
	return b, nil
}
func Parse(b []byte) ([]domain.Node, error) {
	if len(b) > MaxBytes {
		return nil, domain.Invalid("input_too_large", "content")
	}
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil, domain.Invalid("empty_source", "content")
	}
	var nodes []domain.Node
	if bytes.HasPrefix(b, []byte("ss://")) {
		var e error
		nodes, e = parseURIs(string(b))
		if e != nil {
			return nil, e
		}
	} else {
		var root yaml.Node
		d := yaml.NewDecoder(bytes.NewReader(b))
		if e := d.Decode(&root); e != nil {
			return nil, domain.Invalid("invalid_yaml", "content")
		}
		if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
			return nil, domain.Invalid("invalid_provider", "content")
		}
		mapping := root.Content[0]
		if len(mapping.Content) != 2 || mapping.Content[0].Value != "proxies" {
			return nil, domain.Invalid("unsupported_root_field", "content")
		}
		if mapping.Content[1].Kind != yaml.SequenceNode || len(mapping.Content[1].Content) > MaxNodes {
			return nil, domain.Invalid("invalid_node_count", "content")
		}
		if hasAlias(&root, 0) {
			return nil, domain.Invalid("yaml_alias_rejected", "content")
		}
		for i, node := range mapping.Content[1].Content {
			if node.Kind != yaml.MappingNode {
				return nil, domain.Invalid("invalid_node", "nodes."+strconv.Itoa(i))
			}
			for k := 0; k < len(node.Content); k += 2 {
				key, value := node.Content[k].Value, node.Content[k+1]
				tag := "!!str"
				switch key {
				case "port":
					tag = "!!int"
				case "udp":
					tag = "!!bool"
				case "name", "type", "server", "cipher", "password":
				default:
					return nil, domain.Invalid("unsupported_or_invalid_field", "nodes."+strconv.Itoa(i))
				}
				if value.Kind != yaml.ScalarNode || value.Tag != tag {
					return nil, domain.Invalid("invalid_field_type", "nodes."+strconv.Itoa(i)+"."+key)
				}
			}
		}
		var provider struct {
			Proxies []domain.Node `yaml:"proxies"`
		}
		strict := yaml.NewDecoder(bytes.NewReader(b))
		strict.KnownFields(true)
		if e := strict.Decode(&provider); e != nil {
			return nil, domain.Invalid("unsupported_or_invalid_field", "content")
		}
		var extra any
		if e := strict.Decode(&extra); e != io.EOF {
			return nil, domain.Invalid("multiple_documents", "content")
		}
		nodes = provider.Proxies
	}
	if len(nodes) == 0 || len(nodes) > MaxNodes {
		return nil, domain.Invalid("invalid_node_count", "content")
	}
	seen := map[string]bool{}
	for i, n := range nodes {
		if e := domain.ValidateNode(n); e != nil {
			return nil, domain.Field("nodes."+strconv.Itoa(i), e)
		}
		if seen[n.Name] {
			return nil, domain.Invalid("duplicate_node_name", "content")
		}
		seen[n.Name] = true
	}
	return nodes, nil
}
func hasAlias(n *yaml.Node, depth int) bool {
	if n.Kind == yaml.AliasNode || depth > 16 {
		return true
	}
	for _, c := range n.Content {
		if hasAlias(c, depth+1) {
			return true
		}
	}
	return false
}
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
		u, e := url.Parse(line)
		if e != nil || u.Scheme != "ss" || u.User == nil || u.RawQuery != "" {
			return nil, domain.Invalid("unsupported_uri", "content")
		}
		var auth string
		if password, ok := u.User.Password(); ok {
			auth = u.User.Username() + ":" + password
		} else {
			b, e := decode64(u.User.Username())
			if e != nil {
				return nil, domain.Invalid("invalid_uri", "content")
			}
			auth = string(b)
		}
		method, password, ok := strings.Cut(auth, ":")
		port, e := strconv.Atoi(u.Port())
		if !ok || e != nil {
			return nil, domain.Invalid("invalid_uri", "content")
		}
		name := u.Fragment
		if name == "" {
			name = u.Host
		}
		nodes = append(nodes, domain.Node{Name: name, Type: "ss", Server: u.Hostname(), Port: port, Cipher: method, Password: password, UDP: true})
		if len(nodes) > MaxNodes {
			return nil, domain.Invalid("invalid_node_count", "content")
		}
	}
	if scanner.Err() != nil {
		return nil, domain.Invalid("input_too_large", "content")
	}
	return nodes, nil
}
