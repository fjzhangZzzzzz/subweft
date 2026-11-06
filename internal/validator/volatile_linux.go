//go:build linux

package validator

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

// Plaintext validation candidates must never share the persistent database volume.
func RequireVolatileDirectory(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("validation_tmpfs_required")
	}
	if e := os.MkdirAll(path, 0700); e != nil {
		return errors.New("validation_directory_unavailable")
	}
	var stat unix.Statfs_t
	if unix.Statfs(path, &stat) != nil || stat.Type != unix.TMPFS_MAGIC {
		return errors.New("validation_tmpfs_required")
	}
	return nil
}
