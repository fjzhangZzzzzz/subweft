package validator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/configuration"
	"github.com/fjzhangZzzzzz/subweft/internal/validator"
	"go.yaml.in/yaml/v3"
)

// Runs the locked core as a disposable device, bound only to loopback. Synthetic
// origins and the CONNECT proxy never forward outside this test's loopback server.
func TestDeviceCoreDownloadsThroughDirectAndIndependentProxy(t *testing.T) {
	name := "mihomo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary, _ := filepath.Abs(filepath.Join("..", "..", "output", "tools", name))
	if _, e := os.Stat(binary); e != nil {
		t.Skip("fetch pinned validator")
	}
	sha, _ := validator.ExpectedSHA(runtime.GOOS, runtime.GOARCH)
	if _, e := validator.New(binary, sha, t.TempDir()); e != nil {
		t.Fatal(e)
	}
	var failed atomic.Bool
	var directCalls, proxyCalls, ruleCalls atomic.Int32
	var proxyHost string
	var proxyPort int
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failed.Load() {
			http.Error(w, "unavailable", 503)
			return
		}
		switch r.URL.Path {
		case "/bootstrap":
			fmt.Fprintf(w, "proxies:\n - {name: fixture-proxy, type: http, server: %s, port: %d}\n", proxyHost, proxyPort)
		case "/rules":
			ruleCalls.Add(1)
			fmt.Fprint(w, "payload:\n - example.invalid\n")
		default:
			directCalls.Add(1)
			suffix := "a"
			if r.URL.Path == "/changed" {
				suffix = "b"
			}
			// A full subscription configuration, not just a provider payload.
			fmt.Fprintf(w, "mixed-port: 7890\ndns:\n  enable: false\nproxies:\n - {name: upstream-%s, type: socks5, server: 192.0.2.1, port: 1080}\nproxy-groups:\n - {name: ignored-upstream-group, type: select, proxies: [upstream-%s]}\nrules:\n - MATCH,ignored-upstream-group\n", suffix, suffix)
		}
	}))
	t.Cleanup(origin.Close)
	address := strings.TrimPrefix(origin.URL, "http://")
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Host != address {
			http.Error(w, "test destination only", 403)
			return
		}
		upstream, e := net.DialTimeout("tcp", address, time.Second)
		if e != nil {
			http.Error(w, "unavailable", 502)
			return
		}
		client, _, e := w.(http.Hijacker).Hijack()
		if e != nil {
			upstream.Close()
			return
		}
		proxyCalls.Add(1)
		_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		go func() {
			defer upstream.Close()
			defer client.Close()
			go io.Copy(upstream, client)
			_, _ = io.Copy(client, upstream)
		}()
	}))
	t.Cleanup(proxy.Close)
	proxyHost, portText, _ := net.SplitHostPort(strings.TrimPrefix(proxy.URL, "http://"))
	fmt.Sscanf(portText, "%d", &proxyPort)
	c := configuration.Config{
		Base: map[string]any{"mode": "rule", "dns": map[string]any{"enable": false}}, Fallback: "DIRECT",
		Subscriptions: []configuration.Subscription{
			{ID: "direct", Name: "direct", URL: origin.URL + "/direct", Pool: "quality", Enabled: true, Interval: 3600, Proxy: "DIRECT"},
			{ID: "proxied", Name: "proxied", URL: origin.URL + "/proxied", Pool: "quality", Enabled: true, Interval: 3600, Proxy: "bootstrap-group"},
			{ID: "bootstrap", Name: "bootstrap", URL: origin.URL + "/bootstrap", Pool: "quota", Enabled: true, Interval: 3600, Proxy: "DIRECT"},
		},
		Groups:   []configuration.Group{{ID: "bootstrap-group", Name: "Bootstrap", Type: "select", Pools: []string{"quota"}}, {ID: "traffic", Name: "Traffic", Type: "select", Pools: []string{"quality"}}},
		RuleSets: []configuration.RuleSet{{ID: "rules", Name: "rules", Kind: "http", URL: origin.URL + "/rules", Behavior: "domain", Interval: 86400}},
		Rules:    []configuration.Rule{{ID: "rule", Type: "RULE-SET", Value: "rules", Target: "DIRECT"}},
	}
	dir := t.TempDir()
	transport := &http.Transport{Proxy: nil}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Timeout: 10 * time.Second, Transport: transport}
	start := func() (string, func()) {
		t.Helper()
		result, e := configuration.Render(c, false)
		if e != nil {
			t.Fatal(e)
		}
		var config map[string]any
		if e := yaml.Unmarshal(result, &config); e != nil {
			t.Fatal(e)
		}
		listener, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		control := listener.Addr().String()
		listener.Close()
		config["external-controller"] = control
		config["secret"] = "fixture-controller"
		config["bind-address"] = "127.0.0.1"
		config["log-level"] = "silent"
		data, e := yaml.Marshal(config)
		if e != nil {
			t.Fatal(e)
		}
		file := filepath.Join(dir, "config.yaml")
		if e = os.WriteFile(file, data, 0600); e != nil {
			t.Fatal(e)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, binary, "-d", dir, "-f", file)
		cmd.Dir = dir
		cmd.Env = []string{"HOME=" + dir, "USERPROFILE=" + dir, "APPDATA=" + dir, "LOCALAPPDATA=" + dir, "SystemRoot=" + os.Getenv("SystemRoot")}
		if e = cmd.Start(); e != nil {
			cancel()
			t.Fatal(e)
		}
		stop := sync.OnceFunc(func() { cancel(); _ = cmd.Wait() })
		t.Cleanup(stop)
		for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
			req, _ := http.NewRequest("GET", "http://"+control+"/providers/proxies", nil)
			req.Header.Set("Authorization", "Bearer fixture-controller")
			resp, e := client.Do(req)
			if e == nil {
				var state struct {
					Providers map[string]json.RawMessage `json:"providers"`
				}
				decodeErr := json.NewDecoder(resp.Body).Decode(&state)
				resp.Body.Close()
				if resp.StatusCode == 200 && decodeErr == nil && state.Providers["direct"] != nil && state.Providers["proxied"] != nil && state.Providers["bootstrap"] != nil {
					return control, stop
				}
			}
			time.Sleep(30 * time.Millisecond)
		}
		stop()
		t.Fatal("device core did not become ready")
		return "", func() {}
	}
	read := func(control, method, path string, failure ...bool) map[string]any {
		t.Helper()
		req, _ := http.NewRequest(method, "http://"+control+path, nil)
		req.Header.Set("Authorization", "Bearer fixture-controller")
		resp, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		if len(failure) > 0 && failure[0] {
			if resp.StatusCode < 400 {
				t.Fatal("failed upstream update was accepted")
			}
			return nil
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			t.Fatalf("controller %s %s returned %d", method, path, resp.StatusCode)
		}
		if resp.StatusCode == http.StatusNoContent {
			return nil
		}
		var v map[string]any
		if e := json.NewDecoder(resp.Body).Decode(&v); e != nil {
			t.Fatal("invalid controller response", e)
		}
		return v
	}
	waitNode := func(control, want, provider string) {
		t.Helper()
		for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
			v := read(control, "GET", "/providers/proxies/"+provider)
			b, _ := json.Marshal(v)
			if strings.Contains(string(b), want) {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatal("device provider nodes not updated")
	}
	control, stop := start()
	waitNode(control, "upstream-a", "direct")
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline) && ruleCalls.Load() == 0; {
		time.Sleep(30 * time.Millisecond)
	}
	if directCalls.Load() < 1 || ruleCalls.Load() == 0 {
		stop()
		t.Fatal("device direct/rules acquisition failed")
	}
	// Wait for the independent bootstrap subscription before testing its dependent route.
	waitNode(control, "fixture-proxy", "bootstrap")
	// Explicit group routing must use the proxy; DIRECT must bypass it.
	before := proxyCalls.Load()
	read(control, "PUT", "/providers/proxies/proxied")
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline) && proxyCalls.Load() == before; {
		time.Sleep(30 * time.Millisecond)
	}
	if proxyCalls.Load() <= before {
		stop()
		t.Fatal("explicit group route did not use independent proxy")
	}
	before = proxyCalls.Load()
	hits := directCalls.Load()
	read(control, "PUT", "/providers/proxies/direct")
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline) && directCalls.Load() == hits; {
		time.Sleep(30 * time.Millisecond)
	}
	if directCalls.Load() == hits || proxyCalls.Load() != before {
		stop()
		t.Fatal("explicit direct did not bypass proxy")
	}
	failed.Store(true)
	read(control, "PUT", "/providers/proxies/direct", true)
	waitNode(control, "upstream-a", "direct")
	stop()
	failed.Store(false)
	c.Subscriptions[0].URL = origin.URL + "/changed"
	control, stop = start()
	// Stable provider paths intentionally retain cache across configuration changes.
	// A device refresh must acquire the current URL, including after rollback.
	read(control, "PUT", "/providers/proxies/direct")
	waitNode(control, "upstream-b", "direct")
	stop()
	// Roll back the definition in the same device work directory.
	c.Subscriptions[0].URL = origin.URL + "/direct"
	control, stop = start()
	defer stop()
	read(control, "PUT", "/providers/proxies/direct")
	waitNode(control, "upstream-a", "direct")
}
