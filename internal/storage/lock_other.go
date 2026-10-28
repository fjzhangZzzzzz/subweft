//go:build !linux && !windows

package storage

import (
	"errors"
	"os"
)

func acquireLock(path string) (*os.File, error) { return nil, errors.New("unsupported_platform") }
