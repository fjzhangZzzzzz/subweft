//go:build !linux

package validator

import "os"

func processPeakRSS(*os.ProcessState) uint64 { return 0 }
