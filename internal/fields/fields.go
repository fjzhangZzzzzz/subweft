// Package fields owns the reviewed, pinned core field contract shared with forms.
package fields

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

//go:embed catalog.json
var Catalog []byte

type Field struct {
	Choices      []string          `json:"choices,omitempty"`
	Separator    string            `json:"separator,omitempty"`
	Kind         string            `json:"kind"`
	Required     bool              `json:"required,omitempty"`
	Secret       bool              `json:"secret,omitempty"`
	Enum         []string          `json:"enum,omitempty"`
	Properties   map[string]*Field `json:"properties,omitempty"`
	Items        *Field            `json:"items,omitempty"`
	Values       *Field            `json:"values,omitempty"`
	Alternatives []*Field          `json:"alternatives,omitempty"`
	Min          *float64          `json:"min,omitempty"`
	Max          *float64          `json:"max,omitempty"`
}
type Registry struct {
	AdapterNames map[string]string `json:"adapterNames"`
	Version      string            `json:"version"`
	Protocols    map[string]*Field `json:"protocols"`
	Plugins      map[string]*Field `json:"plugins"`
	Groups       map[string]*Field `json:"groups"`
	DNS          *Field            `json:"dns"`
	Config       *Field            `json:"config"`
}

var registry Registry

func AdapterNames() map[string]string { return registry.AdapterNames }

