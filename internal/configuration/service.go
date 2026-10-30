package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"github.com/fjzhangZzzzzz/subweft/internal/validator"
)

type Release struct {
	Number  int       `json:"number"`
	Created time.Time `json:"created"`
	Note    string    `json:"note"`
	Config  Config    `json:"config"`
	YAML    string    `json:"-"`
}

// StoredRelease uses an explicit field for encrypted persistence, never an API response.
type StoredRelease struct {
	Release
	Content string `json:"content"`
}
type Document struct {
	domain.Meta
	Draft    Draft           `json:"draft"`
	Active   int             `json:"active"`
	Releases []StoredRelease `json:"releases"`
}
type Check struct {
	Revision int       `json:"revision"`
	Problems []Problem `json:"problems"`
	Identity string    `json:"identity"`
	YAML     string    `json:"-"`
}
type Device struct {
	domain.Meta
	Name      string     `json:"name"`
	State     string     `json:"state"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Token     string     `json:"-"`
}
type storedDevice struct {
	Device
	Secret string `json:"secret"`
}
type Service struct {
	Store    *storage.Store
	Checker  validator.Checker
	mu       sync.Mutex
	checked  *Check
	checking bool
}

func New(store *storage.Store, checker validator.Checker) *Service {
	return &Service{Store: store, Checker: checker}
}
func (s *Service) document(ctx context.Context) (Document, error) {
	d, e := storage.Load[Document](ctx, s.Store, "configuration", "single")
	if errors.Is(e, domain.ErrNotFound) {
		d = Document{Meta: domain.Meta{ID: "single"}, Draft: Draft{Revision: 1, Config: Default()}, Releases: []StoredRelease{}}
		e = storage.Save(ctx, s.Store, "configuration", &d.Meta, &d, "")
	}
	return d, e
}
func (s *Service) Read(ctx context.Context) (Document, *Check, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.document(ctx)
	var checked *Check
	if s.checked != nil && s.checked.Revision == d.Draft.Revision {
		x := *s.checked
		checked = &x
	}
	return d, checked, e
}
func same(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func (s *Service) Save(ctx context.Context, in Draft) (Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.document(ctx)
	if e != nil {
		return Draft{}, e
	}
	if in.Revision != d.Draft.Revision {
		return Draft{}, domain.ErrConflict
	}
	for i := range in.Config.Subscriptions {
		if in.Config.Subscriptions[i].URL == "[地址已隐藏]" {
			for _, old := range d.Draft.Config.Subscriptions {
				if old.ID == in.Config.Subscriptions[i].ID {
					in.Config.Subscriptions[i].URL = old.URL
				}
			}
		}
	}
	for i := range in.Config.RuleSets {
		if in.Config.RuleSets[i].URL == "[地址已隐藏]" {
			for _, old := range d.Draft.Config.RuleSets {
				if old.ID == in.Config.RuleSets[i].ID {
					in.Config.RuleSets[i].URL = old.URL
				}
			}
		}
	}
	if !same(in.Config.Base, d.Draft.Config.Base) {
		return Draft{}, domain.Invalid("base_readonly", "base")
	}
	if e = ValidateFields(in.Config); e != nil {
		return Draft{}, e
	}
	if same(in.Config, d.Draft.Config) {
		return d.Draft, nil
	}
	d.Draft = Draft{Revision: d.Draft.Revision + 1, Config: in.Config}
	e = storage.Save(ctx, s.Store, "configuration", &d.Meta, &d, "")
	if e == nil {
		s.checked = nil
	}
	return d.Draft, e
}
func (s *Service) Check(ctx context.Context, revision int) (Check, error) {
	s.mu.Lock()
	if s.checking {
		s.mu.Unlock()
		return Check{}, domain.Invalid("check_busy", "check")
	}
	d, e := s.document(ctx)
	if e != nil {
		s.mu.Unlock()
		return Check{}, e
	}
	if revision != d.Draft.Revision {
		s.mu.Unlock()
		return Check{}, domain.ErrConflict
	}
	s.checking = true
	s.checked = nil
	s.mu.Unlock()
	result := Check{Revision: revision, Problems: Problems(d.Draft.Config)}
	if len(result.Problems) == 0 {
		var data []byte
		data, e = Render(d.Draft.Config, false)
		if e == nil {
			if s.Checker == nil {
				e = domain.Invalid("validator_unavailable", "check")
			} else {
				e = s.Checker.Check(ctx, data)
				result.Identity = s.Checker.Identity()
			}
		}
		if e == nil {
			result.YAML = string(data)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checking = false
	if e != nil {
		return Check{}, e
	}
	current, e := s.document(ctx)
	if e != nil {
		return Check{}, e
	}
	if current.Draft.Revision != revision {
		return Check{}, domain.ErrConflict
	}
	s.checked = &result
	return result, nil
}
func (s *Service) Publish(ctx context.Context, revision int, note string) (Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.document(ctx)
	if e != nil {
		return Release{}, e
	}
	if revision != d.Draft.Revision {
		return Release{}, domain.ErrConflict
	}
	if s.checked == nil || s.checked.Revision != revision || len(s.checked.Problems) > 0 || s.checked.YAML == "" || s.Checker == nil || s.checked.Identity != s.Checker.Identity() {
		return Release{}, domain.Invalid("check_required", "publish")
	}
	if len(note) > 480 || strings.ContainsAny(note, "\r\n") {
		return Release{}, domain.Invalid("invalid_note", "note")
	}
	for _, r := range d.Releases {
		if r.Number == d.Active && same(r.Config, d.Draft.Config) {
			return Release{}, domain.Invalid("no_changes", "publish")
		}
	}
	number := d.Active + 1
	for _, r := range d.Releases {
		if r.Number >= number {
			number = r.Number + 1
		}
	}
	r := Release{Number: number, Created: time.Now().UTC(), Note: note, Config: Clone(d.Draft.Config)}
	d.Releases = append([]StoredRelease{{Release: r, Content: s.checked.YAML}}, d.Releases...)
	if len(d.Releases) > 20 {
		d.Releases = d.Releases[:20]
	}
	d.Active = number
	e = storage.Save(ctx, s.Store, "configuration", &d.Meta, &d, "")
	if e == nil {
		s.checked = nil
	}
	return r, e
}
func (s *Service) Restore(ctx context.Context, revision, number int) (Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, e := s.document(ctx)
	if e != nil {
		return Draft{}, e
	}
	if revision != d.Draft.Revision {
		return Draft{}, domain.ErrConflict
	}
	for _, r := range d.Releases {
		if r.Number == number {
			d.Draft = Draft{Revision: revision + 1, Config: Clone(r.Config)}
			e = storage.Save(ctx, s.Store, "configuration", &d.Meta, &d, "")
			if e == nil {
				s.checked = nil
			}
			return d.Draft, e
		}
	}
	return Draft{}, domain.ErrNotFound
}
func (s *Service) Devices(ctx context.Context) ([]Device, error) {
	values, e := storage.List[storedDevice](ctx, s.Store, "configuration-device")
	out := []Device{}
	for _, v := range values {
		out = append(out, v.Device)
	}
	return out, e
}
func (s *Service) SaveDevice(ctx context.Context, in Device, action string) (Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validName(in.Name) {
		return Device{}, domain.Invalid("invalid_name", "name")
	}
	all, e := s.Devices(ctx)
	if e != nil {
		return Device{}, e
	}
	if len(all) >= 100 && in.ID == "" {
		return Device{}, domain.Invalid("device_limit", "devices")
	}
	for _, d := range all {
		if d.ID != in.ID && d.Name == in.Name {
			return Device{}, domain.Invalid("duplicate_name", "name")
		}
	}
	var v storedDevice
	if in.ID != "" {
		v, e = storage.Load[storedDevice](ctx, s.Store, "configuration-device", in.ID)
		if e != nil {
			return Device{}, e
		}
		if v.Revision != in.Revision {
			return Device{}, domain.ErrConflict
		}
		if v.State == "revoked" {
			return Device{}, domain.Invalid("device_revoked", "device")
		}
	} else {
		if in.Revision != 0 || action != "save" {
			return Device{}, domain.Invalid("invalid_device", "device")
		}
		v = storedDevice{Device: Device{State: "active"}, Secret: domain.RandomToken()}
	}
	switch action {
	case "save":
		v.Name = in.Name
		v.ExpiresAt = in.ExpiresAt
	case "pause":
		v.State = "paused"
	case "resume":
		v.State = "active"
	case "revoke":
		v.State = "revoked"
	case "rotate":
		v.Secret = domain.RandomToken()
	default:
		return Device{}, domain.Invalid("invalid_action", "device")
	}
	e = storage.Save(ctx, s.Store, "configuration-device", &v.Meta, &v, s.Store.TokenHash(v.Secret))
	return v.Device, e
}
func (s *Service) DeviceToken(ctx context.Context, id string) (string, error) {
	v, e := storage.Load[storedDevice](ctx, s.Store, "configuration-device", id)
	if e != nil {
		return "", e
	}
	if v.State != "active" || v.ExpiresAt != nil && !time.Now().Before(*v.ExpiresAt) {
		return "", domain.Invalid("device_unavailable", "device")
	}
	return v.Secret, nil
}
func (s *Service) Download(ctx context.Context, token string) ([]byte, error) {
	if len(token) != 43 {
		return nil, domain.ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	devices, e := storage.List[storedDevice](ctx, s.Store, "configuration-device")
	if e != nil {
		return nil, e
	}
	hash := s.Store.TokenHash(token)
	for _, v := range devices {
		if s.Store.TokenHash(v.Secret) != hash {
			continue
		}
		if v.State != "active" || v.ExpiresAt != nil && !time.Now().Before(*v.ExpiresAt) {
			return nil, domain.ErrNotFound
		}
		d, e := s.document(ctx)
		if e != nil {
			return nil, e
		}
		i := slices.IndexFunc(d.Releases, func(r StoredRelease) bool { return r.Number == d.Active })
		if i < 0 {
			return nil, domain.ErrNotFound
		}
		s.Store.Seen(ctx, v.ID)
		return []byte(d.Releases[i].Content), nil
	}
	return nil, domain.ErrNotFound
}
