package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

// The initial slice stores self-contained releases and retains the newest 20 per profile.
func (s *Store) pruneHistory(ctx context.Context, tx *sql.Tx, profileID string) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,revision,body FROM objects WHERE kind='release' ORDER BY rowid DESC`)
	if err != nil {
		return err
	}
	ids := []string{}
	count := 0
	for rows.Next() {
		var id string
		var revision int
		var body []byte
		if err = rows.Scan(&id, &revision, &body); err != nil {
			rows.Close()
			return err
		}
		plain, err := s.open(aad("release", id, revision), body)
		if err != nil {
			rows.Close()
			return err
		}
		var release domain.Release
		if err = json.Unmarshal(plain, &release); err != nil {
			rows.Close()
			return err
		}
		if release.ProfileID == profileID {
			count++
			if count > 20 {
				ids = append(ids, id)
			}
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.ExecContext(ctx, `DELETE FROM objects WHERE kind='release' AND id=? AND NOT EXISTS(SELECT 1 FROM active WHERE release_id=?)`, id, id); err != nil {
			return err
		}
	}
	return nil
}
