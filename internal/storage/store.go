package storage

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	_ "modernc.org/sqlite"
)

type Store struct {
	assets    *assetCache
	directory string
	lock      *os.File
	db        *sql.DB
	aead      cipher.AEAD
	key       []byte
	mu        sync.Mutex
}

func Initialize(dir, keyFile string) error {
	if _, e := os.Stat(filepath.Join(dir, "subweft.sqlite")); !errors.Is(e, os.ErrNotExist) {
		return errors.New("database_already_exists")
	}
	if e := os.MkdirAll(filepath.Dir(keyFile), 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return errors.New("key_already_exists_or_unwritable")
	}
	k := make([]byte, 32)
	if _, e = rand.Read(k); e != nil {
		f.Close()
		return e
	}
	_, e = f.Write(k)
	if closeErr := f.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	s, e := open(dir, keyFile, true)
	if e != nil {
		return e
	}
	return s.Close()
}
func Open(dir, keyFile string) (*Store, error) { return open(dir, keyFile, false) }
func open(dir, keyFile string, create bool) (*Store, error) {
	k, e := os.ReadFile(keyFile)
	if e != nil || len(k) != 32 {
		return nil, errors.New("key_missing_or_invalid")
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	lock, e := acquireLock(filepath.Join(dir, ".instance-lock"))
	if e != nil {
		return nil, errors.New("state_directory_in_use_or_unavailable")
	}
	keepLock := false
	defer func() {
		if !keepLock {
			lock.Close()
		}
	}()
	path := filepath.Join(dir, "subweft.sqlite")
	if !create {
		if _, e := os.Stat(path); e != nil {
			return nil, errors.New("database_missing")
		}
	}
	if info, e := os.Lstat(path); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("database_link_rejected")
	}
	block, e := aes.NewCipher(k)
	if e != nil {
		return nil, e
	}
	aead, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &Store{assets: newAssetCache(), directory: dir, db: db, aead: aead, key: k, lock: lock}
	fail := func(err error) (*Store, error) { db.Close(); return nil, err }
	if _, e = db.Exec(`PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;`); e != nil {
		return fail(e)
	}
	var version int
	if e = db.QueryRow(`PRAGMA user_version`).Scan(&version); e != nil {
		return fail(e)
	}
	if version > 3 {
		return fail(errors.New("database_version_too_new"))
	}
	if version == 0 {
		tx, e := db.Begin()
		if e != nil {
			return fail(e)
		}
		_, e = tx.Exec(`CREATE TABLE meta(key TEXT PRIMARY KEY,value BLOB NOT NULL);
CREATE TABLE objects(kind TEXT NOT NULL,id TEXT NOT NULL,revision INTEGER NOT NULL,body BLOB NOT NULL,lookup TEXT NOT NULL DEFAULT '',PRIMARY KEY(kind,id));
CREATE UNIQUE INDEX device_lookup ON objects(lookup) WHERE kind='device';
CREATE TABLE active(profile_id TEXT PRIMARY KEY,release_id TEXT NOT NULL);
CREATE TABLE device_seen(device_id TEXT PRIMARY KEY,last_download TEXT NOT NULL);
INSERT INTO meta VALUES('generation','0'); PRAGMA user_version=1;`)
		if e == nil {
			_, e = tx.Exec(`INSERT INTO meta VALUES('keycheck',?)`, s.seal("keycheck", []byte("subweft-key-v1")))
		}
		if e != nil {
			tx.Rollback()
			return fail(e)
		}
		if e = tx.Commit(); e != nil {
			return fail(e)
		}
	}
	var check []byte
	if e = db.QueryRow(`SELECT value FROM meta WHERE key='keycheck'`).Scan(&check); e != nil {
		return fail(e)
	}
	plain, e := s.open("keycheck", check)
	if e != nil || string(plain) != "subweft-key-v1" {
		return fail(errors.New("key_does_not_match_database"))
	}
	if e = os.Chmod(path, 0600); e != nil {
		return fail(e)
	}
	if e = s.ensureReleaseIndex(); e != nil {
		return fail(e)
	}
	if e = s.migrateDeviceUpdates(); e != nil {
		return fail(e)
	}
	keepLock = true
	return s, nil
}
func (s *Store) Close() error {
	err := s.db.Close()
	if s.lock != nil {
		_ = s.lock.Close()
	}
	return err
}
func (s *Store) seal(aad string, b []byte) []byte {
	n := make([]byte, s.aead.NonceSize())
	if _, e := rand.Read(n); e != nil {
		panic(e)
	}
	return s.aead.Seal(n, n, b, []byte(aad))
}
func (s *Store) open(aad string, b []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(b) < n {
		return nil, errors.New("invalid_ciphertext")
	}
	return s.aead.Open(nil, b[:n], b[n:], []byte(aad))
}
func (s *Store) TokenHash(token string) string {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte("device-token\x00" + token))
	return hex.EncodeToString(h.Sum(nil))
}
func aad(kind, id string, revision int) string { return kind + "/" + id + "/" + strconv.Itoa(revision) }
func business(kind string) bool {
	return kind == "source" || kind == "node" || kind == "profile" || kind == "resource"
}

