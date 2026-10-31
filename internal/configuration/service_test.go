package configuration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"github.com/fjzhangZzzzzz/subweft/internal/validator"
	"go.yaml.in/yaml/v3"
)

type checker struct {
	fail    bool
	started chan struct{}
	resume  chan struct{}
}

func (c *checker) Identity() string { return "test-checker" }
func (c *checker) Check(context.Context, []byte) error {
	if c.started != nil {
		close(c.started)
		<-c.resume
	}
	if c.fail {
		return errors.New("test_failure")
	}
	return nil
}
func fixture() Config {
	c := Default()
	c.Subscriptions = []Subscription{{ID: "s1", Name: "主力订阅", URL: "https://subscription.example.com/config", Enabled: true, Pool: "quality", Interval: 3600, Proxy: "DIRECT"}}
	c.Groups = []Group{{ID: "g1", Name: "AI", Type: "select", Pools: []string{"quality"}, Builtins: []string{}}}
	c.RuleSets = []RuleSet{{ID: "set1", Name: "AI", Kind: "local", Entries: []Entry{{ID: "e1", Type: "suffix", Value: "example.com"}, {ID: "e2", Type: "ip", Value: "192.0.2.0/24"}}}}
	c.Rules = []Rule{{ID: "r1", Type: "RULE-SET", Value: "set1", Target: "g1", NoResolve: true}}
	c.Fallback = "DIRECT"
	return c
}
func setup(t *testing.T, c validator.Checker) (*Service, string, string) {
	t.Helper()
	root := t.TempDir()
	dir, key := filepath.Join(root, "state"), filepath.Join(root, "key")
	if e := storage.Initialize(dir, key); e != nil {
		t.Fatal(e)
	}
	store, e := storage.Open(dir, key)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	return New(store, c), dir, key
}
func TestOrderedMixedRulesAndRemoteProviders(t *testing.T) {
	c := fixture()
	b, e := Render(c, false)
	if e != nil {
		t.Fatal(e)
	}
	var m map[string]any
	if e = yaml.Unmarshal(b, &m); e != nil {
		t.Fatal(e)
	}
	rules := m["rules"].([]any)
	if rules[0] != "RULE-SET,AI-domain,AI" || rules[1] != "RULE-SET,AI-ipcidr,AI,no-resolve" || rules[2] != "MATCH,DIRECT" {
		t.Fatal("mixed expansion order or no-resolve scope changed")
	}
	if !strings.Contains(string(b), "type: http") || !strings.Contains(string(b), "主力订阅:") {
		t.Fatal("missing readable remote provider")
	}
	masked, _ := Render(c, true)
	if strings.Contains(string(masked), c.Subscriptions[0].URL) {
		t.Fatal("preview exposed subscription URL")
	}
	c.Subscriptions[0].Proxy = "g1"
	if len(Problems(c)) == 0 {
		t.Fatal("fetch dependency cycle accepted")
	}
	c = fixture()
	c.RuleSets = append(c.RuleSets, RuleSet{ID: "remote", Name: "AI-domain", Kind: "http", URL: "https://rules.example.com/list.yaml", Behavior: "domain", Interval: 3600})
	if len(Problems(c)) == 0 {
		t.Fatal("provider collision accepted")
	}
}
func TestPublishRestoreRestartAndIndependentDevices(t *testing.T) {
	ctx := context.Background()
	check := &checker{}
	s, dir, key := setup(t, check)
	d, _, e := s.Read(ctx)
	if e != nil {
		t.Fatal(e)
	}
	saved, e := s.Save(ctx, Draft{Revision: d.Draft.Revision, Config: fixture()})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Publish(ctx, saved.Revision, ""); e == nil {
		t.Fatal("unchecked publication")
	}
	if _, e = s.Check(ctx, saved.Revision); e != nil {
		t.Fatal(e)
	}
	r, e := s.Publish(ctx, saved.Revision, "first")
	if e != nil {
		t.Fatal(e)
	}
	device, e := s.SaveDevice(ctx, Device{Name: "Phone"}, "save")
	if e != nil {
		t.Fatal(e)
	}
	token, e := s.DeviceToken(ctx, device.ID)
	if e != nil {
		t.Fatal(e)
	}
	old, e := s.Download(ctx, token)
	if e != nil || len(old) == 0 {
		t.Fatal("download failed")
	}
	saved.Config.Subscriptions[0].Enabled = false
	saved, e = s.Save(ctx, saved)
	if e != nil {
		t.Fatal(e)
	}
	result, e := s.Check(ctx, saved.Revision)
	if e != nil || len(result.Problems) == 0 {
		t.Fatal("empty group accepted")
	}
	if _, e = s.Publish(ctx, saved.Revision, ""); e == nil {
		t.Fatal("invalid publication")
	}
	if b, e := s.Download(ctx, token); e != nil || string(b) != string(old) {
		t.Fatal("draft changed active download")
	}
	restored, e := s.Restore(ctx, saved.Revision, r.Number)
	if e != nil {
		t.Fatal(e)
	}
	restored.Config.Subscriptions[0].Name = "Renamed"
	restored, e = s.Save(ctx, restored)
	if e != nil {
		t.Fatal(e)
	}
	check.fail = true
	if _, e = s.Check(ctx, restored.Revision); e == nil {
		t.Fatal("native failure ignored")
	}
	if b, e := s.Download(ctx, token); e != nil || string(b) != string(old) {
		t.Fatal("failed check changed active")
	}
	device, e = s.SaveDevice(ctx, device, "pause")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Download(ctx, token); e == nil {
		t.Fatal("paused download")
	}
	device, e = s.SaveDevice(ctx, device, "resume")
	if e != nil {
		t.Fatal(e)
	}
	device, e = s.SaveDevice(ctx, device, "rotate")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Download(ctx, token); e == nil {
		t.Fatal("old token survived rotation")
	}
	token, e = s.DeviceToken(ctx, device.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Store.Close(); e != nil {
		t.Fatal(e)
	}
	store, e := storage.Open(dir, key)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	reopened := New(store, &checker{})
	if b, e := reopened.Download(ctx, token); e != nil || string(b) != string(old) {
		t.Fatal("restart lost active config or token")
	}
	_, checked, e := reopened.Read(ctx)
	if e != nil || checked != nil {
		t.Fatal("restart reused check")
	}
	if _, e = reopened.SaveDevice(ctx, device, "revoke"); e != nil {
		t.Fatal(e)
	}
	if _, e = reopened.Download(ctx, token); e == nil {
		t.Fatal("revoked token accepted")
	}
}
func TestConcurrentEditInvalidatesInFlightCheck(t *testing.T) {
	ctx := context.Background()
	c := &checker{started: make(chan struct{}), resume: make(chan struct{})}
	s, _, _ := setup(t, c)
	d, _, _ := s.Read(ctx)
	draft, e := s.Save(ctx, Draft{Revision: d.Draft.Revision, Config: fixture()})
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := s.Check(ctx, draft.Revision); done <- e }()
	<-c.started
	draft.Config.Subscriptions[0].Name = "Changed"
	if _, e = s.Save(ctx, draft); e != nil {
		t.Fatal(e)
	}
	close(c.resume)
	if e = <-done; !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale check accepted")
	}
	if _, e = s.Save(ctx, draft); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale edit accepted")
	}
}
func TestNativeDefinitionDoesNotFetchProvider(t *testing.T) {
	path := os.Getenv("SUBWEFT_TEST_MIHOMO")
	if path == "" {
		t.Skip("set SUBWEFT_TEST_MIHOMO to locked native artifact")
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "must not fetch", 500) }))
	defer upstream.Close()
	sha, _ := validator.ExpectedSHA(runtime.GOOS, runtime.GOARCH)
	native, e := validator.New(path, sha, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	c := fixture()
	c.Subscriptions[0].URL = upstream.URL + "/provider"
	data, e := Render(c, false)
	if e != nil {
		t.Fatal(e)
	}
	if e = (validator.Definition{Native: native}).Check(context.Background(), data); e != nil {
		t.Fatal(e)
	}
	c.Rules = append(c.Rules, Rule{ID: "geo", Type: "GEOIP", Value: "CN", Target: "DIRECT"})
	data, _ = Render(c, false)
	if e = (validator.Definition{Native: native}).Check(context.Background(), data); e != nil {
		t.Fatal("partial geographic check rejected", e)
	}
	if geoPath := os.Getenv("SUBWEFT_TEST_GEODB"); geoPath != "" {
		geo, err := os.ReadFile(geoPath)
		if err != nil {
			t.Fatal(err)
		}
		if e = (validator.Definition{Native: native, GeoDB: map[string][]byte{"Country.mmdb": geo}}).Check(context.Background(), data); e != nil {
			t.Fatal(e)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("configuration validation fetched provider")
	}
}

func TestDeviceExpiryAndStaleMutation(t *testing.T) {
	s, _, _ := setup(t, &checker{})
	ctx := context.Background()
	d, _, _ := s.Read(ctx)
	if _, e := s.Check(ctx, d.Draft.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Publish(ctx, d.Draft.Revision, ""); e != nil {
		t.Fatal(e)
	}
	device, e := s.SaveDevice(ctx, Device{Name: "Expiry test"}, "save")
	if e != nil {
		t.Fatal(e)
	}
	token, _ := s.DeviceToken(ctx, device.ID)
	stale := device
	expired := time.Now().Add(-time.Minute)
	device.ExpiresAt = &expired
	if _, e = s.SaveDevice(ctx, device, "save"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Download(ctx, token); e == nil {
		t.Fatal("expired device downloaded")
	}
	if _, e = s.DeviceToken(ctx, device.ID); e == nil {
		t.Fatal("expired link disclosed")
	}
	if _, e = s.SaveDevice(ctx, stale, "resume"); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale device edit accepted")
	}
}

func TestPartialReportPublishPreservesOriginalAndDevice(t *testing.T) {
	path := os.Getenv("SUBWEFT_TEST_MIHOMO")
	if path == "" {
		t.Skip("locked native artifact required")
	}
	sha, _ := validator.ExpectedSHA(runtime.GOOS, runtime.GOARCH)
	native, err := validator.New(path, sha, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, _, _ := setup(t, validator.Definition{Native: native})
	ctx := context.Background()
	d, _, _ := s.Read(ctx)
	device, err := s.SaveDevice(ctx, Device{Name: "Test device"}, "save")
	if err != nil {
		t.Fatal(err)
	}
	token, _ := s.DeviceToken(ctx, device.ID)
	if _, err := s.Download(ctx, token); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("unpublished download exposed state")
	}
	c := fixture()
	c.Rules = append(c.Rules, Rule{ID: "geo", Type: "GEOIP", Value: "CN", Target: "DIRECT", NoResolve: true})
	saved, err := s.Save(ctx, Draft{Revision: d.Draft.Revision, Config: c})
	if err != nil {
		t.Fatal(err)
	}
	checked, err := s.Check(ctx, saved.Revision)
	if err != nil || !checked.Publishable || len(checked.Validation.Items) == 0 {
		t.Fatal("partial check unavailable", err)
	}
	release, err := s.Publish(ctx, saved.Revision, "")
	if err != nil || release.Validation == nil {
		t.Fatal("report not published", err)
	}
	data, err := s.Download(ctx, token)
	if err != nil || !strings.Contains(string(data), "GEOIP,CN,DIRECT,no-resolve") || strings.Contains(string(data), "validation-only") {
		t.Fatal("published projection instead of original")
	}
	doc, _, _ := s.Read(ctx)
	if doc.Releases[0].Validation == nil {
		t.Fatal("report not persisted")
	}
}
