package configuration

import (
	"net/netip"
	"strings"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/fields"
)

// DNS editing is deliberately bounded. Other imported fields remain unchanged.
var editableDNS = []string{"enable", "ipv6", "enhanced-mode", "nameserver", "respect-rules", "default-nameserver", "proxy-server-nameserver", "nameserver-policy", "fallback", "fake-ip-range", "fake-ip-filter", "use-hosts"}
var editableFallback = []string{"geoip", "geoip-code", "ipcidr", "domain"}

func baseWithoutEditableDNS(base map[string]any) map[string]any {
	c := Clone(Config{Base: base}).Base
	dns, ok := c["dns"].(map[string]any)
	if !ok {
		return c
	}
	for _, key := range editableDNS {
		delete(dns, key)
	}
	if filter, ok := dns["fallback-filter"].(map[string]any); ok {
		for _, key := range editableFallback {
			delete(filter, key)
		}
		if len(filter) == 0 {
			delete(dns, "fallback-filter")
		}
	}
	if len(dns) == 0 {
		delete(c, "dns")
	}
	return c
}

func validateDNS(base map[string]any) error {
	value, exists := base["dns"]
	if !exists {
		return nil
	}
	dns, ok := value.(map[string]any)
	if !ok {
		return domain.Invalid("invalid_dns", "dns")
	}
	subset := map[string]any{}
	for _, key := range editableDNS {
		if v, present := dns[key]; present {
			subset[key] = v
		}
	}
	if v, present := dns["fallback-filter"]; present {
		filter, ok := v.(map[string]any)
		if !ok {
			return domain.Invalid("invalid_dns", "fallback-filter")
		}
		selected := map[string]any{}
		for _, key := range editableFallback {
			if v, present := filter[key]; present {
				selected[key] = v
			}
		}
		subset["fallback-filter"] = selected
	}
	if err := fields.Validate("dns", subset); err != nil {
		return domain.Invalid("invalid_dns", "dns")
	}
	// Bound user-maintained lists and reject empty/control-character values without
	// exposing potentially credential-bearing DNS addresses in errors.
	var values func(any) bool
	values = func(v any) bool {
		switch x := v.(type) {
		case string:
			return x != "" && len(x) <= 2048 && strings.TrimSpace(x) == x && !strings.ContainsAny(x, "\r\n\x00")
		case []any:
			if len(x) > 128 {
				return false
			}
			for _, v := range x {
				if !values(v) {
					return false
				}
			}
		case []string:
			if len(x) > 128 {
				return false
			}
			for _, v := range x {
				if !values(v) {
					return false
				}
			}
		case map[string]any:
			if len(x) > 128 {
				return false
			}
			for k, v := range x {
				if !values(k) || !values(v) {
					return false
				}
			}
		}
		return true
	}
	if !values(subset) {
		return domain.Invalid("invalid_dns", "dns")
	}
	if v, ok := dns["fake-ip-range"].(string); ok {
		p, e := netip.ParsePrefix(v)
		if e != nil || !p.Addr().Is4() {
			return domain.Invalid("invalid_dns", "fake-ip-range")
		}
	}
	if filter, ok := dns["fallback-filter"].(map[string]any); ok {
		if v, ok := filter["geoip-code"].(string); ok {
			if len(v) != 2 || v[0] < 'A' || v[0] > 'Z' || v[1] < 'A' || v[1] > 'Z' {
				return domain.Invalid("invalid_dns", "fallback-filter.geoip-code")
			}
		}
		normalized := fields.Normalize(filter)
		if m, ok := normalized.(map[string]any); ok {
			if list, ok := m["ipcidr"].([]any); ok {
				for _, v := range list {
					if _, e := netip.ParsePrefix(v.(string)); e != nil {
						return domain.Invalid("invalid_dns", "fallback-filter.ipcidr")
					}
				}
			}
		}
	}
	return nil
}
