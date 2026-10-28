package source

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

type Options = domain.FetchOptions

func ValidateOptions(o Options) error {
	if o.Timeout != 0 && (o.Timeout < 3 || o.Timeout > 120) || o.Retries < 0 || o.Retries > 3 {
		return domain.Invalid("invalid_fetch_options", "fetch")
	}
	if o.Proxy != "" {
		u, e := url.Parse(o.Proxy)
		if e != nil || u.Hostname() == "" || u.Scheme != "http" && u.Scheme != "https" || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
			return domain.Invalid("invalid_proxy_url", "fetch.proxy")
		}
	}
	if len(o.UserAgent) > 512 || strings.ContainsAny(o.UserAgent, "\r\n\x00") {
		return domain.Invalid("invalid_header", "fetch.userAgent")
	}
	if len(o.Headers) > 32 {
		return domain.Invalid("too_many_headers", "fetch.headers")
	}
	for key, value := range o.Headers {
		if key == "" || len(key) > 128 || len(value) > 4096 || strings.ContainsAny(key, "\r\n\x00 :") || strings.ContainsAny(value, "\r\n\x00") || strings.EqualFold(key, "Host") || strings.EqualFold(key, "Connection") || strings.EqualFold(key, "Proxy-Authorization") {
			return domain.Invalid("invalid_header", "fetch.headers")
		}
		r := http.Request{Header: http.Header{}}
		r.Header.Set(key, value)
	}
	return nil
}
func (f *Fetcher) FetchWith(ctx context.Context, raw string, options Options, limit int) ([]byte, error) {
	if e := ValidateOptions(options); e != nil {
		return nil, e
	}
	transport := f.Client.Transport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	if options.Proxy != "" {
		proxy, _ := url.Parse(options.Proxy)
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			target, e := f.Resolve(ctx, address)
			if e != nil {
				return nil, e
			}
			proxyAddress := proxy.Host
			if proxy.Port() == "" {
				port := "80"
				if proxy.Scheme == "https" {
					port = "443"
				}
				proxyAddress = net.JoinHostPort(proxy.Hostname(), port)
			}
			destination, e := f.Resolve(ctx, proxyAddress)
			if e != nil {
				return nil, e
			}
			conn, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", destination)
			if e != nil {
				return nil, e
			}
			ok := false
			defer func() {
				if !ok {
					conn.Close()
				}
			}()
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
			if proxy.Scheme == "https" {
				secured := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: proxy.Hostname()})
				if e = secured.HandshakeContext(ctx); e != nil {
					return nil, e
				}
				conn = secured
			}
			auth := ""
			if proxy.User != nil {
				password, _ := proxy.User.Password()
				auth = "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(proxy.User.Username()+":"+password)) + "\r\n"
			}
			if _, e = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n%s\r\n", target, target, auth); e != nil {
				return nil, e
			}
			reader := bufio.NewReaderSize(conn, 16384)
			total := 0
			status := ""
			for {
				line, e := reader.ReadString('\n')
				total += len(line)
				if e != nil || total > 16384 {
					return nil, errors.New("proxy_failed")
				}
				if status == "" {
					status = line
				}
				if line == "\r\n" {
					break
				}
			}
			if !strings.HasPrefix(status, "HTTP/1.1 200 ") && !strings.HasPrefix(status, "HTTP/1.0 200 ") {
				return nil, errors.New("proxy_failed")
			}
			if reader.Buffered() != 0 {
				return nil, errors.New("proxy_failed")
			}
			_ = conn.SetDeadline(time.Time{})
			ok = true
			return conn, nil
		}
	}
	client := *f.Client
	client.Transport = transport
	if options.Timeout != 0 {
		client.Timeout = time.Duration(options.Timeout) * time.Second
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if e := f.Client.CheckRedirect(req, via); e != nil {
			return e
		}
		if len(via) > 0 && req.URL.Host != via[0].URL.Host {
			for k := range options.Headers {
				req.Header.Del(k)
			}
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")
		}
		return nil
	}
	copy := *f
	copy.Client = &client
	copy.Headers = options.Headers
	copy.UserAgent = options.UserAgent
	var b []byte
	var e error
	for attempt := 0; attempt <= options.Retries; attempt++ {
		b, e = copy.FetchLimit(ctx, raw, limit)
		if e == nil || ctx.Err() != nil {
			return b, e
		}
		if attempt < options.Retries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 200 * time.Millisecond):
			}
		}
	}
	return nil, e
}
