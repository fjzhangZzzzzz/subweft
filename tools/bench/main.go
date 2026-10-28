// Benchmark the actual authorized download handler with a fixed synthetic dataset.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/app"
	"github.com/fjzhangZzzzzz/subweft/internal/auth"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/server"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
	"github.com/fjzhangZzzzzz/subweft/internal/validator"
	"go.yaml.in/yaml/v3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "benchmark_failed")
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("validator", "", "absolute path to pinned validator")
	requests := flag.Int("requests", 200, "measured requests")
	settle := flag.Duration("settle", 2*time.Second, "settle before measuring")
	flag.Parse()
	if *requests < 20 || *requests > 10000 {
		return fmt.Errorf("invalid_request_count")
	}
	root, err := os.MkdirTemp("", "subweft-benchmark-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	data := filepath.Join(root, "state")
	key := filepath.Join(root, "key")
	if err = storage.Initialize(data, key); err != nil {
		return err
	}
	store, err := storage.Open(data, key)
	if err != nil {
		return err
	}
	defer store.Close()
	sha, ok := validator.ExpectedSHA(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return fmt.Errorf("unsupported_platform")
	}
	checker, err := validator.New(*path, sha, filepath.Join(root, "validation"))
	if err != nil {
		return err
	}
	svc := app.New(store, source.NewFetcher(nil), checker)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		nodes := make([]domain.Node, 200)
		for j := range nodes {
			nodes[j] = domain.Node{Name: fmt.Sprintf("Node-%04d", j), Type: "ss", Server: "192.0.2.10", Port: 443, Cipher: "aes-128-gcm", Password: "fixture-benchmark-password", UDP: true}
		}
		content, err := yaml.Marshal(map[string]any{"proxies": nodes})
		if err != nil {
			return err
		}
		_, err = svc.SaveSource(ctx, app.SourceInput{Name: fmt.Sprintf("source-%02d", i), Kind: "local", Content: string(content), Enabled: true, Interval: 3600})
		if err != nil {
			return err
		}
	}
	tokens := []string{}
	for i := 0; i < 5; i++ {
		p, err := svc.SaveProfile(ctx, domain.Profile{Name: fmt.Sprintf("profile-%02d", i), Groups: []domain.Group{{Name: "Proxy", Type: "select", AllSources: true}}, Rules: []domain.Rule{{Type: "DOMAIN-SUFFIX", Value: "example.invalid", Target: "Proxy", Enabled: true}, {Type: "MATCH", Target: "Proxy", Enabled: true}}, Settings: domain.Settings{MixedPort: 7890}})
		if err != nil {
			return err
		}
		if _, err = svc.Publish(ctx, p.ID, p.Revision, false); err != nil {
			return err
		}
		for j := 0; j < 4; j++ {
			d, err := svc.SaveDevice(ctx, domain.Device{Name: fmt.Sprintf("device-%02d-%d", i, j), ProfileID: p.ID, State: "active"}, "fixture")
			if err != nil {
				return err
			}
			tokens = append(tokens, d.Token)
		}
	}
	web := httptest.NewUnstartedServer(nil)
	base := "http://" + web.Listener.Addr().String()
	idp := httptest.NewUnstartedServer(nil)
	issuerURL := "http://" + idp.Listener.Addr().String()
	fixture, err := testfixture.New(issuerURL, base+"/auth/callback", testfixture.Subject)
	if err != nil {
		return err
	}
	idp.Config.Handler = fixture.Handler()
	idp.Start()
	defer idp.Close()
	authn, err := auth.New(ctx, auth.Config{Issuer: issuerURL, BaseURL: base, ClientID: testfixture.ClientID, ClientSecret: testfixture.ClientSecret, AdminSubjects: []string{testfixture.Subject}, LocalHTTP: true}, store)
	if err != nil {
		return err
	}
	web.Config.Handler = (&server.API{Service: svc, Auth: authn, BaseURL: base, UI: http.NotFoundHandler()}).Handler()
	web.Start()
	defer web.Close()
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{MaxIdleConns: 20, MaxIdleConnsPerHost: 20}}
	defer client.CloseIdleConnections()
	for _, token := range tokens {
		r, err := client.Get(base + "/s/" + token + "/config.yaml")
		if err != nil {
			return err
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
		if r.StatusCode != 200 {
			return fmt.Errorf("warmup_failed")
		}
	}
	runtime.GC()
	time.Sleep(*settle)
	var mu sync.Mutex
	latencies := make([]float64, 0, *requests)
	failures := 0
	totalBytes := int64(0)
	jobs := make(chan int)
	var workers sync.WaitGroup
	start := time.Now()
	for i := 0; i < 10; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				t := time.Now()
				r, err := client.Get(base + "/s/" + tokens[index%len(tokens)] + "/config.yaml")
				size := int64(0)
				success := false
				if err == nil {
					size, err = io.Copy(io.Discard, r.Body)
					r.Body.Close()
					success = err == nil && r.StatusCode == 200
				}
				elapsed := float64(time.Since(t).Microseconds()) / 1000
				mu.Lock()
				latencies = append(latencies, elapsed)
				totalBytes += size
				if !success {
					failures++
				}
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < *requests; i++ {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	duration := time.Since(start)
	sort.Float64s(latencies)
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"sources": 10, "nodes": 2000, "profiles": 5, "devices": 20, "requests": *requests, "concurrency": 10, "failures": failures, "p50_ms": latencies[len(latencies)/2], "p95_ms": latencies[(len(latencies)-1)*95/100], "max_ms": latencies[len(latencies)-1], "duration_ms": duration.Milliseconds(), "mean_response_bytes": totalBytes / int64(*requests), "go_heap_alloc_bytes": stats.HeapAlloc, "native_validation": checker.Identity(), "scope": "loopback cached self-contained config; no large public ruleset; Go heap is not process RSS"})
}
