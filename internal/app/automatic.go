package app

import (
	"context"
	"errors"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
)

type PublishStatus struct {
	domain.Meta
	Attempt time.Time `json:"attempt"`
	Error   string    `json:"error,omitempty"`
}

// AutoPublish refreshes the last published definition, never a newly saved draft.
func (s *Service) AutoPublish(ctx context.Context) {
	profiles, e := storage.List[domain.Profile](ctx, s.Store, "profile")
	if e != nil {
		return
	}
	for _, draft := range profiles {
		if !draft.AutoPublish {
			continue
		}
		snap, e := s.Store.Snapshot(ctx)
		if e != nil {
			return
		}
		activeID := snap.Active[draft.ID]
		if activeID == "" {
			continue
		}
		active, e := storage.Load[domain.Release](ctx, s.Store, "release", activeID)
		if e != nil || active.Definition == nil {
			continue
		}
		p := *active.Definition
		result, e := compile.Compile(p, snap, false)
		if e == nil && compile.Digest(result.Config) == active.Digest && sameResources(result.Resources, active.Resources) {
			continue
		}
		if e == nil {
			e = s.checkResult(ctx, result)
		}
		if e == nil {
			r := domain.Release{ProfileID: p.ID, ProfileRevision: p.Revision, Definition: &p, Resources: result.Resources, Config: result.Config, Digest: compile.Digest(result.Config), Sources: result.Sources, Validator: s.Validator.Identity(), CreatedAt: time.Now().UTC()}
			if e = s.persistAssets(ctx, result); e == nil {
				e = s.Store.Publish(ctx, snap.Generation, &r)
			}
		}
		status, loadErr := storage.Load[PublishStatus](ctx, s.Store, "publish-status", p.ID)
		if loadErr != nil {
			status = PublishStatus{Meta: domain.Meta{ID: p.ID}}
		}
		status.Attempt = time.Now().UTC()
		status.Error = ""
		if e != nil {
			status.Error = "automatic_publish_failed"
			var de *domain.Error
			if errors.As(e, &de) {
				status.Error = de.Code
			}
		}
		_ = storage.Save(ctx, s.Store, "publish-status", &status.Meta, &status, "")
		if ctx.Err() != nil {
			return
		}
	}
}
func sameResources(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
