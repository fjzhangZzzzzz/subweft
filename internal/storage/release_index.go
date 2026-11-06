package storage

import (
	"context"
	"encoding/json"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

// This encrypted derived index keeps history listing independent of configuration size.
// Existing preview databases are migrated after their key has been verified.
func (s *Store) ensureReleaseIndex() error {
	if _, e := s.db.Exec(`CREATE TABLE IF NOT EXISTS release_index(id TEXT PRIMARY KEY,body BLOB NOT NULL)`); e != nil {
		return e
	}
	rows, e := s.db.Query(`SELECT id,revision,body FROM objects WHERE kind='release' AND NOT EXISTS(SELECT 1 FROM release_index WHERE release_index.id=objects.id) ORDER BY rowid`)
	if e != nil {
		return e
	}
	type entry struct {
		id   string
		body []byte
	}
	pending := []entry{}
	for rows.Next() {
		var id string
		var revision int
		var body []byte
		if e = rows.Scan(&id, &revision, &body); e != nil {
			rows.Close()
			return e
		}
		plain, e := s.open(aad("release", id, revision), body)
		if e != nil {
			rows.Close()
			return e
		}
		var v domain.Release
		if e = json.Unmarshal(plain, &v); e != nil {
			rows.Close()
			return e
		}
		v.Config = nil
		v.Definition = nil
		encoded, _ := json.Marshal(v)
		pending = append(pending, entry{id, s.seal(aad("release-index", id, 1), encoded)})
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return e
	}
	rows.Close()
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, v := range pending {
		if _, e = tx.Exec(`INSERT INTO release_index VALUES(?,?)`, v.id, v.body); e != nil {
			return e
		}
	}
	var version int
	if e = tx.QueryRow(`PRAGMA user_version`).Scan(&version); e != nil {
		return e
	}
	if version < 2 {
		if _, e = tx.Exec(`PRAGMA user_version=2`); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *Store) ReleaseSummaries(ctx context.Context) ([]domain.Release, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,body FROM release_index ORDER BY rowid DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Release{}
	for rows.Next() {
		var id string
		var body []byte
		if e = rows.Scan(&id, &body); e != nil {
			return nil, e
		}
		plain, e := s.open(aad("release-index", id, 1), body)
		if e != nil {
			return nil, e
		}
		var v domain.Release
		if e = json.Unmarshal(plain, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
