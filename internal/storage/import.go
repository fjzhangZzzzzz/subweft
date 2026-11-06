package storage

import (
	"context"
	"encoding/json"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

type NewObject struct {
	Kind  string
	Meta  domain.Meta
	Value any
}

// ImportDraft creates reviewed definitions atomically. It never writes active pointers.
func (s *Store) ImportDraft(ctx context.Context, generation int64, candidateID string, objects []NewObject) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var current int64
	if e = tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='generation'`).Scan(&current); e != nil {
		return e
	}
	if current != generation {
		return domain.ErrConflict
	}
	var count int
	if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM objects WHERE kind='import-candidate' AND id=?`, candidateID).Scan(&count); e != nil || count != 1 {
		return domain.ErrConflict
	}
	for _, v := range objects {
		if !business(v.Kind) || v.Meta.ID == "" || v.Meta.Revision != 1 {
			return domain.Invalid("invalid_import_object", "import")
		}
		b, e := json.Marshal(v.Value)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO objects(kind,id,revision,body) VALUES(?,?,1,?)`, v.Kind, v.Meta.ID, s.seal(aad(v.Kind, v.Meta.ID, 1), b)); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM objects WHERE kind='import-candidate' AND id=?`, candidateID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE meta SET value=CAST(value AS INTEGER)+1 WHERE key='generation'`); e != nil {
		return e
	}
	return tx.Commit()
}
