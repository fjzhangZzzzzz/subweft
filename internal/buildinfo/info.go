package buildinfo

import (
	_ "embed"
	"runtime"
	"runtime/debug"
	"strings"
)

//go:embed VERSION
var version string

type Information struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Dirty    bool   `json:"dirty"`
	Platform string `json:"platform"`
}

func Current() Information {
	v := Information{Version: strings.TrimSpace(version), Commit: "unknown", Platform: runtime.GOOS + "/" + runtime.GOARCH}
	if b, ok := debug.ReadBuildInfo(); ok {
		for _, s := range b.Settings {
			switch s.Key {
			case "vcs.revision":
				v.Commit = s.Value
			case "vcs.modified":
				v.Dirty = s.Value == "true"
			}
		}
	}
	return v
}
