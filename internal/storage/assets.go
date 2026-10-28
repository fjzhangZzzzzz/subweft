package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

func (s *Store) PutAsset(ctx context.Context, data []byte) (string, error) {
	if len(data) > 64<<20 {
		return "", domain.Invalid("input_too_large", "resource")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	h := sha256.Sum256(data)
	id := hex.EncodeToString(h[:])
	b, e := json.Marshal(data)
	if e != nil {
		return "", e
	}
	var exists int
	if e = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM objects WHERE kind='asset' AND id=?`, id).Scan(&exists); e != nil {
		return "", e
	}
	if exists == 1 {
		_, e = s.db.ExecContext(ctx, `UPDATE objects SET lookup=? WHERE kind='asset' AND id=?`, time.Now().UTC().Format(time.RFC3339), id)
		return id, e
	}
	var total int64
	if e = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(LENGTH(body)),0) FROM objects WHERE kind='asset'`).Scan(&total); e != nil {
		return "", e
	}
	if total+int64(len(b)) > 1<<30 {
		return "", domain.Invalid("asset_storage_limit", "resource")
	}
	_, e = s.db.ExecContext(ctx, `INSERT INTO objects(kind,id,revision,body,lookup) VALUES('asset',?,1,?,?)`, id, s.seal(aad("asset", id, 1), b), time.Now().UTC().Format(time.RFC3339))
	return id, e
}
func (s *Store) Asset(ctx context.Context, id string) ([]byte, error) {
	return s.assets.get(ctx, id, func() ([]byte, error) { return s.loadAsset(ctx, id) })
}
func (s *Store) loadAsset(ctx context.Context, id string) ([]byte, error) {
	if len(id) != 64 {
		return nil, domain.ErrNotFound
	}
	b, e := Load[[]byte](ctx, s, "asset", id)
	if e != nil {
		return nil, e
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != id {
		return nil, domain.Invalid("asset_digest_mismatch", "resource")
	}
	return b, nil
}
func (s *Store) SaveResourceSnapshot(ctx context.Context, v domain.Resource) error {
	return saveSnapshot(ctx, s, "resource", v.Meta, v)
}
func saveSnapshot[T any](ctx context.Context, s *Store, kind string, meta domain.Meta, value T) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, e := json.Marshal(value)
	if e != nil {
		return e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, `UPDATE objects SET body=? WHERE kind=? AND id=? AND revision=?`, s.seal(aad(kind, meta.ID, meta.Revision), b), kind, meta.ID, meta.Revision)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return domain.ErrConflict
	}
	if _, e = tx.ExecContext(ctx, `UPDATE meta SET value=CAST(value AS INTEGER)+1 WHERE key='generation'`); e != nil {
		return e
	}
	return tx.Commit()
}

// ResourceDownload validates the current device first, then the retained release's profile and exact manifest.
func (s *Store) ResourceDownload(ctx context.Context, token, releaseID, name string) ([]byte, error) {
	d, _, e := s.Download(ctx, token)
	if e != nil {
		return nil, e
	}
	r, e := Load[domain.Release](ctx, s, "release", releaseID)
	if e != nil || r.ProfileID != d.ProfileID {
		return nil, domain.ErrNotFound
	}
	digest, ok := r.Resources[name]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return s.Asset(ctx, digest)
}

func (s *Store) PruneAssets(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	resources, e := List[domain.Resource](ctx, s, "resource")
	if e != nil {
		return e
	}
	releases, e := s.ReleaseSummaries(ctx)
	if e != nil {
		return e
	}
	keep := map[string]bool{}
	for _, r := range resources {
		keep[r.Digest] = true
	}
	for _, r := range releases {
		for _, digest := range r.Resources {
			keep[digest] = true
		}
	}
	rows, e := s.db.QueryContext(ctx, "SELECT id FROM objects WHERE kind='asset' AND lookup < ?", time.Now().Add(-24*time.Hour).UTC().Format(time.RFC3339))
	if e != nil {
		return e
	}
	remove := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		if !keep[id] {
			remove = append(remove, id)
		}
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return e
	}
	rows.Close()
	for _, id := range remove {
		if _, e = s.db.ExecContext(ctx, "DELETE FROM objects WHERE kind='asset' AND id=?", id); e != nil {
			return e
		}
	}
	return nil
}