func init() {
	if err := json.Unmarshal(Catalog, &registry); err != nil {
		panic(err)
	}
}
func Protocols() []string {
	out := []string{}
	for k := range registry.Protocols {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func Schema(kind string) *Field {
	switch kind {
	case "dns":
		return registry.DNS
	case "config":
		return registry.Config
	}
	if strings.HasPrefix(kind, "group:") {
		return registry.Groups[strings.TrimPrefix(kind, "group:")]
	}
	return registry.Protocols[kind]
}

type Error struct{ Code, Path string }

func (e *Error) Error() string        { return e.Code }
func invalid(code, path string) error { return &Error{code, path} }

// DecodeJSON preserves numeric intent, rejects duplicate keys, and never includes values in errors.
func DecodeJSON(data []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	v, e := decode(d, 0)
	if e != nil {
		return nil, e
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, invalid("invalid_json", "")
	}
	return v, nil
}
func decode(d *json.Decoder, depth int) (any, error) {
	if depth > 24 {
		return nil, invalid("input_too_deep", "")
	}
	t, e := d.Token()
	if e != nil {
		return nil, e
	}
	switch v := t.(type) {
	case json.Delim:
		switch v {
		case '{':
			m := map[string]any{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return nil, e
				}
				s, ok := key.(string)
				if !ok {
					return nil, invalid("invalid_json", "")
				}
				if _, exists := m[s]; exists {
					return nil, invalid("duplicate_field", s)
				}
				x, e := decode(d, depth+1)
				if e != nil {
					return nil, e
				}
				m[s] = x
			}
			_, e = d.Token()
			return m, e
		case '[':
			a := []any{}
			for d.More() {
				x, e := decode(d, depth+1)
				if e != nil {
					return nil, e
				}
				a = append(a, x)
			}
			_, e = d.Token()
			return a, e
		}
	case nil:
		return nil, invalid("invalid_field_type", "")
	}
	return t, nil
}
func Validate(kind string, value any) error { return ValidateInput(kind, value, true) }
func Check(schema *Field, value any) error {
	b, e := json.Marshal(value)
	if e != nil {
		return invalid("invalid_field_type", "")
	}
	v, e := DecodeJSON(b)
	if e != nil {
		return e
	}
	return check(schema, v, "", 0)
}
func ValidateInput(kind string, value any, complete bool) error {
	root := Schema(kind)
	if root != nil && !complete {
		copy := *root
		copy.Properties = map[string]*Field{}
		for k, v := range root.Properties {
			p := *v
			p.Required = false
			copy.Properties[k] = &p
		}
		root = &copy
	}
	if root == nil {
		return invalid("unsupported_protocol", "type")
	}
	// Decode native Go values through JSON to give all callers identical scalar semantics.
	b, e := json.Marshal(value)
	if e != nil {
		return invalid("invalid_field_type", kind)
	}
	v, e := DecodeJSON(b)
	if e != nil {
		return e
	}
	if e = check(root, v, "", 0); e != nil {
		return e
	}
	if kind == "ss" {
		m := v.(map[string]any)
		if opts, ok := m["plugin-opts"]; ok {
			plugin, _ := m["plugin"].(string)
			s := registry.Plugins[plugin]
			if s == nil {
				return invalid("plugin_required", "plugin")
			}
			if e = check(s, opts, "plugin-opts", 0); e != nil {
				return e
			}
		}
	}
	return nil
}
func check(f *Field, v any, path string, depth int) error {
	if f == nil || v == nil {
		return invalid("invalid_field_type", path)
	}
	if depth > 24 {
		return invalid("input_too_deep", path)
	}
	switch f.Kind {
	case "union":
		for _, option := range f.Alternatives {
			if check(option, v, path, depth+1) == nil {
				return nil
			}
		}
		return invalid("invalid_field_type", path)
	case "object", "map":
		m, ok := v.(map[string]any)
		if !ok {
			return invalid("invalid_field_type", path)
		}
		if len(m) > 10000 {
			return invalid("too_many_items", path)
		}
		for key, field := range f.Properties {
			if field.Required {
				if x, exists := m[key]; !exists || x == "" {
					return invalid("required_field", join(path, key))
				}
			}
		}
		seenHeaders := map[string]bool{}
		for key, x := range m {
			if f.Kind == "map" && (strings.HasSuffix(path, "headers") || strings.HasSuffix(path, "ws-headers")) {
				canonical := strings.ToLower(key)
				if !HeaderToken(key) || seenHeaders[canonical] {
					return invalid("invalid_or_duplicate_header", path)
				}
				seenHeaders[canonical] = true
			}
			if key == "" || len(key) > 2048 || strings.ContainsAny(key, "\r\n\x00") {
				return invalid("invalid_field_name", path)
			}
			field := f.Values
			if f.Kind == "object" {
				field = f.Properties[key]
				if field == nil {
					return invalid("unsupported_or_invalid_field", join(path, key))
				}
			}
			if e := check(field, x, join(path, key), depth+1); e != nil {
				return e
			}
		}
	case "array":
		a, ok := v.([]any)
		if !ok {
			return invalid("invalid_field_type", path)
		}
		if len(a) > 10000 {
			return invalid("too_many_items", path)
		}
		for i, x := range a {
			if e := check(f.Items, x, fmt.Sprintf("%s.%d", path, i), depth+1); e != nil {
				return e
			}
		}
	case "string":
		s, ok := v.(string)
		if !ok {
			return invalid("invalid_field_type", path)
		}
		if len(s) > 65536 || strings.ContainsRune(s, 0) {
			return invalid("invalid_field_value", path)
		}
		if len(f.Enum) > 0 {
			found := false
			for _, v := range f.Enum {
				found = found || v == s
			}
			if !found {
				return invalid("unsupported_field_value", path)
			}
		}
		// Core accepts paths here; Subweft only accepts inline PEM to keep server files private.
		if strings.Contains(path, "headers.") && strings.ContainsAny(s, "\r\n") {
			return invalid("invalid_header", path)
		}
		key := path[strings.LastIndex(path, ".")+1:]
		if (key == "certificate" || key == "private-key" && strings.Contains(s, "-----")) && s != "" && !strings.HasPrefix(s, "-----BEGIN ") {
			return invalid("inline_pem_required", path)
		}
		if key == "private-key" && s != "" && !strings.HasPrefix(s, "-----BEGIN ") {
			decoded, e := base64.StdEncoding.DecodeString(s)
			if e != nil || len(decoded) != 32 {
				return invalid("inline_key_required", path)
			}
		}
		if key == "private-key" && s != "" && !strings.HasPrefix(s, "-----BEGIN ") && len(s) != 44 {
			return invalid("inline_key_required", path)
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return invalid("invalid_field_type", path)
		}
	case "integer", "number":
		n, ok := v.(json.Number)
		if !ok {
			return invalid("invalid_field_type", path)
		}
		x, e := n.Float64()
		if e != nil || math.IsInf(x, 0) || math.IsNaN(x) || f.Kind == "integer" && (math.Trunc(x) != x || math.Abs(x) > 9007199254740991) || f.Min != nil && x < *f.Min || f.Max != nil && x > *f.Max {
			return invalid("invalid_field_value", path)
		}
	case "any":
		switch x := v.(type) {
		case string:
			return check(&Field{Kind: "string"}, x, path, depth+1)
		case bool, json.Number:
			return nil
		case []any:
			return check(&Field{Kind: "array", Items: &Field{Kind: "any"}}, x, path, depth+1)
		case map[string]any:
			return check(&Field{Kind: "map", Values: &Field{Kind: "any"}}, x, path, depth+1)
		default:
			return invalid("invalid_field_type", path)
		}
	default:
		return invalid("invalid_field_type", path)
	}
	return nil
}
func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + "." + b
}

