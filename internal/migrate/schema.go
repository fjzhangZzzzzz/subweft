package migrate

import "github.com/fjzhangZzzzzz/subweft/internal/fields"

func scalar(kind string) *fields.Field { return &fields.Field{Kind: kind} }
func array(kind string) *fields.Field  { return &fields.Field{Kind: "array", Items: scalar(kind)} }
func validateProvider(raw map[string]any) error {
	return fields.Check(&fields.Field{Kind: "object", Properties: map[string]*fields.Field{"name": scalar("string"), "type": scalar("string"), "url": scalar("string"), "path": scalar("string"), "interval": scalar("integer"), "proxy": scalar("string"), "payload": array("any"), "override": {Kind: "object", Properties: map[string]*fields.Field{"additional-prefix": scalar("string")}}, "health-check": {Kind: "object", Properties: map[string]*fields.Field{"enable": scalar("boolean"), "url": scalar("string"), "interval": scalar("integer"), "timeout": scalar("integer"), "lazy": scalar("boolean"), "expected-status": scalar("string")}}}}, raw)
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
