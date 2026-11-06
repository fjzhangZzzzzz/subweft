package storage_test

import (
	"context"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestLiveConsistentBackupRestoresAssetsAndRejectsOverwrite(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	key := filepath.Join(root, "key")
	data := filepath.Join(root, "live")
	if e := storage.Initialize(data, key); e != nil {
		t.Fatal(e)
	}
	live, e := storage.Open(data, key)
	if e != nil {
		t.Fatal(e)
	}
	defer live.Close()
	asset, e := live.PutAsset(ctx, []byte("synthetic immutable payload"))
	if e != nil {
		t.Fatal(e)
	}
	source := domain.Source{Name: "fixture", Kind: "local", Interval: 3600}
	if e = storage.Save(ctx, live, "source", &source.Meta, &source, ""); e != nil {
		t.Fatal(e)
	}
	backup := filepath.Join(root, "snapshot.sqlite")
	if e = storage.BackupDatabase(ctx, data, backup); e != nil {
		t.Fatal(e)
	}
	source.Name = "changed after snapshot"
	if e = storage.Save(ctx, live, "source", &source.Meta, &source, ""); e != nil {
		t.Fatal(e)
	}
	restoredDir := filepath.Join(root, "restored")
	if e = storage.RestoreDatabase(ctx, backup, restoredDir, key); e != nil {
		t.Fatal(e)
	}
	restored, e := storage.Open(restoredDir, key)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	old, e := storage.Load[domain.Source](ctx, restored, "source", source.ID)
	if e != nil || old.Name != "fixture" {
		t.Fatal("backup was not a point-in-time snapshot")
	}
	payload, e := restored.Asset(ctx, asset)
	if e != nil || string(payload) != "synthetic immutable payload" {
		t.Fatal("immutable resource missing from backup")
	}
	if storage.BackupDatabase(ctx, data, backup) == nil || storage.RestoreDatabase(ctx, backup, restoredDir, key) == nil {
		t.Fatal("existing backup or state overwritten")
	}
	raw, _ := os.ReadFile(backup)
	if len(raw) == 0 {
		t.Fatal("backup is empty")
	}
}
