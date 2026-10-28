package validator

import (
	_ "embed"
	"encoding/json"
)

//go:embed lock.json
var artifactLock []byte

func ExpectedSHA(goos, goarch string) (string, bool) {
	var lock struct {
		Artifacts map[string]struct {
			SHA string `json:"binary_sha256"`
		} `json:"artifacts"`
	}
	if json.Unmarshal(artifactLock, &lock) != nil {
		return "", false
	}
	a, ok := lock.Artifacts[goos+"-"+goarch]
	return a.SHA, ok && len(a.SHA) == 64
}
