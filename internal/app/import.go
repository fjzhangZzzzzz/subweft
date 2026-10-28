package app

import (
	"context"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/migrate"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"time"
)

type ImportCandidate struct {
	domain.Meta
	Generation int64        `json:"generation"`
	Expires    time.Time    `json:"expires"`
	Plan       migrate.Plan `json:"plan"`
}

func (s *Service) PreviewImport(ctx context.Context, input migrate.Input) (ImportCandidate, error) {
	pending, e := storage.List[ImportCandidate](ctx, s.Store, "import-candidate")
	if e != nil {
		return ImportCandidate{}, e
	}
	active := 0
	for _, old := range pending {
		if time.Now().After(old.Expires) {
			_ = s.Store.Delete(ctx, "import-candidate", old.ID, old.Revision)
		} else {
			active++
		}
	}
	if active >= 5 {
		return ImportCandidate{}, domain.Invalid("too_many_import_previews", "import")
	}
	plan, e := migrate.Parse(input)
	if e != nil {
		return ImportCandidate{}, e
	}
	for _, v := range plan.Sources {
		if e = source.ValidateOptions(v.Fetch); e != nil {
			return ImportCandidate{}, e
		}
	}
	for i, draft := range plan.Resources {
		if e = validateResource(draft.Resource); e != nil {
			return ImportCandidate{}, e
		}
		if draft.Resource.Kind == "local" {
			data, count, e := normalizeRules(draft.Data, draft.Resource.Behavior, draft.Resource.Format)
			if e != nil {
				return ImportCandidate{}, e
			}
			plan.Resources[i].Data = data
			plan.Resources[i].Resource.Count = count
		}
	}
	snap, e := s.Store.Snapshot(ctx)
	if e != nil {
		return ImportCandidate{}, e
	}
	candidate := ImportCandidate{Generation: snap.Generation, Expires: time.Now().Add(30 * time.Minute).UTC(), Plan: plan}
	e = storage.Save(ctx, s.Store, "import-candidate", &candidate.Meta, &candidate, "")
	return candidate, e
}
func (s *Service) CommitImport(ctx context.Context, id string, revision int) (domain.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	candidate, e := storage.Load[ImportCandidate](ctx, s.Store, "import-candidate", id)
	if e != nil {
		return domain.Profile{}, e
	}
	if candidate.Revision != revision || !time.Now().Before(candidate.Expires) {
		return domain.Profile{}, domain.ErrConflict
	}
	plan := candidate.Plan
	objects := []storage.NewObject{}
	for _, v := range plan.Sources {
		v.Revision = 1
		objects = append(objects, storage.NewObject{Kind: "source", Meta: v.Meta, Value: v})
	}
	for _, v := range plan.Nodes {
		v.Revision = 1
		objects = append(objects, storage.NewObject{Kind: "node", Meta: v.Meta, Value: v})
	}
	for _, draft := range plan.Resources {
		v := draft.Resource
		v.Revision = 1
		if v.Kind == "local" {
			v.Digest, e = s.Store.PutAsset(ctx, draft.Data)
			if e != nil {
				return domain.Profile{}, e
			}
			v.LastSuccess = time.Now().UTC()
			v.LastAttempt = v.LastSuccess
		}
		objects = append(objects, storage.NewObject{Kind: "resource", Meta: v.Meta, Value: v})
	}
	p := plan.Profile
	p.Revision = 1
	objects = append(objects, storage.NewObject{Kind: "profile", Meta: p.Meta, Value: p})
	e = s.Store.ImportDraft(ctx, candidate.Generation, id, objects)
	return p, e
}
