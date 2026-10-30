package compile

import (
	"net/url"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"go.yaml.in/yaml/v3"
)

func providerResource(s domain.Source, nodes []domain.Node) (string, map[string]any, []byte, error) {
	settings := s.ClientProvider
	if settings == nil || !settings.Enabled {
		return "", nil, nil, domain.Invalid("provider_disabled", "source")
	}
	if settings.Interval < 60 || settings.Interval > 604800 {
		return "", nil, nil, domain.Invalid("invalid_interval", "clientProvider.interval")
	}
	check := settings.HealthCheck
	if check.Enable {
		u, e := url.Parse(check.URL)
		if e != nil || u.Hostname() == "" || u.User != nil || u.Scheme != "https" && u.Scheme != "http" || check.Interval < 30 || check.Interval > 86400 || check.Timeout < 100 || check.Timeout > 30000 {
			return "", nil, nil, domain.Invalid("invalid_health_check", "clientProvider.healthCheck")
		}
	}
	id := "provider-" + s.ID
	data, e := yaml.Marshal(map[string]any{"proxies": nodes})
	if e != nil {
		return "", nil, nil, e
	}
	out := map[string]any{"type": "http", "url": ResourceURL(id), "path": "./providers/" + id + ".yaml", "interval": settings.Interval, "health-check": map[string]any{"enable": check.Enable, "url": check.URL, "interval": check.Interval, "timeout": check.Timeout, "lazy": check.Lazy, "expected-status": check.ExpectedStatus}}
	if settings.Proxy != "" {
		out["proxy"] = settings.Proxy
	}
	return id, out, data, nil
}
