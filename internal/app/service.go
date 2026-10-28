package app

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"github.com/fjzhangZzzzzz/subweft/internal/validator"
)

type Service struct {
	Store     *storage.Store
	Fetcher   *source.Fetcher
	Validator validator.Checker
	mu        sync.Mutex
	fetchSem  chan struct{}
}

func New(store *storage.Store, fetcher *source.Fetcher, checker validator.Checker) *Service {
	return &Service{Store: store, Fetcher: fetcher, Validator: checker, fetchSem: make(chan struct{}, 1)}
}

type SourceInput struct {
	domain.Meta
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	URL      string   `json:"url"`
	Content  string   `json:"content"`
	Enabled  bool     `json:"enabled"`
	Tags     []string `json:"tags"`
	Interval int      `json:"interval"`
}

func (s *Service) SaveSource(ctx context.Context, in SourceInput) (domain.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := domain.Source{Meta: in.Meta, Name: in.Name, Kind: in.Kind, URL: in.URL, Content: in.Content, Enabled: in.Enabled, Tags: in.Tags, Interval: in.Interval}
	if in.ID != "" {
		old, e := storage.Load[domain.Source](ctx, s.Store, "source", in.ID)
		if e != nil {
			return v, e
		}
		if old.Revision != in.Revision {
			return v, domain.ErrConflict
		}
		v.Nodes = old.Nodes
		v.LastSuccess = old.LastSuccess
		v.LastAttempt = old.LastAttempt
		v.LastError = old.LastError
		if v.URL == "" && v.Kind == old.Kind {
			v.URL = old.URL
		}
		if v.Content == "" && v.Kind == old.Kind {
			v.Content = old.Content
		}
		if v.Kind != old.Kind || (v.Kind == "http" && v.URL != old.URL) {
			v.Nodes = nil
			v.LastSuccess = time.Time{}
			v.LastAttempt = time.Time{}
			v.LastError = "source_changed_refresh_required"
		}
	}
	if e := domain.ValidateSource(v); e != nil {
		return v, e
	}
	if v.Kind == "http" {
		if e := source.ValidateURL(v.URL); e != nil {
			return v, e
		}
		v.Content = ""
	} else {
		nodes, e := source.Parse([]byte(v.Content))
		if e != nil {
			return v, e
		}
		v.Nodes = nodes
		v.LastSuccess = time.Now().UTC()
		v.LastAttempt = v.LastSuccess
		v.LastError = ""
		v.URL = ""
	}
	e := storage.Save(ctx, s.Store, "source", &v.Meta, &v, "")
	return v, e
}
func (s *Service) Refresh(ctx context.Context, id string) (domain.Source, error) {
	select {
	case s.fetchSem <- struct{}{}:
		defer func() { <-s.fetchSem }()
	case <-ctx.Done():
		return domain.Source{}, ctx.Err()
	}
	v, e := storage.Load[domain.Source](ctx, s.Store, "source", id)
	if e != nil {
		return v, e
	}
	if !v.Enabled {
		return v, domain.Invalid("source_disabled", "source")
	}
	if v.Kind != "http" {
		return v, domain.Invalid("not_http_source", "source")
	}
	b, e := s.Fetcher.Fetch(ctx, v.URL)
	if e == nil {
		v.Nodes, e = source.Parse(b)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, loadErr := storage.Load[domain.Source](ctx, s.Store, "source", id)
	if loadErr != nil {
		return v, loadErr
	}
	if current.Revision != v.Revision {
		return v, domain.ErrConflict
	}
	v.LastAttempt = time.Now().UTC()
	if e != nil {
		v.Nodes = current.Nodes
		v.LastError = "refresh_failed"
		var de *domain.Error
		if errors.As(e, &de) {
			v.LastError = de.Code
		}
	} else {
		v.LastSuccess = v.LastAttempt
		v.LastError = ""
	}
	if saveErr := s.Store.SaveSourceSnapshot(ctx, v); saveErr != nil {
		return v, saveErr
	}
	return v, e
}
func (s *Service) SaveNode(ctx context.Context, v domain.ManualNode) (domain.ManualNode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.ID != "" {
		old, e := storage.Load[domain.ManualNode](ctx, s.Store, "node", v.ID)
		if e != nil {
			return v, e
		}
		if old.Revision != v.Revision {
			return v, domain.ErrConflict
		}
		if v.Node.Password == "" {
			v.Node.Password = old.Node.Password
		}
	}
	if e := domain.ValidateNode(v.Node); e != nil {
		return v, e
	}
	e := storage.Save(ctx, s.Store, "node", &v.Meta, &v, "")
	return v, e
}
func (s *Service) CopyNode(ctx context.Context, sourceID, nodeName, newName string) (domain.ManualNode, error) {
	v, e := storage.Load[domain.Source](ctx, s.Store, "source", sourceID)
	if e != nil {
		return domain.ManualNode{}, e
	}
	for _, n := range v.Nodes {
		if n.Name == nodeName {
			n.Name = newName
			return s.SaveNode(ctx, domain.ManualNode{Node: n, Enabled: true})
		}
	}
	return domain.ManualNode{}, domain.ErrNotFound
}
func (s *Service) SaveProfile(ctx context.Context, p domain.Profile) (domain.Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := domain.ValidateProfile(p); e != nil {
		return p, e
	}
	if p.ID != "" {
		old, e := storage.Load[domain.Profile](ctx, s.Store, "profile", p.ID)
		if e != nil {
			return p, e
		}
		if old.Revision != p.Revision {
			return p, domain.ErrConflict
		}
	}
	e := storage.Save(ctx, s.Store, "profile", &p.Meta, &p, "")
	return p, e
}
func (s *Service) Delete(ctx context.Context, kind, id string, revision int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, e := s.Store.Snapshot(ctx)
	if e != nil {
		return e
	}
	if kind == "source" || kind == "node" {
		for _, p := range snap.Profiles {
			for _, g := range p.Groups {
				refs := g.SourceIDs
				if kind == "node" {
					refs = g.NodeIDs
				}
				for _, ref := range refs {
					if ref == id {
						return domain.Invalid("resource_in_use", "id")
					}
				}
			}
		}
	}
	if kind == "profile" {
		if snap.Active[id] != "" {
			return domain.Invalid("published_profile_cannot_delete", "id")
		}
		for _, d := range snap.Devices {
			if d.ProfileID == id {
				return domain.Invalid("resource_in_use", "id")
			}
		}
	}
	return s.Store.Delete(ctx, kind, id, revision)
}
func (s *Service) Preview(ctx context.Context, id string, allowStale bool) (compile.Result, domain.Profile, int64, error) {
	snap, e := s.Store.Snapshot(ctx)
	if e != nil {
		return compile.Result{}, domain.Profile{}, 0, e
	}
	for _, p := range snap.Profiles {
		if p.ID == id {
			result, e := compile.Compile(p, snap, allowStale)
			return result, p, snap.Generation, e
		}
	}
	return compile.Result{}, domain.Profile{}, 0, domain.ErrNotFound
}
func (s *Service) Publish(ctx context.Context, id string, revision int, allowStale bool) (domain.Release, error) {
	result, p, gen, e := s.Preview(ctx, id, allowStale)
	if e != nil {
		return domain.Release{}, e
	}
	if p.Revision != revision {
		return domain.Release{}, domain.ErrConflict
	}
	if e = s.Validator.Check(ctx, result.Config); e != nil {
		return domain.Release{}, e
	}
	r := domain.Release{ProfileID: p.ID, ProfileRevision: p.Revision, CreatedAt: time.Now().UTC(), Config: result.Config, Digest: compile.Digest(result.Config), Sources: result.Sources, Validator: s.Validator.Identity()}
	e = s.Store.Publish(ctx, gen, &r)
	return r, e
}
func (s *Service) SaveDevice(ctx context.Context, d domain.Device, owner string) (domain.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !domain.ValidName(d.Name) {
		return d, domain.Invalid("invalid_name", "name")
	}
	if d.State != "active" && d.State != "paused" && d.State != "revoked" {
		return d, domain.Invalid("invalid_device_state", "state")
	}
	snap, e := s.Store.Snapshot(ctx)
	if e != nil {
		return d, e
	}
	if snap.Active[d.ProfileID] == "" {
		return d, domain.Invalid("profile_not_published", "profileId")
	}
	if d.ID != "" {
		old, e := storage.Load[domain.Device](ctx, s.Store, "device", d.ID)
		if e != nil {
			return d, e
		}
		if old.Revision != d.Revision {
			return d, domain.ErrConflict
		}
		if old.State == "revoked" && d.State != "revoked" {
			return d, domain.Invalid("device_revoked", "state")
		}
		d.Token = old.Token
		d.Owner = old.Owner
	} else {
		d.Token = domain.RandomToken()
		d.Owner = owner
	}
	e = storage.Save(ctx, s.Store, "device", &d.Meta, &d, s.Store.TokenHash(d.Token))
	return d, e
}
func (s *Service) Rotate(ctx context.Context, id string, revision int) (domain.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := storage.Load[domain.Device](ctx, s.Store, "device", id)
	if e != nil {
		return d, e
	}
	if d.Revision != revision {
		return d, domain.ErrConflict
	}
	if d.State == "revoked" {
		return d, domain.Invalid("device_revoked", "state")
	}
	d.Token = domain.RandomToken()
	e = storage.Save(ctx, s.Store, "device", &d.Meta, &d, s.Store.TokenHash(d.Token))
	return d, e
}
func (s *Service) Schedule(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			values, e := storage.List[domain.Source](ctx, s.Store, "source")
			if e != nil {
				continue
			}
			for _, v := range values {
				if !v.Enabled || v.Kind != "http" || time.Since(v.LastAttempt) < time.Duration(v.Interval)*time.Second {
					continue
				}
				_, _ = s.Refresh(ctx, v.ID)
				if ctx.Err() != nil {
					return
				}
			}
		}
	}
}
func CloneProfile(p domain.Profile) domain.Profile {
	b, _ := json.Marshal(p)
	var out domain.Profile
	_ = json.Unmarshal(b, &out)
	out.Meta = domain.Meta{}
	out.Name = p.Name + " copy"
	return out
}
