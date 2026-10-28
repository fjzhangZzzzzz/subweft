package source

import (
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"go.yaml.in/yaml/v3"
)

func validateNodeYAML(n *yaml.Node) error { var node domain.Node; return n.Decode(&node) }
