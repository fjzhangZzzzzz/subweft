package domain

import (
	"crypto/sha256"
	"encoding/hex"
)

type User struct {
	Meta
	Issuer     string   `json:"issuer"`
	Subject    string   `json:"subject"`
	Name       string   `json:"name"`
	Role       string   `json:"role"`
	Enabled    bool     `json:"enabled"`
	ProfileIDs []string `json:"profileIds"`
	Bootstrap  bool     `json:"bootstrap,omitempty"`
}

func UserID(issuer, subject string) string {
	h := sha256.Sum256([]byte(issuer + "\x00" + subject))
	return hex.EncodeToString(h[:])
}
func (u User) Owner() string { return u.Issuer + "#" + u.Subject }
func (u User) Allows(profile string) bool {
	if !u.Enabled {
		return false
	}
	if u.Role == "admin" {
		return true
	}
	for _, id := range u.ProfileIDs {
		if id == profile {
			return true
		}
	}
	return false
}
