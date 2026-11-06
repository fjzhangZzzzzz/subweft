package source

import (
	"encoding/base64"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"net/url"
	"strings"
)

// URL userinfo is converted to an encrypted request header before the URL reaches
// fetching, redirect checks, metadata or any generated client configuration.
func NormalizeCredentials(raw string, headers map[string]string) (string, map[string]string, error) {
	u, e := url.Parse(raw)
	if e != nil {
		return "", nil, domain.Invalid("invalid_url", "url")
	}
	if u.User == nil {
		return raw, headers, nil
	}
	password, _ := u.User.Password()
	out := map[string]string{}
	for k, v := range headers {
		if strings.EqualFold(k, "Authorization") {
			return "", nil, domain.Invalid("conflicting_credentials", "url")
		}
		out[k] = v
	}
	out["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(u.User.Username()+":"+password))
	u.User = nil
	return u.String(), out, nil
}
