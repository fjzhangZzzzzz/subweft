package app_test

import (
	"bytes"
	"context"
	"github.com/fjzhangZzzzzz/subweft/internal/app"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
	"testing"
)

func (c *checker) CheckBundle(ctx context.Context, config []byte, assets map[string][]byte) error {
	return c.Check(ctx, config)
}
func TestResourceVersionsStayBoundToReleaseAndRevokedDevice(t *testing.T) {
	ctx := context.Background()
	s, c, _, _ := setup(t)
	_, p := create(t, s)
	input := app.ResourceInput{Resource: domain.Resource{Name: "self rules", Kind: "local", Type: "ruleset", Behavior: "domain", Format: "yaml", Enabled: true, Interval: 3600}, Entries: []string{"+.example.invalid"}}
	resource, e := s.SaveResource(ctx, input)
	if e != nil {
		t.Fatal(e)
	}
	p.Rules = append([]domain.Rule{{Type: "RULE-SET", Value: resource.ID, Target: "Proxy", Enabled: true}}, p.Rules...)
	p, e = s.SaveProfile(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	first, e := s.Publish(ctx, p.ID, p.Revision, false)
	if e != nil {
		t.Fatal(e)
	}
	device, e := s.SaveDevice(ctx, domain.Device{Name: "laptop", ProfileID: p.ID, State: "active"}, "fixture")
	if e != nil {
		t.Fatal(e)
	}
	before, e := s.Store.ResourceDownload(ctx, device.Token, first.ID, resource.ID)
	if e != nil {
		t.Fatal(e)
	}
	input.Resource = resource
	input.Entries = []string{"+.changed.invalid"}
	resource, e = s.SaveResource(ctx, input)
	if e != nil {
		t.Fatal(e)
	}
	c.reject.Store(true)
	if _, e = s.Publish(ctx, p.ID, p.Revision, false); e == nil {
		t.Fatal("invalid bundle activated")
	}
	_, active, _ := s.Store.Download(ctx, device.Token)
	if active.ID != first.ID {
		t.Fatal("failed resource publish changed active release")
	}
	c.reject.Store(false)
	second, e := s.Publish(ctx, p.ID, p.Revision, false)
	if e != nil {
		t.Fatal(e)
	}
	old, e := s.Store.ResourceDownload(ctx, device.Token, first.ID, resource.ID)
	if e != nil || !bytes.Equal(old, before) {
		t.Fatal("immutable resource changed")
	}
	current, e := s.Store.ResourceDownload(ctx, device.Token, second.ID, resource.ID)
	if e != nil || bytes.Equal(current, before) {
		t.Fatal("new resource version unavailable")
	}
	other := app.CloneProfile(p)
	other, e = s.SaveProfile(ctx, other)
	if e != nil {
		t.Fatal(e)
	}
	foreign, e := s.Publish(ctx, other.ID, other.Revision, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.ResourceDownload(ctx, device.Token, foreign.ID, resource.ID); e == nil {
		t.Fatal("cross-profile resource access accepted")
	}
	device.State = "revoked"
	if _, e = s.SaveDevice(ctx, device, "fixture"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.ResourceDownload(ctx, device.Token, first.ID, resource.ID); e == nil {
		t.Fatal("revoked resource URL still usable")
	}
}
func TestAutomaticPublishUsesFrozenDefinitionAndKeepsLastGood(t *testing.T) {
	ctx := context.Background()
	s, c, _, _ := setup(t)
	src, p := create(t, s)
	p.AutoPublish = true
	p, e := s.SaveProfile(ctx, p)
	if e != nil {
		t.Fatal(e)
	}
	first, e := s.Publish(ctx, p.ID, p.Revision, false)
	if e != nil {
		t.Fatal(e)
	}
	draft := p
	draft.Rules[0].Value = "unpublished-draft.invalid"
	draft, e = s.SaveProfile(ctx, draft)
	if e != nil {
		t.Fatal(e)
	}
	src, e = s.SaveSource(ctx, app.SourceInput{Meta: src.Meta, Name: src.Name, Kind: "local", Content: testfixture.ProviderYAML + "\n", Enabled: true, Interval: 3600})
	if e != nil {
		t.Fatal(e)
	}
	n := src.Nodes[0]
	_, e = s.SaveOverride(ctx, src.ID, src.Revision, domain.NodeOverride{Identity: domain.NodeIdentity(n), Name: "renamed locally"}, false)
	if e != nil {
		t.Fatal(e)
	}
	s.AutoPublish(ctx)
	snap, e := s.Store.Snapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	next, e := storage.Load[domain.Release](ctx, s.Store, "release", snap.Active[p.ID])
	if e != nil || next.ID == first.ID || bytes.Contains(next.Config, []byte("unpublished-draft.invalid")) || next.ProfileRevision != p.Revision {
		t.Fatal("automatic update published draft or missed source change")
	}
	src, _ = storage.Load[domain.Source](ctx, s.Store, "source", src.ID)
	_, e = s.SaveOverride(ctx, src.ID, src.Revision, domain.NodeOverride{Identity: domain.NodeIdentity(n), Name: "another change"}, false)
	if e != nil {
		t.Fatal(e)
	}
	c.reject.Store(true)
	s.AutoPublish(ctx)
	snap, _ = s.Store.Snapshot(ctx)
	if snap.Active[p.ID] != next.ID {
		t.Fatal("automatic validation failure replaced last good release")
	}
}
func TestOverridesSurviveUpstreamRenameAndCredentialsButRefuseAmbiguity(t *testing.T) {
	ctx := context.Background()
	s, _, _, _ := setup(t)
	src, _ := create(t, s)
	identity := domain.NodeIdentity(src.Nodes[0])
	src, e := s.SaveOverride(ctx, src.ID, src.Revision, domain.NodeOverride{Identity: identity, Name: "local label", Notes: "fixture"}, false)
	if e != nil {
		t.Fatal(e)
	}
	src.Nodes[0].Name = "upstream renamed"
	src.Nodes[0].Password = "changed credential"
	effective, e := domain.SourceNodes(src)
	if e != nil || effective[0].Name != "local label" {
		t.Fatal("override followed mutable name or credential")
	}
	src.Nodes = append(src.Nodes, src.Nodes[0])
	if _, e = domain.SourceNodes(src); e == nil {
		t.Fatal("ambiguous identity silently reassigned")
	}
}
