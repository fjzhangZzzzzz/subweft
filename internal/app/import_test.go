package app_test

import (
	"context"
	"github.com/fjzhangZzzzzz/subweft/internal/app"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/migrate"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
	"strings"
	"testing"
)

func TestReviewedImportIsAtomicDraftOnlyAndCannotReplay(t *testing.T) {
	ctx := context.Background()
	s, _, _, _ := setup(t)
	input := migrate.Input{Name: "Imported", Content: testfixture.MigrationConfig}
	candidate, e := s.PreviewImport(ctx, input)
	if e != nil {
		t.Fatal(e)
	}
	before, _ := s.Store.Snapshot(ctx)
	if len(before.Profiles) != 0 || len(before.Sources) != 0 {
		t.Fatal("preview wrote business definitions")
	}
	p, e := s.CommitImport(ctx, candidate.ID, candidate.Revision)
	if e != nil {
		t.Fatal(e)
	}
	after, e := s.Store.Snapshot(ctx)
	if e != nil || len(after.Profiles) != 1 || len(after.Sources) != 2 || len(after.Resources) != 2 || len(after.Active) != 0 || p.Revision != 1 || p.GeoIPID != "" {
		t.Fatal("import was incomplete or activated a release")
	}
	if _, e = s.CommitImport(ctx, candidate.ID, candidate.Revision); e == nil {
		t.Fatal("import replayed")
	}
}

func TestImportPreservesAnEmptyInlineRuleSet(t *testing.T) {
	ctx := context.Background()
	s, _, _, _ := setup(t)
	config := strings.Replace(testfixture.MigrationConfig, "payload: ['+.example.invalid']", "payload: []", 1)
	if config == testfixture.MigrationConfig {
		t.Fatal("test fixture did not change")
	}
	candidate, err := s.PreviewImport(ctx, migrate.Input{Name: "Empty rules", Content: config})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CommitImport(ctx, candidate.ID, candidate.Revision); err != nil {
		t.Fatal(err)
	}
	snap, err := s.Store.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range snap.Resources {
		if resource.Name == "self" {
			entries, err := s.ResourceEntries(ctx, resource.ID)
			if err != nil || len(entries) != 0 || resource.Digest == "" || resource.Count != 0 {
				t.Fatal("empty ruleset was not retained")
			}
			return
		}
	}
	t.Fatal("empty ruleset missing")
}
func TestImportConcurrentChangeLeavesAllDraftsUnwritten(t *testing.T) {
	ctx := context.Background()
	s, _, _, _ := setup(t)
	candidate, e := s.PreviewImport(ctx, migrate.Input{Name: "Imported", Content: testfixture.MigrationConfig})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.SaveSource(ctx, app.SourceInput{Name: "concurrent", Kind: "local", Content: testfixture.ProviderYAML, Enabled: true, Interval: 3600})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CommitImport(ctx, candidate.ID, candidate.Revision); e != domain.ErrConflict {
		t.Fatal("concurrent generation accepted")
	}
	snap, _ := s.Store.Snapshot(ctx)
	if len(snap.Profiles) != 0 || len(snap.Resources) != 0 || len(snap.Sources) != 1 {
		t.Fatal("failed import partially wrote business objects")
	}
}
