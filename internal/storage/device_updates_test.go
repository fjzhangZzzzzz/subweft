package storage

import (
	"bytes"
	"context"
	"fmt"
	"os"
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
	implicit := domain.Source{Name: "implicit", Kind: "http", ClientProvider: &domain.ClientProvider{}}
	if e = Save(ctx, s, "source", &implicit.Meta, &implicit, ""); e != nil {
		t.Fatal(e)
	}
	p := domain.Profile{Name: "profile", AutoPublish: true, Groups: []domain.Group{{Name: "Proxy", SourceIDs: []string{src.ID, implicit.ID}}}}
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
	if route, found := profile.SourceUpdates[implicit.ID]; !found || route != "" {
		t.Fatal("migration changed implicit route")
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

func TestDeviceMigrationRollsBackEveryWriteOnMidTransactionFailure(t *testing.T) {
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
	defer s.Close()
	src := domain.Source{Name: "source", Kind: "http", Interval: 3600, Fetch: domain.FetchOptions{UserAgent: "fixture-agent"}}
	if e = Save(ctx, s, "source", &src.Meta, &src, ""); e != nil {
		t.Fatal(e)
	}
	p := domain.Profile{Name: "profile", AutoPublish: true}
	if e = Save(ctx, s, "profile", &p.Meta, &p, ""); e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"device", "user", "release"} {
		v := domain.Meta{}
		if e = Save(ctx, s, kind, &v, &v, kind); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = s.db.Exec(`PRAGMA user_version=2; CREATE TRIGGER fail_profile BEFORE UPDATE ON objects WHEN OLD.kind='profile' BEGIN SELECT RAISE(ABORT,'synthetic migration failure'); END;`); e != nil {
		t.Fatal(e)
	}
	state := func() string {
		t.Helper()
		rows, e := s.db.Query(`SELECT kind,id,revision,hex(body),lookup FROM objects ORDER BY rowid`)
		if e != nil {
			t.Fatal(e)
		}
		out := ""
		for rows.Next() {
			var kind, id, body, lookup string
			var rev int
			if e = rows.Scan(&kind, &id, &rev, &body, &lookup); e != nil {
				t.Fatal(e)
			}
			out += fmt.Sprint(kind, id, rev, body, lookup)
		}
		rows.Close()
		var version int
		var generation string
		_ = s.db.QueryRow(`PRAGMA user_version`).Scan(&version)
		_ = s.db.QueryRow(`SELECT value FROM meta WHERE key='generation'`).Scan(&generation)
		return out + fmt.Sprint(version, generation)
	}
	before := state()
	keyBefore, _ := os.ReadFile(key)
	if e = s.migrateDeviceUpdates(); e == nil {
		t.Fatal("injected failure ignored")
	}
	if state() != before {
		t.Fatal("migration changed state despite rollback")
	}
	keyAfter, _ := os.ReadFile(key)
	if !bytes.Equal(keyBefore, keyAfter) {
		t.Fatal("migration changed master key")
	}
	if _, e = s.db.Exec(`DROP TRIGGER fail_profile`); e != nil {
		t.Fatal(e)
	}
	if e = s.migrateDeviceUpdates(); e != nil {
		t.Fatal(e)
	}
	got, e := Load[domain.Source](ctx, s, "source", src.ID)
	if e != nil || got.Update.UserAgent != "fixture-agent" {
		t.Fatal("retry failed")
	}
}
