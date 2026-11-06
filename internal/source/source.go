package source

import (
	"bytes"
	"context"
	"crypto/tls"
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
type Fetcher struct {
	Client    *http.Client
	Resolve   func(context.Context, string) (string, error)
	Headers   map[string]string
	UserAgent string
}

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
	resolve := func(ctx context.Context, address string) (string, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return "", errors.New("blocked_destination")
		}
		ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if e != nil || len(ips) == 0 {
			return "", errors.New("dns_failed")
		}
		for _, ip := range ips {
			if !allowedIP(ip) {
				return "", errors.New("blocked_destination")
			}
		}
		return net.JoinHostPort(ips[0].String(), port), nil
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		resolved, e := resolve(ctx, address)
		if e != nil {
			return nil, e
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, resolved)
	}
	return &Fetcher{Resolve: resolve, Client: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
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
	return f.FetchLimit(ctx, raw, MaxBytes)
}
func (f *Fetcher) FetchLimit(ctx context.Context, raw string, limit int) ([]byte, error) {
	if e := ValidateURL(raw); e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if e != nil {
		return nil, domain.Invalid("invalid_url", "url")
	}
	req.Header.Set("User-Agent", "Subweft/0.1-dev")
	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	}
	for key, value := range f.Headers {
		req.Header.Set(key, value)
	}
	resp, e := f.Client.Do(req)
	if e != nil {
		return nil, domain.Invalid("fetch_failed", "url")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, domain.Invalid("upstream_status", "url")
	}
	if resp.ContentLength > int64(limit) {
		return nil, domain.Invalid("input_too_large", "url")
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if e != nil {
		return nil, domain.Invalid("fetch_failed", "url")
	}
	if len(b) > limit {
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
	// Decode one bounded Base64 envelope; inner fields still receive full validation.
	if !bytes.ContainsAny(b, ":") {
		if decoded, e := decode64(strings.Join(strings.Fields(string(b)), "")); e == nil && len(decoded) > 0 && len(decoded) <= MaxBytes {
			b = bytes.TrimSpace(decoded)
		}
	}
	var nodes []domain.Node
	if first, _, _ := strings.Cut(string(b), "\n"); strings.Contains(first, "://") {
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
			if e := validateNodeYAML(node); e != nil {
				return nil, domain.Field("nodes."+strconv.Itoa(i), e)
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