func Save[T any](ctx context.Context, s *Store, kind string, meta *domain.Meta, value T, lookup string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	expected := meta.Revision
	if expected < 0 {
		return domain.ErrConflict
	}
	if meta.ID == "" {
		meta.ID = domain.NewID()
	}
	meta.Revision = expected + 1
	b, e := json.Marshal(value)
	if e != nil {
		return e
	}
	body := s.seal(aad(kind, meta.ID, meta.Revision), b)
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if expected == 0 {
		_, e = tx.ExecContext(ctx, `INSERT INTO objects(kind,id,revision,body,lookup) VALUES(?,?,?,?,?)`, kind, meta.ID, meta.Revision, body, lookup)
	} else {
		var result sql.Result
		result, e = tx.ExecContext(ctx, `UPDATE objects SET revision=?,body=?,lookup=? WHERE kind=? AND id=? AND revision=?`, meta.Revision, body, lookup, kind, meta.ID, expected)
		if e == nil {
			n, _ := result.RowsAffected()
			if n != 1 {
				return domain.ErrConflict
			}
		}
	}
	if e != nil {
		return e
	}
	if business(kind) {
		if _, e = tx.ExecContext(ctx, `UPDATE meta SET value=CAST(value AS INTEGER)+1 WHERE key='generation'`); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func Load[T any](ctx context.Context, s *Store, kind, id string) (T, error) {
	var v T
	var b []byte
	var rev int
	e := s.db.QueryRowContext(ctx, `SELECT revision,body FROM objects WHERE kind=? AND id=?`, kind, id).Scan(&rev, &b)
	if errors.Is(e, sql.ErrNoRows) {
		return v, domain.ErrNotFound
	}
	if e != nil {
		return v, e
	}
	plain, e := s.open(aad(kind, id, rev), b)
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(plain, &v)
	return v, e
}
func List[T any](ctx context.Context, s *Store, kind string) ([]T, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,revision,body FROM objects WHERE kind=? ORDER BY id`, kind)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	values := []T{}
	for rows.Next() {
		var id string
		var rev int
		var b []byte
		if e = rows.Scan(&id, &rev, &b); e != nil {
			return nil, e
		}
		plain, e := s.open(aad(kind, id, rev), b)
		if e != nil {
			return nil, e
		}
		var v T
		if e = json.Unmarshal(plain, &v); e != nil {
			return nil, e
		}
		values = append(values, v)
	}
	return values, rows.Err()
}
func (s *Store) Delete(ctx context.Context, kind, id string, revision int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, `DELETE FROM objects WHERE kind=? AND id=? AND revision=?`, kind, id, revision)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return domain.ErrConflict
	}
	if business(kind) {
		if _, e = tx.ExecContext(ctx, `UPDATE meta SET value=CAST(value AS INTEGER)+1 WHERE key='generation'`); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *Store) Snapshot(ctx context.Context) (domain.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var snap domain.Snapshot
	var e error
	if e = s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='generation'`).Scan(&snap.Generation); e != nil {
		return snap, e
	}
	if snap.Sources, e = List[domain.Source](ctx, s, "source"); e != nil {
		return snap, e
	}
	if snap.Resources, e = List[domain.Resource](ctx, s, "resource"); e != nil {
		return snap, e
	}
	if snap.Nodes, e = List[domain.ManualNode](ctx, s, "node"); e != nil {
		return snap, e
	}
	if snap.Profiles, e = List[domain.Profile](ctx, s, "profile"); e != nil {
		return snap, e
	}
	for i := range snap.Profiles {
		domain.NormalizeProfile(&snap.Profiles[i])
	}
	if snap.Devices, e = List[domain.Device](ctx, s, "device"); e != nil {
		return snap, e
	}
	if snap.Releases, e = s.ReleaseSummaries(ctx); e != nil {
		return snap, e
	}
	snap.Active = map[string]string{}
	rows, e := s.db.QueryContext(ctx, `SELECT profile_id,release_id FROM active`)
	if e != nil {
		return snap, e
	}
	defer rows.Close()
	for rows.Next() {
		var p, r string
		if e = rows.Scan(&p, &r); e != nil {
			return snap, e
		}
		snap.Active[p] = r
	}
	return snap, rows.Err()
}
func (s *Store) Publish(ctx context.Context, generation int64, r *domain.Release) error {
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
	r.ID = domain.NewID()
	r.Revision = 1
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO objects(kind,id,revision,body) VALUES('release',?,1,?)`, r.ID, s.seal(aad("release", r.ID, 1), b)); e != nil {
		return e
	}
	summary := *r
	summary.Config = nil
	summary.Definition = nil
	summaryBytes, _ := json.Marshal(summary)
	if _, e = tx.ExecContext(ctx, `INSERT INTO release_index(id,body) VALUES(?,?)`, r.ID, s.seal(aad("release-index", r.ID, 1), summaryBytes)); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO active VALUES(?,?) ON CONFLICT(profile_id) DO UPDATE SET release_id=excluded.release_id`, r.ProfileID, r.ID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE meta SET value=CAST(value AS INTEGER)+1 WHERE key='generation'`); e != nil {
		return e
	}
	if e = s.pruneHistory(ctx, tx, r.ProfileID); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Activate(ctx context.Context, profileID, releaseID string, profileRevision int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := Load[domain.Profile](ctx, s, "profile", profileID)
	if e != nil {
		return e
	}
	if p.Revision != profileRevision {
		return domain.ErrConflict
	}
	r, e := Load[domain.Release](ctx, s, "release", releaseID)
	if e != nil {
		return e
	}
	if r.ProfileID != profileID {
		return domain.Invalid("wrong_profile", "releaseId")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `INSERT INTO active VALUES(?,?) ON CONFLICT(profile_id) DO UPDATE SET release_id=excluded.release_id`, profileID, releaseID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE meta SET value=CAST(value AS INTEGER)+1 WHERE key='generation'`); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Download(ctx context.Context, token string) (domain.Device, domain.Release, error) {
	var d domain.Device
	var r domain.Release
	var id string
	if len(token) != 43 {
		return d, r, domain.ErrNotFound
	}
	e := s.db.QueryRowContext(ctx, `SELECT id FROM objects WHERE kind='device' AND lookup=?`, s.TokenHash(token)).Scan(&id)
	if errors.Is(e, sql.ErrNoRows) {
		return d, r, domain.ErrNotFound
	}
	if e != nil {
		return d, r, e
	}
	d, e = Load[domain.Device](ctx, s, "device", id)
	if e != nil {
		return d, r, e
	}
	if d.State != "active" || (d.ExpiresAt != nil && !time.Now().Before(*d.ExpiresAt)) {
		return d, r, domain.ErrNotFound
	}
	var releaseID string
	if e = s.db.QueryRowContext(ctx, `SELECT release_id FROM active WHERE profile_id=?`, d.ProfileID).Scan(&releaseID); e != nil {
		return d, r, domain.ErrNotFound
	}
	r, e = Load[domain.Release](ctx, s, "release", releaseID)
	return d, r, e
}
func (s *Store) Seen(ctx context.Context, id string) {
	_, _ = s.db.ExecContext(ctx, `INSERT INTO device_seen VALUES(?,?) ON CONFLICT(device_id) DO UPDATE SET last_download=excluded.last_download`, id, time.Now().UTC().Format(time.RFC3339))
}
func (s *Store) SeenAll(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	rows, e := s.db.QueryContext(ctx, `SELECT device_id,last_download FROM device_seen`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id, t string
		if e = rows.Scan(&id, &t); e != nil {
			return nil, e
		}
		out[id] = t
	}
	return out, rows.Err()
}
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *Store) String() string                 { return fmt.Sprintf("encrypted-sqlite") }
