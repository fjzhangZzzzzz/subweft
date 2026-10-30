package app_test

import (
	"context"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"testing"
)

func TestPoolMembersCannotBeDeletedOrSavedAsMissing(t *testing.T) {
	s, _, _, _ := setup(t)
	src, p := create(t, s)
	ctx := context.Background()
	p.Groups[0].SourceIDs = nil
	p.Groups[0].Pools = []string{"常用"}
	p.Pools = []domain.SourcePool{{Name: "常用", SourceIDs: []string{src.ID}}}
	saved, err := s.SaveProfile(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(ctx, "source", src.ID, src.Revision); err == nil {
		t.Fatal("referenced pool member deleted")
	}
	preview, _, _, err := s.Preview(ctx, saved.ID, false)
	if err != nil || len(preview.Members["Proxy"]) != 2 {
		t.Fatal("saved pool did not resolve members", err)
	}
	saved.Pools[0].SourceIDs = []string{"missing"}
	if _, err = s.SaveProfile(ctx, saved); err == nil {
		t.Fatal("missing pool member saved")
	}
	// Rejected edits preserve the last stored definition.
	if _, _, _, err = s.Preview(ctx, p.ID, false); err != nil {
		t.Fatal("rejected pool edit damaged saved profile", err)
	}
}
