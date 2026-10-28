package compile

import (
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/fields"
	"strings"
)

// Names are facts from the pinned core's AdapterType.String contract.
var typeNames = fields.AdapterNames()

func excludedTypes(raw any) (map[string]bool, error) {
	out := map[string]bool{}
	if raw == nil || raw == "" {
		return out, nil
	}
	value, ok := raw.(string)
	if !ok {
		return nil, domain.Invalid("invalid_field_type", "options.exclude-type")
	}
	for _, part := range strings.Split(value, "|") {
		found := false
		for _, name := range typeNames {
			if strings.EqualFold(name, part) {
				out[strings.ToLower(name)] = true
				found = true
				break
			}
		}
		if !found {
			return nil, domain.Invalid("unknown_excluded_type", "options.exclude-type")
		}
	}
	return out, nil
}
