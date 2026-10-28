package source

import (
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"go.yaml.in/yaml/v3"
)

// Check scalars before decoding: YAML string coercion must not change credentials.
func validateNodeYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return domain.Invalid("invalid_node", "node")
	}
	protocol := ""
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == "type" {
			protocol = node.Content[i+1].Value
		}
	}
	supported := false
	for _, p := range domain.Protocols() {
		if p == protocol {
			supported = true
		}
	}
	if !supported {
		return domain.Invalid("unsupported_protocol", "type")
	}
	policy := domain.NodeFields(protocol, true)
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		if !policy[key] {
			return domain.Invalid("unsupported_or_invalid_field", "node")
		}
		tag := "!!str"
		switch key {
		case "port", "alterId":
			tag = "!!int"
		case "udp", "tls", "skip-cert-verify":
			tag = "!!bool"
		case "alpn":
			if !stringSequence(value) {
				return domain.Invalid("invalid_field_type", key)
			}
			continue
		case "ws-opts":
			if e := optionMap(value, map[string]string{"path": "!!str", "headers": "map", "max-early-data": "!!int", "early-data-header-name": "!!str"}); e != nil {
				return domain.Field(key, e)
			}
			continue
		case "grpc-opts":
			if e := optionMap(value, map[string]string{"grpc-service-name": "!!str"}); e != nil {
				return domain.Field(key, e)
			}
			continue
		case "reality-opts":
			if e := optionMap(value, map[string]string{"public-key": "!!str", "short-id": "!!str"}); e != nil {
				return domain.Field(key, e)
			}
			continue
		}
		if value.Kind != yaml.ScalarNode || value.Tag != tag {
			return domain.Invalid("invalid_field_type", key)
		}
	}
	return nil
}
func stringSequence(n *yaml.Node) bool {
	if n.Kind != yaml.SequenceNode {
		return false
	}
	for _, v := range n.Content {
		if v.Kind != yaml.ScalarNode || v.Tag != "!!str" {
			return false
		}
	}
	return true
}
func optionMap(n *yaml.Node, policy map[string]string) error {
	if n.Kind != yaml.MappingNode {
		return domain.Invalid("invalid_field_type", "options")
	}
	for i := 0; i < len(n.Content); i += 2 {
		key, value := n.Content[i].Value, n.Content[i+1]
		tag, ok := policy[key]
		if !ok {
			return domain.Invalid("unsupported_or_invalid_field", "options")
		}
		if tag == "map" {
			if value.Kind != yaml.MappingNode {
				return domain.Invalid("invalid_field_type", key)
			}
			for j := 0; j < len(value.Content); j += 2 {
				if value.Content[j].Tag != "!!str" || value.Content[j+1].Kind != yaml.ScalarNode || value.Content[j+1].Tag != "!!str" {
					return domain.Invalid("invalid_field_type", key)
				}
			}
		} else if value.Kind != yaml.ScalarNode || value.Tag != tag {
			return domain.Invalid("invalid_field_type", key)
		}
	}
	return nil
}
