package validator_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/validator"
)

func TestRemoteProjectionUsesPinnedCoreWithoutFetching(t *testing.T) {
	name := "mihomo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary, _ := filepath.Abs(filepath.Join("..", "..", "output", "tools", name))
	if _, e := os.Stat(binary); e != nil {
		t.Skip("fetch pinned validator")
	}
	sha, _ := validator.ExpectedSHA(runtime.GOOS, runtime.GOARCH)
	checker, e := validator.New(binary, sha, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "must not fetch", 503) }))
	defer upstream.Close()
	p := domain.Profile{Name: "offline validation", Groups: []domain.Group{{Name: "Proxy", Type: "select", SourceIDs: []string{"s"}}}, Rules: []domain.Rule{{Type: "RULE-SET", Value: "r", Target: "Proxy", Enabled: true}, {Type: "GEOSITE", Value: "cn", Target: "DIRECT", Enabled: true}, {Type: "MATCH", Target: "Proxy", Enabled: true}}, GeoSiteID: "geo"}
	snap := domain.Snapshot{Sources: []domain.Source{{Meta: domain.Meta{ID: "s"}, Kind: "http", Name: "remote", URL: upstream.URL + "/sub", Enabled: true, Interval: 3600}}, Resources: []domain.Resource{{Meta: domain.Meta{ID: "r"}, Name: "rules", Kind: "http", Type: "ruleset", Behavior: "domain", Format: "yaml", URL: upstream.URL + "/rules", Enabled: true, Interval: 86400}, {Meta: domain.Meta{ID: "geo"}, Name: "geo", Kind: "http", Type: "geosite", URL: upstream.URL + "/geo", Enabled: true, Interval: 86400}}}
	result, e := compile.Compile(p, snap, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = checker.Check(context.Background(), result.Config); e != nil {
		t.Fatal(e)
	}
	p.Settings.DNSEnabled = true
	p.Settings.DNSMode = "redir-host"
	p.Settings.Nameservers = []string{"1.1.1.1", "bogus://geosite.example.invalid"}
	bad, e := compile.Compile(p, snap, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = checker.Check(context.Background(), bad.Config); e == nil {
		t.Fatal("projection masked invalid ordinary DNS server")
	}
	if calls.Load() != 0 {
		t.Fatal("validator contacted remote")
	}
}
