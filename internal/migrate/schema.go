package migrate

import "github.com/fjzhangZzzzzz/subweft/internal/fields"

func scalar(kind string) *fields.Field { return &fields.Field{Kind: kind} }
func array(kind string) *fields.Field  { return &fields.Field{Kind: "array", Items: scalar(kind)} }
func validateProvider(raw map[string]any) error {
	props := map[string]*fields.Field{}
	for _, key := range []string{"name", "type", "url", "path", "proxy", "filter", "exclude-filter"} {
		props[key] = scalar("string")
	}
	props["interval"] = scalar("integer")
	props["payload"] = array("any")
	props["header"] = &fields.Field{Kind: "map", Values: array("string")}
	props["override"] = &fields.Field{Kind: "object", Properties: map[string]*fields.Field{
		"additional-prefix": scalar("string"), "additional-suffix": scalar("string"),
		"proxy-name": {Kind: "array", Items: &fields.Field{Kind: "object", Properties: map[string]*fields.Field{"pattern": scalar("string"), "target": scalar("string")}}},
	}}
	props["health-check"] = &fields.Field{Kind: "object", Properties: map[string]*fields.Field{"enable": scalar("boolean"), "url": scalar("string"), "interval": scalar("integer"), "timeout": scalar("integer"), "lazy": scalar("boolean"), "expected-status": scalar("string")}}
	return fields.Check(&fields.Field{Kind: "object", Properties: props}, raw)
}
func validateGroup(raw map[string]any) error {
	root := fields.Schema("group:" + text(raw["type"]))
	if root == nil {
		return fields.Validate("group:"+text(raw["type"]), raw)
	}
	properties := map[string]*fields.Field{}
	for k, v := range root.Properties {
		properties[k] = v
	}
	for _, key := range []string{"name", "type", "filter", "exclude-filter", "url", "strategy"} {
		properties[key] = scalar("string")
	}
	for _, key := range []string{"interval", "tolerance"} {
		properties[key] = scalar("integer")
	}
	properties["use"] = array("string")
	properties["proxies"] = array("string")
	return fields.Check(&fields.Field{Kind: "object", Properties: properties}, raw)
}
