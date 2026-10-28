package source

import (
	"bytes"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"go.yaml.in/yaml/v3"
	"io"
)

// Extraction is explicit: only proxies are imported from a full client config.
func ParseInput(data []byte, extract bool) ([]domain.Node, error) {
	if !extract {
		return Parse(data)
	}
	if len(data) > MaxBytes {
		return nil, domain.Invalid("input_too_large", "content")
	}
	var root yaml.Node
	d := yaml.NewDecoder(bytes.NewReader(data))
	if d.Decode(&root) != nil || len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode || hasAlias(&root, 0) {
		return nil, domain.Invalid("invalid_config", "content")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, domain.Invalid("multiple_documents", "content")
	}
	mapping := root.Content[0]
	seen := map[string]bool{}
	var proxies *yaml.Node
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		if key.Tag != "!!str" || seen[key.Value] {
			return nil, domain.Invalid("duplicate_or_invalid_field", "content")
		}
		seen[key.Value] = true
		if key.Value == "proxies" {
			proxies = mapping.Content[i+1]
		}
	}
	if proxies == nil {
		return nil, domain.Invalid("proxies_required", "content")
	}
	out := yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{{Kind: yaml.ScalarNode, Tag: "!!str", Value: "proxies"}, proxies}}
	encoded, e := yaml.Marshal(&out)
	if e != nil {
		return nil, domain.Invalid("invalid_config", "content")
	}
	return Parse(encoded)
}
