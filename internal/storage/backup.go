package storage

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// BackupDatabase uses SQLite's consistent snapshot operation, including while the
// service is writing WAL. It exports encrypted state only; the separate key is mandatory.
func BackupDatabase(ctx context.Context, dir, destination string) error {
	if !filepath.IsAbs(destination) {
		return errors.New("backup_output_must_be_absolute")
	}
	if _, e := os.Lstat(destination); !errors.Is(e, os.ErrNotExist) {
		return errors.New("backup_output_exists_or_unavailable")
	}
	source := filepath.Join(dir, "subweft.sqlite")
	if info, e := os.Lstat(source); e != nil || !info.Mode().IsRegular() {
		return errors.New("database_missing_or_invalid")
	}
	db, e := sql.Open("sqlite", source)
	if e != nil {
		return e
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, e = db.ExecContext(ctx, `PRAGMA busy_timeout=30000`); e != nil {
		return e
	}
	private, e := os.MkdirTemp(filepath.Dir(destination), ".subweft-backup-")
	if e != nil {
		return errors.New("backup_directory_unavailable")
	}
	defer os.RemoveAll(private)
	temp := filepath.Join(private, "snapshot.sqlite")
	if _, e = db.ExecContext(ctx, `VACUUM main INTO ?`, temp); e != nil {
		return errors.New("consistent_backup_failed")
	}
	if e = os.Chmod(temp, 0600); e != nil {
		return e
	}
	if e = checkDatabase(ctx, temp); e != nil {
		return e
	}
	if _, e = os.Lstat(destination); !errors.Is(e, os.ErrNotExist) {
		return errors.New("backup_output_exists_or_unavailable")
	}
	return os.Rename(temp, destination)
}
func checkDatabase(ctx context.Context, file string) error {
	db, e := sql.Open("sqlite", file)
	if e != nil {
		return e
	}
	defer db.Close()
	var result string
	if e = db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); e != nil || result != "ok" {
		return errors.New("backup_integrity_failed")
	}
	var count int
	if e = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM meta WHERE key='keycheck'`).Scan(&count); e != nil || count != 1 {
		return errors.New("not_subweft_backup")
	}
	return nil
}
func RestoreDatabase(ctx context.Context, backup, dir, keyFile string) error {
	target := filepath.Join(dir, "subweft.sqlite")
	if _, e := os.Lstat(target); !errors.Is(e, os.ErrNotExist) {
		return errors.New("restore_requires_new_state_directory")
	}
	if e := checkDatabase(ctx, backup); e != nil {
		return e
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	in, e := os.Open(backup)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	if e == nil {
		e = out.Sync()
	}
	closeErr := out.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	s, e := Open(dir, keyFile)
	if e != nil {
		return e
	}
	defer s.Close()
	rows, e := s.db.QueryContext(ctx, `SELECT kind,id,revision,body FROM objects`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var kind, id string
		var revision int
		var body []byte
		if e = rows.Scan(&kind, &id, &revision, &body); e != nil {
			return e
		}
		if _, e = s.open(aad(kind, id, revision), body); e != nil {
			return errors.New("backup_decryption_failed")
		}
	}
	return rows.Err()
}

func (s *Store) PrepareBackup(ctx context.Context) (string, func(), error) {
	private, e := os.MkdirTemp(s.directory, "backup-")
	if e != nil {
		return "", func() {}, e
	}
	cleanup := func() { _ = os.RemoveAll(private) }
	file := filepath.Join(private, "subweft.sqlite")
	if e = BackupDatabase(ctx, s.directory, file); e != nil {
		cleanup()
		return "", func() {}, e
	}
	return file, cleanup, nil
}
func (s *Store) ExportBackup(ctx context.Context, w io.Writer) error {
	file, cleanup, e := s.PrepareBackup(ctx)
	if e != nil {
		return e
	}
	defer cleanup()
	in, e := os.Open(file)
	if e != nil {
		return e
	}
	defer in.Close()
	_, e = io.Copy(w, in)
	return e
}
