package storage

import (
	"encoding/json"
	"fmt"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

// Upgrade definitions atomically. Historical releases and encrypted assets stay intact.
func (s *Store) migrateDeviceUpdates() error {
	var version int
	if e := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); e != nil {
		return e
	}
	if version >= 3 {
		return nil
	}
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	rows, e := tx.Query(`SELECT kind,id,revision,body FROM objects WHERE kind IN ('source','profile') ORDER BY rowid`)
	if e != nil {
		return e
	}
	type entry struct {
		kind, id string
		revision int
		plain    []byte
	}
	entries := []entry{}
	sources := []domain.Source{}
	for rows.Next() {
		var v entry
		var body []byte
		if e = rows.Scan(&v.kind, &v.id, &v.revision, &body); e != nil {
			rows.Close()
			return e
		}
		v.plain, e = s.open(aad(v.kind, v.id, v.revision), body)
		if e != nil {
			rows.Close()
			return e
		}
		entries = append(entries, v)
		if v.kind == "source" {
			var src domain.Source
			if e = json.Unmarshal(v.plain, &src); e != nil {
				rows.Close()
				return e
			}
			sources = append(sources, src)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range entries {
		var body []byte
		if v.kind == "source" {
			var src domain.Source
			if e = json.Unmarshal(v.plain, &src); e != nil {
				return e
			}
			if src.Kind == "http" {
				src.Update.UserAgent = src.Fetch.UserAgent
				src.Update.Headers = src.Fetch.Headers
				src.LegacyProxy = src.Fetch.Proxy
				if src.ClientProvider != nil {
					src.Update.HealthCheck = src.ClientProvider.HealthCheck
					if src.ClientProvider.Interval > 0 {
						src.Interval = src.ClientProvider.Interval
					}
				}
			}
			src.ClientProvider = nil
			src.Fetch = domain.FetchOptions{}
			src.Revision = v.revision + 1
			body, e = json.Marshal(src)
		} else {
			var p domain.Profile
			if e = json.Unmarshal(v.plain, &p); e != nil {
				return e
			}
			p.AutoPublish = false
			if p.SourceUpdates == nil {
				p.SourceUpdates = map[string]string{}
			}
			for _, src := range sources {
				if src.Kind != "http" {
					continue
				}
				selected := false
				for _, g := range p.Groups {
					for _, id := range g.SourceIDs {
						if id == src.ID {
							selected = true
						}
					}
					if src.Enabled {
						if g.AllSources {
							selected = true
						}
						for _, a := range g.Tags {
							for _, b := range src.Tags {
								if a == b {
									selected = true
								}
							}
						}
					}
				}
				if selected {
					route := "DIRECT"
					if src.ClientProvider != nil && src.ClientProvider.Proxy != "" {
						route = src.ClientProvider.Proxy
					}
					p.SourceUpdates[src.ID] = route
				}
			}
			p.Revision = v.revision + 1
			body, e = json.Marshal(p)
		}
		if e != nil {
			return e
		}
		if _, e = tx.Exec(`UPDATE objects SET revision=?,body=? WHERE kind=? AND id=?`, v.revision+1, s.seal(aad(v.kind, v.id, v.revision+1), body), v.kind, v.id); e != nil {
			return fmt.Errorf("device_update_migration: %w", e)
		}
	}
	if len(entries) > 0 {
		if _, e = tx.Exec(`UPDATE meta SET value=CAST(value AS INTEGER)+1 WHERE key='generation'`); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(`PRAGMA user_version=3`); e != nil {
		return e
	}
	return tx.Commit()
}
