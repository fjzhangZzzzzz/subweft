package app_test

import (
	"context"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/app"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
)

func TestLibraryEditsRequireApplyAndDoNotLeakAcrossProfiles(t *testing.T) {
	ctx := context.Background()
	s, _, _, _ := setup(t)
	src, p := create(t, s)
	p.AutoPublish = true
	p, err := s.SaveProfile(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Publish(ctx, p.ID, p.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	assertPending := func(want bool) {
		t.Helper()
		snap, err := s.Store.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if app.PendingProfiles(snap)[p.ID] != want {
			t.Fatal("incorrect pending application status")
		}
	}
	assertPending(false)
	// Editing an unrelated source does not dirty this profile.
	_, err = s.SaveSource(ctx, app.SourceInput{Name: "unrelated", Kind: "local", Content: testfixture.ProviderYAML, Enabled: true, Interval: 3600})
	if err != nil {
		t.Fatal(err)
	}
	assertPending(false)
	_, err = s.SaveOverride(ctx, src.ID, src.Revision, domain.NodeOverride{Identity: domain.NodeIdentity(src.Nodes[0]), Name: "human edit"}, false)
	if err != nil {
		t.Fatal(err)
	}
	assertPending(true)
	s.AutoPublish(ctx)
	snap, err := s.Store.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Active[p.ID] != first.ID {
		t.Fatal("manual library edit applied automatically")
	}
	if _, err = s.Publish(ctx, p.ID, p.Revision, false); err != nil {
		t.Fatal(err)
	}
	assertPending(false)
}

func TestManualNodeAndRulesetEditsRequireApply(t *testing.T) {
	for _, kind := range []string{"node", "ruleset"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			s, _, _, _ := setup(t)
			src, p := create(t, s)
			n := src.Nodes[0]
			n.Name = "manual fixture"
			node, err := s.SaveNode(ctx, domain.ManualNode{Node: n, Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			input := app.ResourceInput{Resource: domain.Resource{Name: "fixture rules", Kind: "local", Type: "ruleset", Behavior: "domain", Format: "yaml", Enabled: true, Interval: 3600}, Entries: []string{"+.example.invalid"}}
			resource, err := s.SaveResource(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			p.Groups[0].NodeIDs = []string{node.ID}
			p.Rules = append([]domain.Rule{{Type: "RULE-SET", Value: resource.ID, Target: "Proxy", Enabled: true}}, p.Rules...)
			p.AutoPublish = true
			p, err = s.SaveProfile(ctx, p)
			if err != nil {
				t.Fatal(err)
			}
			first, err := s.Publish(ctx, p.ID, p.Revision, false)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "node" {
				node.Node.Name = "changed manually"
				_, err = s.SaveNode(ctx, node)
			} else {
				input.Resource = resource
				input.Entries = []string{"+.changed.invalid"}
				_, err = s.SaveResource(ctx, input)
			}
			if err != nil {
				t.Fatal(err)
			}
			s.AutoPublish(ctx)
			snap, err := s.Store.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if snap.Active[p.ID] != first.ID || !app.PendingProfiles(snap)[p.ID] {
				t.Fatal("human dependency edit crossed the apply boundary")
			}
			if _, err = s.Publish(ctx, p.ID, p.Revision, false); err != nil {
				t.Fatal(err)
			}
			snap, err = s.Store.Snapshot(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if app.PendingProfiles(snap)[p.ID] {
				t.Fatal("applied dependency still pending")
			}
		})
	}
}
