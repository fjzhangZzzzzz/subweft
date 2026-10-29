package compile

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
)

const DeviceUserAgent = "mihomo/1.19.33"

func ValidateDeviceUpdate(u domain.DeviceUpdate) error {
	if e := source.ValidateOptions(domain.FetchOptions{UserAgent: u.UserAgent, Headers: u.Headers}); e != nil {
		return domain.Field("update", e)
	}
	if len(u.Renames) > 100 || len(u.Prefix) > 160 || len(u.Suffix) > 160 || strings.ContainsAny(u.Prefix+u.Suffix, "\r\n\x00") {
		return domain.Invalid("invalid_update", "update")
	}
	patterns := []string{u.Filter, u.Exclude}
	for _, r := range u.Renames {
		patterns = append(patterns, r.Pattern)
		if len(r.Replacement) > 512 || strings.ContainsAny(r.Replacement, "\r\n\x00") {
			return domain.Invalid("invalid_replacement", "update.renames")
		}
	}
	for _, p := range patterns {
		if len(p) > 512 {
			return domain.Invalid("filter_too_long", "update")
		}
		if _, e := regexp.Compile(p); e != nil {
			return domain.Invalid("invalid_filter", "update")
		}
	}
	h := u.HealthCheck
	if h.Enable {
		if source.ValidateURL(h.URL) != nil || h.Interval < 30 || h.Interval > 86400 || h.Timeout < 100 || h.Timeout > 30000 {
			return domain.Invalid("invalid_health_check", "update.healthCheck")
		}
	}
	return nil
}
func deviceHeaders(userAgent string, values map[string]string) map[string][]string {
	if userAgent == "" {
		userAgent = DeviceUserAgent
	}
	h := map[string][]string{"User-Agent": {userAgent}}
	for k, v := range values {
		h[http.CanonicalHeaderKey(k)] = []string{v}
	}
	return h
}

// The definition fingerprint prevents cross-profile and changed-URL cache reuse.
func devicePath(profile, id, extension string, definition any) string {
	b, _ := json.Marshal([]any{profile, id, definition})
	return "./subweft/" + Digest(b) + "." + extension
}
func remoteProvider(s domain.Source, p domain.Profile) (map[string]any, error) {
	if e := domain.ValidateSource(s); e != nil {
		return nil, e
	}
	if e := source.ValidateURL(s.URL); e != nil {
		return nil, e
	}
	if e := ValidateDeviceUpdate(s.Update); e != nil {
		return nil, e
	}
	if len(s.Overrides) > 0 || s.LegacyProxy != "" {
		return nil, domain.Invalid("source_migration_required", "sources."+s.ID)
	}
	proxy := p.SourceUpdates[s.ID]
	if proxy == "" {
		proxy = "DIRECT"
	}
	h := s.Update.HealthCheck
	override := map[string]any{"additional-prefix": "[" + s.Name + "] " + s.Update.Prefix, "additional-suffix": s.Update.Suffix}
	if len(s.Update.Renames) > 0 {
		renames := []map[string]string{}
		for _, r := range s.Update.Renames {
			renames = append(renames, map[string]string{"pattern": r.Pattern, "target": r.Replacement})
		}
		override["proxy-name"] = renames
	}
	out := map[string]any{"type": "http", "url": s.URL, "interval": s.Interval, "proxy": proxy, "header": deviceHeaders(s.Update.UserAgent, s.Update.Headers), "override": override, "health-check": map[string]any{"enable": h.Enable, "url": h.URL, "interval": h.Interval, "timeout": h.Timeout, "lazy": h.Lazy, "expected-status": h.ExpectedStatus}}
	if s.Update.Filter != "" {
		out["filter"] = s.Update.Filter
	}
	if s.Update.Exclude != "" {
		out["exclude-filter"] = s.Update.Exclude
	}
	out["path"] = devicePath(p.ID, s.ID, "yaml", out)
	return out, nil
}
