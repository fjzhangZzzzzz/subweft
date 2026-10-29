package app

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"github.com/fjzhangZzzzzz/subweft/internal/validator"
	"go.yaml.in/yaml/v3"
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
	Update             domain.DeviceUpdate    `json:"update"`
	ConvertCached      bool                   `json:"convertCached,omitempty"`
	ResolveLegacyProxy bool                   `json:"resolveLegacyProxy,omitempty"`
	ClientProvider     *domain.ClientProvider `json:"clientProvider,omitempty"`
	Fetch              domain.FetchOptions    `json:"fetch"`
	ExtractNodes       bool                   `json:"extractNodes"`
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
	if in.ClientProvider != nil || in.Fetch.Proxy != "" || in.Fetch.Timeout != 0 || in.Fetch.Retries != 0 || in.Fetch.UserAgent != "" || len(in.Fetch.Headers) > 0 {
		return domain.Source{}, domain.Invalid("retired_fetch_settings", "update")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v := domain.Source{Update: in.Update, ClientProvider: in.ClientProvider, Fetch: in.Fetch, ExtractNodes: in.ExtractNodes, Meta: in.Meta, Name: in.Name, Kind: in.Kind, URL: in.URL, Content: in.Content, Enabled: in.Enabled, Tags: in.Tags, Interval: in.Interval}
	if in.ID != "" {
		old, e := storage.Load[domain.Source](ctx, s.Store, "source", in.ID)
		if e != nil {
			return v, e
		}
		if old.Revision != in.Revision {
			return v, domain.ErrConflict
		}
		if in.ConvertCached {
			if old.Kind != "http" || len(old.Nodes) == 0 {
				return v, domain.Invalid("local_snapshot_unavailable", "source")
			}
			counts := map[string]int{}
			for _, n := range old.Nodes {
				counts[domain.NodeIdentity(n)]++
			}
			for _, o := range old.Overrides {
				if counts[o.Identity] != 1 {
					return v, domain.Invalid("ambiguous_or_missing_node", "overrides")
				}
			}
			nodes, e := domain.SourceNodes(old)
			if e != nil {
				return v, e
			}
			if len(nodes) == 0 {
				return v, domain.Invalid("empty_source", "source")
			}
			data, e := yaml.Marshal(map[string]any{"proxies": nodes})
			if e != nil {
				return v, e
			}
			v.Kind = "local"
			v.Content = string(data)
			v.URL = ""
			v.Nodes = nodes
			v.Overrides = nil
			v.Fetch = domain.FetchOptions{}
			v.ClientProvider = nil
			v.LegacyProxy = ""
			v.ExtractNodes = false
			if e = domain.ValidateSource(v); e != nil {
				return v, e
			}
			e = storage.Save(ctx, s.Store, "source", &v.Meta, &v, "")
			return v, e
		}
		v.Nodes = old.Nodes
		v.Overrides = old.Overrides
		v.LegacyProxy = old.LegacyProxy
		if in.ResolveLegacyProxy {
			v.LegacyProxy = ""
		}
		v.LastSuccess = old.LastSuccess
		v.LastAttempt = old.LastAttempt
		v.LastError = old.LastError
		if v.URL == "" && v.Kind == old.Kind {
			v.URL = old.URL
		}
		if v.Content == "" && v.Kind == old.Kind {
			v.Content = old.Content
		}
		if v.Kind != old.Kind || v.ExtractNodes != old.ExtractNodes || (v.Kind == "http" && v.URL != old.URL) {
			v.Nodes = nil
			v.LastSuccess = time.Time{}
			v.LastAttempt = time.Time{}
			v.LastError = "source_changed_refresh_required"
		}
	}
	if v.Kind == "http" {
		var e error
		v.URL, v.Update.Headers, e = source.NormalizeCredentials(v.URL, v.Update.Headers)
		if e != nil {
			return v, e
		}
	}
	if e := compile.ValidateDeviceUpdate(v.Update); e != nil {
		return v, e
	}
	if e := domain.ValidateSource(v); e != nil {
		return v, e
	}
	if v.Kind == "http" {
		if e := source.ValidateURL(v.URL); e != nil {
			return v, e
		}
		v.Content = ""
		v.LastError = ""
	} else {
		nodes, e := source.ParseInput([]byte(v.Content), v.ExtractNodes)
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
	return domain.Source{}, domain.Invalid("device_updates_only", "source")
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
		if v.Node.Type == old.Node.Type {
			if v.Node.Type != "tuic" && v.Node.Password == "" && (v.Node.Type != "http" && v.Node.Type != "socks5" || v.Node.Username != "" && v.Node.Username == old.Node.Username) {
				v.Node.Password = old.Node.Password
			}
			if (v.Node.Type == "vmess" || v.Node.Type == "vless") && v.Node.UUID == "" {
				v.Node.UUID = old.Node.UUID
			}
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
	domain.NormalizeProfile(&p)
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
	if kind == "node" {
		for _, n := range snap.Nodes {
			if n.ID == id {
				for _, p := range snap.Profiles {
					for _, routes := range []map[string]string{p.SourceUpdates, p.RuleUpdates} {
						for _, route := range routes {
							if route == n.Node.Name {
								return domain.Invalid("resource_in_use", "id")
							}
						}
					}
				}
			}
		}
	}
	if kind == "resource" {
		for _, p := range snap.Profiles {
			if p.GeoIPID == id || p.GeoSiteID == id || p.GeoASNID == id {
				return domain.Invalid("resource_in_use", "id")
			}
			for _, r := range p.Rules {
				if r.Type == "RULE-SET" && r.Value == id {
					return domain.Invalid("resource_in_use", "id")
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
	if e = s.checkResult(ctx, result); e != nil {
		return domain.Release{}, e
	}
	r := domain.Release{Resources: result.Resources, Definition: &p, ProfileID: p.ID, ProfileRevision: p.Revision, CreatedAt: time.Now().UTC(), Config: result.Config, Digest: compile.Digest(result.Config), Sources: result.Sources, Validator: s.Validator.Identity()}
	if e = s.persistAssets(ctx, result); e != nil {
		return r, e
	}
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
	lastPrune := time.Time{}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Since(lastPrune) > time.Hour {
				_ = s.Store.PruneAssets(ctx)
				lastPrune = time.Now()
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

func (s *Service) checkResult(ctx context.Context, result compile.Result) error {
	if len(result.Resources) == 0 {
		return s.Validator.Check(ctx, result.Config)
	}
	checker, ok := s.Validator.(interface {
		CheckBundle(context.Context, []byte, map[string][]byte) error
	})
	if !ok {
		return domain.Invalid("validator_bundle_unsupported", "publish")
	}
	assets := map[string][]byte{}
	total := 0
	for id, digest := range result.Resources {
		b, ok := result.Assets[id]
		var e error
		if !ok {
			b, e = s.Store.Asset(ctx, digest)
		}
		if e != nil {
			return e
		}
		total += len(b)
		if total > 128<<20 {
			return domain.Invalid("resources_too_large", "publish")
		}
		assets[id] = b
	}
	return checker.CheckBundle(ctx, result.Config, assets)
}

func (s *Service) persistAssets(ctx context.Context, result compile.Result) error {
	for _, body := range result.Assets {
		if _, e := s.Store.PutAsset(ctx, body); e != nil {
			return e
		}
	}
	return nil
}