// YAMLValue inspects tags rather than allowing YAML's permissive string coercion.
func YAMLValue(n *yaml.Node) (any, error) { return yamlValue(n, 0) }
func yamlValue(n *yaml.Node, depth int) (any, error) {
	if depth > 24 {
		return nil, invalid("input_too_deep", "")
	}
	switch n.Kind {
	case yaml.MappingNode:
		m := map[string]any{}
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Tag != "!!str" {
				return nil, invalid("invalid_field_type", "")
			}
			if _, ok := m[k.Value]; ok {
				return nil, invalid("duplicate_field", k.Value)
			}
			v, e := yamlValue(n.Content[i+1], depth+1)
			if e != nil {
				return nil, e
			}
			m[k.Value] = v
		}
		return m, nil
	case yaml.SequenceNode:
		a := []any{}
		for _, c := range n.Content {
			v, e := yamlValue(c, depth+1)
			if e != nil {
				return nil, e
			}
			a = append(a, v)
		}
		return a, nil
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!str":
			return n.Value, nil
		case "!!bool":
			var v bool
			e := n.Decode(&v)
			return v, e
		case "!!int", "!!float":
			var v any
			if e := n.Decode(&v); e != nil {
				return nil, e
			}
			switch x := v.(type) {
			case int:
				return json.Number(fmt.Sprint(x)), nil
			case uint64:
				return json.Number(fmt.Sprint(x)), nil
			case float64:
				return json.Number(fmt.Sprint(x)), nil
			}
		}
	}
	return nil, invalid("invalid_field_type", "")
}
func Redact(kind string, value any) any { return redact(Schema(kind), value) }
func redact(f *Field, v any) any {
	if f != nil && f.Secret {
		return "[REDACTED]"
	}
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, v := range x {
			var child *Field
			if f != nil {
				child = f.Properties[k]
				if child == nil {
					child = f.Values
				}
			}
			if k == "plugin-opts" {
				if plugin, ok := x["plugin"].(string); ok {
					child = registry.Plugins[plugin]
				}
			}
			out[k] = redact(child, v)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			var child *Field
			if f != nil {
				child = f.Items
			}
			out[i] = redact(child, v)
		}
		return out
	}
	return v
}

// Normalize converts json.Number to native scalars for YAML encoding.
func Normalize(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, e := x.Int64(); e == nil {
			return i
		}
		f, _ := x.Float64()
		return f
	case map[string]any:
		out := map[string]any{}
		for k, v := range x {
			out[k] = Normalize(v)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = Normalize(v)
		}
		return out
	default:
		rv := reflect.ValueOf(v)
		if rv.IsValid() && rv.Kind() == reflect.Ptr && !rv.IsNil() {
			return Normalize(rv.Elem().Interface())
		}
		return v
	}
}

func HeaderToken(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if c > 127 || !strings.ContainsRune("!#$%&'*+-.^_`|~0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", c) {
			return false
		}
	}
	return true
}
