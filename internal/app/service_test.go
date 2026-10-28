package app_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/app"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
)

type checker struct {
	reject  atomic.Bool
	entered chan struct{}
	resume  chan struct{}
}

func (c *checker) Identity() string { return "test-validator" }
func (c *checker) Check(ctx context.Context, b []byte) error {
	if c.entered != nil {
		close(c.entered)
		<-c.resume
	}
	if c.reject.Load() {
		return domain.Invalid("native_validation_failed", "publish")
	}
	return nil
}
func setup(t *testing.T) (*app.Service, *checker, string, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "state")
	key := filepath.Join(root, "key")
	if e := storage.Initialize(dir, key); e != nil {
		t.Fatal(e)
	}
	s, e := storage.Open(dir, key)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	c := &checker{}
	return app.New(s, source.NewFetcher([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}), c), c, dir, key
}
func create(t *testing.T, s *app.Service) (domain.Source, domain.Profile) {
	t.Helper()
	ctx := context.Background()
	src, e := s.SaveSource(ctx, app.SourceInput{Name: "fixture", Kind: "local", Content: testfixture.ProviderYAML, Interval: 3600, Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.SaveProfile(ctx, domain.Profile{Name: "daily", Groups: []domain.Group{{Name: "Proxy", Type: "select", SourceIDs: []string{src.ID}}}, Rules: []domain.Rule{{Type: "DOMAIN-SUFFIX", Value: "example.invalid", Target: "Proxy", Enabled: true}, {Type: "MATCH", Target: "Proxy", Enabled: true}}, Settings: domain.Settings{MixedPort: 7890}})
	if e != nil {
		t.Fatal(e)
	}
	return src, p
}
func TestPublishRejectPreservesActiveAndIndependentDeviceRevocation(t *testing.T) {
	ctx := context.Background()
	s, c, _, _ := setup(t)
	_, p := create(t, s)
	first, e := s.Publish(ctx, p.ID, p.Revision, false)
	if e != nil {
		t.Fatal(e)
	}
	laptop, e := s.SaveDevice(ctx, domain.Device{Name: "laptop", ProfileID: p.ID, State: "active"}, "fixture")
	if e != nil {
		t.Fatal(e)
	}
	phone, e := s.SaveDevice(ctx, domain.Device{Name: "phone", ProfileID: p.ID, State: "active"}, "fixture")
	if e != nil {
		t.Fatal(e)
	}
	c.reject.Store(true)
	p.Rules[0].Value = "changed.invalid"
	p, e = s.SaveProfile(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Publish(ctx, p.ID, p.Revision, false); e == nil {
		t.Fatal("bad validation published")
	}
	_, got, e := s.Store.Download(ctx, laptop.Token)
	if e != nil || !bytes.Equal(got.Config, first.Config) {
		t.Fatal("active release not preserved")
	}
	oldToken := phone.Token
	phone, e = s.Rotate(ctx, phone.ID, phone.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Store.Download(ctx, oldToken); e == nil {
		t.Fatal("old token still valid")
	}
	if _, _, e = s.Store.Download(ctx, phone.Token); e != nil {
		t.Fatal(e)
	}
	laptop.State = "revoked"
	laptop, e = s.SaveDevice(ctx, laptop, "fixture")
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Store.Download(ctx, laptop.Token); e == nil {
		t.Fatal("revoked device accepted")
	}
	if _, _, e = s.Store.Download(ctx, phone.Token); e != nil {
		t.Fatal("other device affected")
	}
	laptop.State = "active"
	if _, e = s.SaveDevice(ctx, laptop, "fixture"); e == nil {
		t.Fatal("revoked device restored")
	}
}
func TestRefreshFailurePreservesSnapshotAndRequiresAcknowledgment(t *testing.T) {
	ctx := context.Background()
	s, _, _, _ := setup(t)
	var bad atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bad.Load() {
			http.Error(w, "unavailable", 503)
			return
		}
		_, _ = w.Write([]byte(testfixture.ProviderYAML))
	}))
	defer upstream.Close()
	src, e := s.SaveSource(ctx, app.SourceInput{Name: "remote", Kind: "http", URL: upstream.URL, Enabled: true, Interval: 60})
	if e != nil {
		t.Fatal(e)
	}
	src, e = s.Refresh(ctx, src.ID)
	if e != nil {
		t.Fatal(e)
	}
	bad.Store(true)
	if _, e = s.Refresh(ctx, src.ID); e == nil {
		t.Fatal("expected fetch failure")
	}
	current, e := storage.Load[domain.Source](ctx, s.Store, "source", src.ID)
	if e != nil || len(current.Nodes) != 2 || current.LastError == "" {
		t.Fatal("valid cache lost")
	}
	p, e := s.SaveProfile(ctx, domain.Profile{Name: "cached", Groups: []domain.Group{{Name: "Proxy", Type: "select", AllSources: true}}, Rules: []domain.Rule{{Type: "MATCH", Target: "Proxy", Enabled: true}}, Settings: domain.Settings{MixedPort: 7890}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Publish(ctx, p.ID, p.Revision, false); e == nil {
		t.Fatal("stale cache not acknowledged")
	}
	if _, e = s.Publish(ctx, p.ID, p.Revision, true); e != nil {
		t.Fatal(e)
	}
}
func TestConcurrentEditCannotPublishStaleCandidate(t *testing.T) {
	ctx := context.Background()
	s, c, _, _ := setup(t)
	_, p := create(t, s)
	c.entered = make(chan struct{})
	c.resume = make(chan struct{})
	finished := make(chan error, 1)
	go func() { _, e := s.Publish(ctx, p.ID, p.Revision, false); finished <- e }()
	<-c.entered
	p.Name = "changed"
	if _, e := s.SaveProfile(ctx, p); e != nil {
		t.Fatal(e)
	}
	close(c.resume)
	if e := <-finished; !errors.Is(e, domain.ErrConflict) {
		t.Fatal("concurrent modification accepted")
	}
	snap, e := s.Store.Snapshot(ctx)
	if e != nil || snap.Active[p.ID] != "" {
		t.Fatal("stale publication became active")
	}
}

func TestManualCredentialEditsRemainScopedToProtocol(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := setup(t)
	original, e := svc.SaveNode(ctx, domain.ManualNode{Node: domain.Node{Name: "manual", Type: "vmess", Server: "192.0.2.1", Port: 443, UUID: "00000000-0000-4000-8000-000000000001", Cipher: "auto"}, Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	original.Node.UUID = ""
	updated, e := svc.SaveNode(ctx, original)
	if e != nil || updated.Node.UUID == "" {
		t.Fatal("same-protocol UUID was not retained")
	}
	updated.Node = domain.Node{Name: "manual", Type: "trojan", Server: "192.0.2.1", Port: 443}
	if _, e = svc.SaveNode(ctx, updated); e == nil {
		t.Fatal("credentials carried into another protocol")
	}
	account, e := svc.SaveNode(ctx, domain.ManualNode{Node: domain.Node{Name: "account", Type: "http", Server: "192.0.2.2", Port: 443, Username: "fixture-user", Password: "fixture-password"}, Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	account.Node.Password = ""
	account, e = svc.SaveNode(ctx, account)
	if e != nil || account.Node.Password != "fixture-password" {
		t.Fatal("password was not retained")
	}
	account.Node.Username = "changed-user"
	account.Node.Password = ""
	if _, e = svc.SaveNode(ctx, account); e == nil {
		t.Fatal("old password silently assigned to a changed account")
	}
	account.Node.Username = ""
	account.Node.Password = ""
	account, e = svc.SaveNode(ctx, account)
	if e != nil || account.Node.Password != "" {
		t.Fatal("optional authentication could not be cleared")
	}
}
