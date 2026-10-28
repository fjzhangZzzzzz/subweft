package storage

import (
	"context"
	"encoding/json"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

// Cache refreshes change the compilation generation, not the user's definition revision.
func (s *Store) SaveSourceSnapshot(ctx context.Context, v domain.Source) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, `UPDATE objects SET body=? WHERE kind='source' AND id=? AND revision=?`, s.seal(aad("source", v.ID, v.Revision), b), v.ID, v.Revision)
	if e != nil {
		return e
	}
	count, _ := r.RowsAffected()
	if count != 1 {
		return domain.ErrConflict
	}
	if _, e = tx.ExecContext(ctx, `UPDATE meta SET value=CAST(value AS INTEGER)+1 WHERE key='generation'`); e != nil {
		return e
	}
	return tx.Commit()
}
