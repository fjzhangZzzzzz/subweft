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
	"sync/atomic"
	"testing"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
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
	var directCalls, proxyCalls, ruleCalls, wrongUA atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != compile.DeviceUserAgent {
			wrongUA.Add(1)
			http.Error(w, "wrong client format", 400)
			return
		}
		if failed.Load() {
			http.Error(w, "unavailable", 503)
			return
		}
		switch r.URL.Path {
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
	defer origin.Close()
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
	defer proxy.Close()
	host, portText, _ := net.SplitHostPort(strings.TrimPrefix(proxy.URL, "http://"))
	var port int
	fmt.Sscanf(portText, "%d", &port)
	p := domain.Profile{Meta: domain.Meta{ID: "device-profile"}, Name: "device", Groups: []domain.Group{{Name: "Proxy", Type: "select", SourceIDs: []string{"direct", "proxied"}}}, SourceUpdates: map[string]string{"proxied": "Bootstrap"}, RuleUpdates: map[string]string{"rules": "Bootstrap"}, Rules: []domain.Rule{{Type: "RULE-SET", Value: "rules", Target: "DIRECT", Enabled: true}, {Type: "MATCH", Target: "DIRECT", Enabled: true}}}
	snap := domain.Snapshot{Sources: []domain.Source{{Meta: domain.Meta{ID: "direct"}, Name: "direct", Kind: "http", URL: origin.URL + "/direct", Enabled: true, Interval: 3600}, {Meta: domain.Meta{ID: "proxied"}, Name: "proxied", Kind: "http", URL: origin.URL + "/proxied", Enabled: true, Interval: 3600}}, Nodes: []domain.ManualNode{{Meta: domain.Meta{ID: "bootstrap"}, Enabled: true, Node: domain.Node{Name: "Bootstrap", Type: "http", Server: host, Port: port}}}, Resources: []domain.Resource{{Meta: domain.Meta{ID: "rules"}, Name: "rules", Kind: "http", Type: "ruleset", URL: origin.URL + "/rules", Behavior: "domain", Format: "yaml", Interval: 86400, Enabled: true}}}
	dir := t.TempDir()
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	start := func() (string, func()) {
		t.Helper()
		result, e := compile.Compile(p, snap, false)
		if e != nil {
			t.Fatal(e)
		}
		var config map[string]any
		_ = yaml.Unmarshal(result.Config, &config)
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
		data, _ := yaml.Marshal(config)
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
		stop := func() { cancel(); _ = cmd.Wait() }
		for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
			req, _ := http.NewRequest("GET", "http://"+control+"/version", nil)
			req.Header.Set("Authorization", "Bearer fixture-controller")
			resp, e := client.Do(req)
			if e == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					return control, stop
				}
			}
			time.Sleep(30 * time.Millisecond)
		}
		stop()
		t.Fatal("device core did not become ready")
		return "", func() {}
	}
	read := func(control, method, path string) map[string]any {
		t.Helper()
		req, _ := http.NewRequest(method, "http://"+control+path, nil)
		req.Header.Set("Authorization", "Bearer fixture-controller")
		resp, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		var v map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&v)
		return v
	}
	waitNode := func(control, want string) {
		t.Helper()
		for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
			v := read(control, "GET", "/providers/proxies/direct")
			b, _ := json.Marshal(v)
			if strings.Contains(string(b), want) {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatal("device provider nodes not updated")
	}
	control, stop := start()
	waitNode(control, "upstream-a")
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline) && (ruleCalls.Load() == 0 || proxyCalls.Load() < 2); {
		time.Sleep(30 * time.Millisecond)
	}
	if directCalls.Load() < 2 || ruleCalls.Load() == 0 || proxyCalls.Load() < 2 || wrongUA.Load() != 0 {
		stop()
		t.Fatal("device direct/proxy/header acquisition failed")
	}
	failed.Store(true)
	read(control, "PUT", "/providers/proxies/direct")
	waitNode(control, "upstream-a")
	stop()
	failed.Store(false)
	snap.Sources[0].URL = origin.URL + "/changed"
	control, stop = start()
	waitNode(control, "upstream-b")
	stop()
	// Roll back the definition in the same device work directory.
	snap.Sources[0].URL = origin.URL + "/direct"
	control, stop = start()
	defer stop()
	waitNode(control, "upstream-a")
}
