package app

import (
	"context"
	"encoding/hex"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
)

func (s *Service) SaveOverride(ctx context.Context, id string, revision int, value domain.NodeOverride, remove bool) (domain.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := storage.Load[domain.Source](ctx, s.Store, "source", id)
	if e != nil {
		return v, e
	}
	if v.Revision != revision {
		return v, domain.ErrConflict
	}
	raw, e := hex.DecodeString(value.Identity)
	if e != nil || len(raw) != 32 {
		return v, domain.Invalid("invalid_identity", "identity")
	}
	if value.Name != "" && !domain.ValidName(value.Name) || len(value.Notes) > 4096 {
		return v, domain.Invalid("invalid_override", "name")
	}
	if !remove {
		count := 0
		for _, n := range v.Nodes {
			if domain.NodeIdentity(n) == value.Identity {
				count++
			}
		}
		if count != 1 {
			return v, domain.Invalid("ambiguous_or_missing_node", "identity")
		}
	}
	out := []domain.NodeOverride{}
	for _, old := range v.Overrides {
		if old.Identity != value.Identity {
			out = append(out, old)
		}
	}
	if !remove {
		out = append(out, value)
	}
	v.Overrides = out
	e = storage.Save(ctx, s.Store, "source", &v.Meta, &v, "")
	return v, e
}
