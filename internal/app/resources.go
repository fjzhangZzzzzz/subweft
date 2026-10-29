package app

import (
	"bytes"
	"context"
	"io"
	"net/netip"
	"strings"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"go.yaml.in/yaml/v3"
)

type ResourceInput struct {
	domain.Resource
	Entries []string `json:"entries"`
	Data    []byte   `json:"data,omitempty"`
}

func validateResource(v domain.Resource) error {
	if !domain.ValidName(v.Name) {
		return domain.Invalid("invalid_name", "name")
	}
	if v.Kind != "http" && v.Kind != "local" {
		return domain.Invalid("invalid_source_kind", "kind")
	}
	if v.Interval < 60 || v.Interval > 604800 {
		return domain.Invalid("invalid_interval", "interval")
	}
	switch v.Type {
	case "ruleset":
		if v.Behavior != "domain" && v.Behavior != "ipcidr" && v.Behavior != "classical" {
			return domain.Invalid("invalid_behavior", "behavior")
		}
		if v.Format != "yaml" && v.Format != "text" {
			return domain.Invalid("invalid_format", "format")
		}
	case "mmdb", "geoip", "geosite", "asn":
	default:
		return domain.Invalid("invalid_resource_type", "type")
	}
	if v.Kind == "http" {
		if e := compile.ValidateDeviceUpdate(domain.DeviceUpdate{Headers: v.Headers}); e != nil {
			return e
		}
		return source.ValidateURL(v.URL)
	}
	return nil
}
func normalizeRules(data []byte, behavior, format string, allowEmpty bool) ([]byte, int, error) {
	if len(data) > 16<<20 {
		return nil, 0, domain.Invalid("input_too_large", "entries")
	}
	entries := []string{}
	if format == "yaml" {
		var root yaml.Node
		d := yaml.NewDecoder(bytes.NewReader(data))
		if d.Decode(&root) != nil || len(root.Content) != 1 {
			return nil, 0, domain.Invalid("invalid_ruleset", "entries")
		}
		m := root.Content[0]
		if m.Kind != yaml.MappingNode || len(m.Content) != 2 || m.Content[0].Value != "payload" || m.Content[1].Kind != yaml.SequenceNode {
			return nil, 0, domain.Invalid("invalid_ruleset", "entries")
		}
		for _, v := range m.Content[1].Content {
			if v.Kind != yaml.ScalarNode || v.Tag != "!!str" {
				return nil, 0, domain.Invalid("invalid_field_type", "entries")
			}
			entries = append(entries, v.Value)
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			return nil, 0, domain.Invalid("multiple_documents", "entries")
		}
	} else {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			entries = append(entries, line)
		}
	}
	if (len(entries) == 0 && !allowEmpty) || len(entries) > 500000 {
		return nil, 0, domain.Invalid("invalid_rule_count", "entries")
	}
	seen := map[string]bool{}
	clean := []string{}
	for _, v := range entries {
		if len(v) > 2048 || strings.ContainsAny(v, "\r\n\x00") {
			return nil, 0, domain.Invalid("invalid_rule_value", "entries")
		}
		switch behavior {
		case "domain":
			if strings.ContainsAny(v, " /,:()") || v == "" {
				return nil, 0, domain.Invalid("invalid_rule_value", "entries")
			}
		case "ipcidr":
			if _, e := netip.ParsePrefix(v); e != nil {
				return nil, 0, domain.Invalid("invalid_cidr", "entries")
			}
		case "classical":
			typ, value, ok := strings.Cut(v, ",")
			if !ok || typ == "MATCH" {
				return nil, 0, domain.Invalid("invalid_rule_value", "entries")
			}
			noResolve := strings.HasSuffix(value, ",no-resolve")
			value = strings.TrimSuffix(value, ",no-resolve")
			if _, e := compile.RuleCondition(domain.Condition{Type: typ, Value: value, NoResolve: noResolve}, 0); e != nil {
				return nil, 0, e
			}
		}
		if !seen[v] {
			clean = append(clean, v)
			seen[v] = true
		}
	}
	if format == "text" {
		return []byte(strings.Join(clean, "\n") + "\n"), len(clean), nil
	}
	b, e := yaml.Marshal(map[string]any{"payload": clean})
	return b, len(clean), e
}
func (s *Service) SaveResource(ctx context.Context, in ResourceInput) (domain.Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := in.Resource
	v.Digest = ""
	v.Count = 0
	v.LastSuccess = time.Time{}
	v.LastAttempt = time.Time{}
	v.LastError = ""
	if v.ID != "" {
		old, e := storage.Load[domain.Resource](ctx, s.Store, "resource", v.ID)
		if e != nil {
			return v, e
		}
		if old.Revision != v.Revision {
			return v, domain.ErrConflict
		}
		v.Digest = old.Digest
		v.Count = old.Count
		v.LastSuccess = old.LastSuccess
		v.LastAttempt = old.LastAttempt
		v.LastError = old.LastError
		if v.URL == "" && v.Kind == old.Kind {
			v.URL = old.URL
		}
		if v.Kind != old.Kind || v.Type != old.Type || v.Format != old.Format || v.Behavior != old.Behavior || v.Kind == "http" && v.URL != old.URL {
			v.Digest = ""
			v.Count = 0
			v.LastSuccess = time.Time{}
			v.LastAttempt = time.Time{}
			v.LastError = "source_changed_refresh_required"
		}
	}
	if e := validateResource(v); e != nil {
		return v, e
	}
	if v.Kind == "local" {
		data := in.Data
		var e error
		if v.Type == "ruleset" {
			if in.Entries != nil {
				if v.Format == "yaml" {
					data, e = yaml.Marshal(map[string]any{"payload": in.Entries})
				} else {
					data = []byte(strings.Join(in.Entries, "\n"))
				}
			}
			if data != nil && e == nil {
				data, v.Count, e = normalizeRules(data, v.Behavior, v.Format, true)
			}
		}
		if e != nil {
			return v, e
		}
		if data != nil {
			if len(data) == 0 {
				return v, domain.Invalid("empty_resource", "data")
			}
			v.Digest, e = s.Store.PutAsset(ctx, data)
			if e != nil {
				return v, e
			}
			v.LastSuccess = time.Now().UTC()
			v.LastAttempt = v.LastSuccess
			v.LastError = ""
		}
		if v.Digest == "" {
			return v, domain.Invalid("resource_content_required", "data")
		}
		v.URL = ""
	}
	e := storage.Save(ctx, s.Store, "resource", &v.Meta, &v, "")
	return v, e
}
func (s *Service) RefreshResource(ctx context.Context, id string) (domain.Resource, error) {
	return domain.Resource{}, domain.Invalid("device_updates_only", "resource")
}

func (s *Service) ResourceEntries(ctx context.Context, id string) ([]string, error) {
	v, e := storage.Load[domain.Resource](ctx, s.Store, "resource", id)
	if e != nil {
		return nil, e
	}
	if v.Type != "ruleset" {
		return nil, domain.Invalid("not_ruleset", "resource")
	}
	data, e := s.Store.Asset(ctx, v.Digest)
	if e != nil {
		return nil, e
	}
	if v.Format == "text" {
		text := strings.TrimSuffix(string(data), "\n")
		if text == "" {
			return []string{}, nil
		}
		return strings.Split(text, "\n"), nil
	}
	var m struct {
		Payload []string `yaml:"payload"`
	}
	e = yaml.Unmarshal(data, &m)
	return m.Payload, e
}
