package compile

import (
	"go.yaml.in/yaml/v3"
	"strings"
)

// Rewrite only service-owned resource URL positions, never credentials or user fields.
func DeviceConfig(config []byte, base string) ([]byte, error) {
	var document yaml.Node
	if e := yaml.Unmarshal(config, &document); e != nil {
		return nil, e
	}
	if len(document.Content) != 1 {
		return config, nil
	}
	root := document.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i].Value, root.Content[i+1]
		if key == "geox-url" {
			for j := 1; j < len(value.Content); j += 2 {
				n := value.Content[j]
				if strings.HasPrefix(n.Value, ResourcePrefix) {
					n.Value = base + strings.TrimPrefix(n.Value, ResourcePrefix)
				}
			}
		}
		if key == "rule-providers" || key == "proxy-providers" {
			for j := 1; j < len(value.Content); j += 2 {
				provider := value.Content[j]
				for k := 0; k < len(provider.Content); k += 2 {
					if provider.Content[k].Value == "url" {
						n := provider.Content[k+1]
						if strings.HasPrefix(n.Value, ResourcePrefix) {
							n.Value = base + strings.TrimPrefix(n.Value, ResourcePrefix)
						}
					}
				}
			}
		}
	}
	return yaml.Marshal(&document)
}
