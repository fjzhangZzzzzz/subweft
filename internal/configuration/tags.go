package configuration

import (
	"strings"
	"unicode"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

const SystemUnclassifiedTag = "未分类"

const (
	maxTagsPerSubscription = 10
	maxTagLength           = 32
	maxDistinctTags        = 100
)

func normalizeTag(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == SystemUnclassifiedTag || len([]rune(value)) > maxTagLength || strings.ContainsAny(value, "\r\n,，") || strings.ContainsFunc(value, unicode.IsControl) {
		return "", domain.Invalid("invalid_tag", "tags")
	}
	return value, nil
}

// NormalizeTags keeps the first spelling while comparing English labels
// case-insensitively. The system label is derived for subscriptions and is
// therefore never persisted in their custom tag list.
func NormalizeTags(values []string, allowSystem bool) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if value == SystemUnclassifiedTag {
			if !allowSystem {
				return nil, domain.Invalid("invalid_tag", "tags")
			}
			if !seen[value] {
				seen[value] = true
				out = append(out, value)
			}
			continue
		}
		value, err := normalizeTag(value)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	if len(out) > maxTagsPerSubscription && !allowSystem {
		return nil, domain.Invalid("tag_limit", "subscriptions.tags")
	}
	if len(out) > maxDistinctTags {
		return nil, domain.Invalid("tag_limit", "tags")
	}
	return out, nil
}

func effectiveTags(s Subscription) []string {
	if len(s.Tags) == 0 {
		return []string{SystemUnclassifiedTag}
	}
	return s.Tags
}

func tagsMatch(s Subscription, selected []string) bool {
	if len(selected) == 0 {
		return false
	}
	available := effectiveTags(s)
	for _, wanted := range selected {
		for _, actual := range available {
			if strings.EqualFold(wanted, actual) {
				return true
			}
		}
	}
	return false
}

func allTags(c Config) []string {
	seen := map[string]bool{SystemUnclassifiedTag: true}
	out := []string{SystemUnclassifiedTag}
	for _, s := range c.Subscriptions {
		for _, tag := range effectiveTags(s) {
			key := strings.ToLower(tag)
			if !seen[key] {
				seen[key] = true
				out = append(out, tag)
			}
		}
	}
	return out
}
