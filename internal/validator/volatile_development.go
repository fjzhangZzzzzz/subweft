//go:build !linux

package validator

import "errors"

func RequireVolatileDirectory(path string) error { return errors.New("production_requires_linux") }
