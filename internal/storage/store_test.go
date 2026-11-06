package storage_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
)

func TestEncryptedPersistenceAndRealCopyRestore(t *testing.T) {
	ctx := context.Background()
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
	v := domain.Source{Name: "sample", Kind: "http", URL: "https://example.com/?secret=fixture-private-upstream", Content: "fixture-private-password", Interval: 3600}
	if e = storage.Save(ctx, s, "source", &v.Meta, &v, ""); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(filepath.Join(dir, "subweft.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{"fixture-private-upstream", "fixture-private-password"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatal("database contains plaintext secret")
		}
	}
	restore := filepath.Join(root, "restored")
	if e = os.MkdirAll(restore, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(restore, "subweft.sqlite"), data, 0600); e != nil {
		t.Fatal(e)
	}
	keyBytes, e := os.ReadFile(key)
	if e != nil {
		t.Fatal(e)
	}
	restoreKey := filepath.Join(root, "restored-key")
	if e = os.WriteFile(restoreKey, keyBytes, 0600); e != nil {
		t.Fatal(e)
	}
	s, e = storage.Open(restore, restoreKey)
	if e != nil {
		t.Fatal(e)
	}
	got, e := storage.Load[domain.Source](ctx, s, "source", v.ID)
	if e != nil || got.URL != v.URL || got.Revision != 1 {
		t.Fatal("restored content differs")
	}
	s.Close()
	if e = os.WriteFile(restoreKey, bytes.Repeat([]byte{7}, 32), 0600); e != nil {
		t.Fatal(e)
	}
	if bad, e := storage.Open(restore, restoreKey); e == nil {
		bad.Close()
		t.Fatal("wrong key accepted")
	}
	if e = os.Remove(restoreKey); e != nil {
		t.Fatal(e)
	}
	if bad, e := storage.Open(restore, restoreKey); e == nil {
		bad.Close()
		t.Fatal("missing key accepted")
	}
	if bad, e := storage.Open(filepath.Join(root, "absent"), key); e == nil {
		bad.Close()
		t.Fatal("missing database silently initialized")
	}
}
func TestRevisionAndPublishGeneration(t *testing.T) {
	ctx := context.Background()
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
	defer s.Close()
	p := domain.Profile{Name: "profile"}
	if e = storage.Save(ctx, s, "profile", &p.Meta, &p, ""); e != nil {
		t.Fatal(e)
	}
	stale := p
	if e = storage.Save(ctx, s, "profile", &p.Meta, &p, ""); e != nil {
		t.Fatal(e)
	}
	if e = storage.Save(ctx, s, "profile", &stale.Meta, &stale, ""); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale update accepted")
	}
	snap, e := s.Snapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	release := domain.Release{ProfileID: p.ID, Config: []byte("fixture")}
	if e = s.Publish(ctx, snap.Generation, &release); e != nil {
		t.Fatal(e)
	}
	if e = s.Publish(ctx, snap.Generation, &domain.Release{ProfileID: p.ID}); !errors.Is(e, domain.ErrConflict) {
		t.Fatal("stale publication accepted")
	}
	after, e := s.Snapshot(ctx)
	if e != nil || after.Active[p.ID] != release.ID {
		t.Fatal("active publication changed incorrectly")
	}
}

func TestSingleInstanceAndBoundedReleaseHistory(t *testing.T) {
	ctx := context.Background()
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
	defer s.Close()
	if second, e := storage.Open(dir, key); e == nil {
		second.Close()
		t.Fatal("second writer accepted")
	}
	p := domain.Profile{Name: "history"}
	if e = storage.Save(ctx, s, "profile", &p.Meta, &p, ""); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 23; i++ {
		snap, e := s.Snapshot(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if e = s.Publish(ctx, snap.Generation, &domain.Release{ProfileID: p.ID, Config: []byte("fixture")}); e != nil {
			t.Fatal(e)
		}
	}
	snap, e := s.Snapshot(ctx)
	if e != nil || len(snap.Releases) != 20 {
		t.Fatal("history is not bounded")
	}
	found := false
	for _, r := range snap.Releases {
		if r.ID == snap.Active[p.ID] {
			found = true
		}
	}
	if !found {
		t.Fatal("current release pruned")
	}
}
