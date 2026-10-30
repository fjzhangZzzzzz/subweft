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
		snap, e := s.Store.Snapshot(ctx)
		if e != nil {
			return
		}
		activeID := snap.Active[draft.ID]
		if activeID == "" {
			continue
		}
		active, e := storage.Load[domain.Release](ctx, s.Store, "release", activeID)
		if e != nil || active.Definition == nil || !active.Definition.AutoPublish {
			continue
		}
		p := *active.Definition
		result, e := compile.Compile(p, snap, false)
		// Human library edits require an explicit Apply, even when automatic
		// content refresh is enabled. Older releases need one explicit Apply.
		if e == nil && (active.InputDigest == "" || result.InputDigest != active.InputDigest) {
			continue
		}
		if e == nil && compile.Digest(result.Config) == active.Digest && sameResources(result.Resources, active.Resources) {
			continue
		}
		if e == nil {
			e = s.checkResult(ctx, result)
		}
		if e == nil {
			r := domain.Release{InputDigest: result.InputDigest, ProfileID: p.ID, ProfileRevision: p.Revision, Definition: &p, Resources: result.Resources, Config: result.Config, Digest: compile.Digest(result.Config), Sources: result.Sources, Validator: s.Validator.Identity(), CreatedAt: time.Now().UTC()}
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

// PendingProfiles reports whether the saved inputs differ from each active
// release. It uses cached data only, without upstream fetches or subprocesses.
func PendingProfiles(snap domain.Snapshot) map[string]bool {
	active := map[string]domain.Release{}
	for _, release := range snap.Releases {
		if snap.Active[release.ProfileID] == release.ID {
			active[release.ProfileID] = release
		}
	}
	pending := map[string]bool{}
	for _, profile := range snap.Profiles {
		release, ok := active[profile.ID]
		if !ok || release.ProfileRevision != profile.Revision {
			pending[profile.ID] = true
			continue
		}
		result, err := compile.Compile(profile, snap, true)
		pending[profile.ID] = err != nil || result.InputDigest != release.InputDigest || compile.Digest(result.Config) != release.Digest || !sameResources(result.Resources, release.Resources)
	}
	return pending
}
