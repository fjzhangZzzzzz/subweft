package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

func TestDeviceMigrationPreservesInputsAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := filepath.Join(root, "state")
	key := filepath.Join(root, "key")
	if e := Initialize(dir, key); e != nil {
		t.Fatal(e)
	}
	s, e := Open(dir, key)
	if e != nil {
		t.Fatal(e)
	}
	src := domain.Source{Name: "legacy", Kind: "http", Enabled: false, Interval: 60, Fetch: domain.FetchOptions{Proxy: "http://example.invalid:8080", Headers: map[string]string{"Authorization": "fixture-secret"}}, ClientProvider: &domain.ClientProvider{Interval: 7200, Proxy: "Independent", HealthCheck: domain.HealthCheck{Enable: true}}, Overrides: []domain.NodeOverride{{Identity: "kept", Name: "custom"}}}
	if e = Save(ctx, s, "source", &src.Meta, &src, ""); e != nil {
		t.Fatal(e)
	}
	p := domain.Profile{Name: "profile", AutoPublish: true, Groups: []domain.Group{{Name: "Proxy", SourceIDs: []string{src.ID}}}}
	if e = Save(ctx, s, "profile", &p.Meta, &p, ""); e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec(`PRAGMA user_version=2`); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(dir, key)
	if e != nil {
		t.Fatal(e)
	}
	got, e := Load[domain.Source](ctx, s, "source", src.ID)
	if e != nil {
		t.Fatal(e)
	}
	if got.Enabled || got.LegacyProxy != src.Fetch.Proxy || len(got.Overrides) != 1 || got.Interval != 7200 || got.Update.Headers["Authorization"] != "fixture-secret" {
		t.Fatal("migration lost source state")
	}
	profile, e := Load[domain.Profile](ctx, s, "profile", p.ID)
	if e != nil || profile.AutoPublish || profile.SourceUpdates[src.ID] != "Independent" {
		t.Fatal("migration lost route")
	}
	rev := got.Revision
	s.Close()
	s, e = Open(dir, key)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	got, e = Load[domain.Source](ctx, s, "source", src.ID)
	if e != nil || got.Revision != rev {
		t.Fatal("migration repeated")
	}
}
