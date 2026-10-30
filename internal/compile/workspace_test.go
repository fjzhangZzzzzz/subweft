package compile_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/validator"
	"go.yaml.in/yaml/v3"
)

func TestNamedSharedProvidersPreservePurposeSelections(t *testing.T) {
	p, snap := fixture(t)
	snap.Sources[0].Name = "示例订阅"
	snap.Sources[0].ClientProvider = &domain.ClientProvider{Enabled: true, Interval: 3600}
	p.Pools = []domain.SourcePool{{Name: "常用", SourceIDs: []string{"source"}}}
	p.Groups = []domain.Group{
		{Name: "全部", Type: "select", Pools: []string{"常用"}},
		{Name: "精选", Type: "select", Pools: []string{"常用"}, Filter: "QUOTA"},
	}
	p.Rules = []domain.Rule{{Type: "MATCH", Target: "全部", Enabled: true}}
	result, err := compile.Compile(p, snap, false)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err = yaml.Unmarshal(result.Config, &document); err != nil {
		t.Fatal(err)
	}
	providers := document["proxy-providers"].(map[string]any)
	if len(providers) != 1 || providers["示例订阅"] == nil {
		t.Fatal("provider names or sharing incorrect")
	}
	groups := document["proxy-groups"].([]any)
	for i, raw := range groups {
		g := raw.(map[string]any)
		if !reflect.DeepEqual(g["use"], []any{"示例订阅"}) {
			t.Fatal("unreadable provider reference")
		}
		var asset struct {
			Proxies []domain.Node `yaml:"proxies"`
		}
		for _, data := range result.Assets {
			if err = yaml.Unmarshal(data, &asset); err != nil {
				t.Fatal(err)
			}
		}
		selected := []string{}
		for _, n := range asset.Proxies {
			if filter, ok := g["filter"].(string); !ok || regexp.MustCompile(filter).MatchString(n.Name) {
				selected = append(selected, n.Name)
			}
		}
		if !reflect.DeepEqual(selected, result.Members[p.Groups[i].Name]) {
			t.Fatal("shared provider changed selected members")
		}
	}
	p.Groups[0], p.Groups[1] = p.Groups[1], p.Groups[0]
	reordered, err := compile.Compile(p, snap, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Resources, reordered.Resources) {
		t.Fatal("group ordering changed provider resources")
	}
	if len(p.Groups[0].SourceIDs) != 0 {
		t.Fatal("pool expansion mutated saved definition")
	}
	binary := "mihomo"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	path, _ := filepath.Abs(filepath.Join("..", "..", "output", "tools", binary))
	if _, err = os.Stat(path); err != nil {
		t.Skip("pinned validator unavailable; compile assertions passed")
	}
	sha, ok := validator.ExpectedSHA(runtime.GOOS, runtime.GOARCH)
	if !ok {
		t.Skip("platform not locked")
	}
	checker, err := validator.New(path, sha, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = checker.CheckBundle(context.Background(), result.Config, result.Assets); err != nil {
		t.Fatal("native validator rejected shared providers", err)
	}
}

func TestReadableRulesetNamesAndCollisions(t *testing.T) {
	p, snap := fixture(t)
	snap.Resources = []domain.Resource{{Meta: domain.Meta{ID: "opaque-rule-id"}, Name: "示例域名", Type: "ruleset", Kind: "local", Behavior: "domain", Format: "yaml", Enabled: true, Digest: "fixture-digest"}}
	p.Rules = append([]domain.Rule{{Type: "RULE-SET", Value: "opaque-rule-id", Target: "Proxy", Enabled: true}}, p.Rules...)
	result, err := compile.Compile(p, snap, false)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err = yaml.Unmarshal(result.Config, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["rule-providers"].(map[string]any)["示例域名"] == nil || doc["rules"].([]any)[0] != "RULE-SET,示例域名,Proxy" {
		t.Fatal("rule name or reference unreadable")
	}
	duplicate := snap.Resources[0]
	duplicate.ID = "another-id"
	snap.Resources = append(snap.Resources, duplicate)
	p.Rules = append([]domain.Rule{{Type: "RULE-SET", Value: duplicate.ID, Target: "Proxy", Enabled: true}}, p.Rules...)
	if _, err = compile.Compile(p, snap, false); err == nil {
		t.Fatal("duplicate rule provider names accepted")
	}
}

func TestPoolReferenceBoundaries(t *testing.T) {
	for _, name := range []string{"missing-pool", "missing-source", "duplicate-pool", "empty-pool"} {
		t.Run(name, func(t *testing.T) {
			p, snap := fixture(t)
			p.Groups[0].AllSources = false
			p.Groups[0].Pools = []string{"pool"}
			p.Pools = []domain.SourcePool{{Name: "pool", SourceIDs: []string{"source"}}}
			switch name {
			case "missing-pool":
				p.Pools = nil
			case "missing-source":
				p.Pools[0].SourceIDs = []string{"absent"}
			case "duplicate-pool":
				p.Pools = append(p.Pools, p.Pools[0])
			case "empty-pool":
				p.Pools[0].SourceIDs = nil
			}
			if _, err := compile.Compile(p, snap, false); err == nil {
				t.Fatal("invalid pool accepted")
			}
		})
	}
}
