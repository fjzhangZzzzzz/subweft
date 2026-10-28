package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/fjzhangZzzzzz/subweft/internal/app"
	"github.com/fjzhangZzzzzz/subweft/internal/auth"
	"github.com/fjzhangZzzzzz/subweft/internal/buildinfo"
	"github.com/fjzhangZzzzzz/subweft/internal/compile"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/fields"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
)

type API struct {
	resourceGate chan struct{}
	Service      *app.Service
	Auth         *auth.Auth
	BaseURL      string
	UI           http.Handler
}

func (a *API) Handler() http.Handler {
	a.resourceGate = make(chan struct{}, 2)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if a.Service.Store.Ping(r.Context()) != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		respond(w, 200, map[string]any{"status": "ok", "build": buildinfo.Current()})
	})
	mux.HandleFunc("GET /auth/login", a.Auth.Login)
	mux.HandleFunc("GET /auth/callback", a.Auth.Callback)
	mux.HandleFunc("GET /s/{token}/config.yaml", a.download)
	mux.HandleFunc("GET /s/{token}/releases/{release}/resources/{name}", a.resourceDownload)
	mux.Handle("/api/v1/", http.HandlerFunc(a.manage))
	mux.Handle("/", a.UI)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		defer func() {
			if recover() != nil {
				http.Error(w, "internal_error", 500)
			}
		}()
		mux.ServeHTTP(w, r)
	})
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, e error) {
	status := 500
	v := domain.Error{Code: "internal_error"}
	if errors.Is(e, domain.ErrConflict) {
		status = 409
		v.Code = "conflict"
	} else if errors.Is(e, domain.ErrNotFound) {
		status = 404
		v.Code = "not_found"
	} else {
		var de *domain.Error
		if errors.As(e, &de) {
			v = *de
			status = 422
		}
	}
	respond(w, status, map[string]any{"error": v})
}
func read(w http.ResponseWriter, r *http.Request, v any) bool {
	limit := int64(64 << 10)
	if strings.HasPrefix(r.URL.Path, "/api/v1/resources") {
		limit = 90 << 20
	} else if r.URL.Path == "/api/v1/imports/preview" {
		limit = 40 << 20
	} else if strings.HasPrefix(r.URL.Path, "/api/v1/profiles") {
		limit = 16 << 20
	} else if strings.HasPrefix(r.URL.Path, "/api/v1/sources") || strings.HasPrefix(r.URL.Path, "/api/v1/nodes") {
		limit = 6 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(v); e != nil {
		var de *domain.Error
		if errors.As(e, &de) {
			fail(w, de)
			return false
		}
		fail(w, domain.Invalid("invalid_json", "request"))
		return false
	}
	var extra any
	if e := decoder.Decode(&extra); e != io.EOF {
		fail(w, domain.Invalid("invalid_json", "request"))
		return false
	}
	return true
}

type sourceView struct {
	ClientProvider *domain.ClientProvider `json:"clientProvider,omitempty"`
	ExtractNodes   bool                   `json:"extractNodes"`
	Overrides      []domain.NodeOverride  `json:"overrides,omitempty"`
	domain.Meta
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Enabled     bool     `json:"enabled"`
	Tags        []string `json:"tags"`
	Interval    int      `json:"interval"`
	NodeCount   int      `json:"nodeCount"`
	LastSuccess any      `json:"lastSuccess"`
	LastAttempt any      `json:"lastAttempt"`
	LastError   string   `json:"lastError"`
}

func sourceMeta(v domain.Source) sourceView {
	return sourceView{v.ClientProvider, v.ExtractNodes, v.Overrides, v.Meta, v.Name, v.Kind, v.Enabled, v.Tags, v.Interval, len(v.Nodes), v.LastSuccess, v.LastAttempt, v.LastError}
}
func releaseMeta(v domain.Release) domain.Release    { v.Config = nil; v.Definition = nil; return v }
func deviceMeta(v domain.Device) domain.Device       { v.Token = ""; return v }
func nodeMeta(v domain.ManualNode) domain.ManualNode { v.Node = domain.NodeMetadata(v.Node); return v }
func (a *API) manage(w http.ResponseWriter, r *http.Request) {
	session, e := a.Auth.Session(r)
	if e != nil {
		respond(w, 401, map[string]any{"error": domain.Error{Code: "login_required"}})
		return
	}
	if r.Method != "GET" && !a.Auth.CheckMutation(r, session) {
		respond(w, 403, map[string]any{"error": domain.Error{Code: "csrf_rejected"}})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	parts := strings.Split(path, "/")
	kind := parts[0]
	id := ""
	action := ""
	if len(parts) > 1 {
		id = parts[1]
	}
	if len(parts) > 2 {
		action = parts[2]
	}
	if len(parts) > 3 {
		fail(w, domain.ErrNotFound)
		return
	}
	ctx := r.Context()
	s := a.Service
	if path == "fields" && r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(fields.Catalog)
		return
	}
	if path == "logout" && r.Method == "POST" {
		a.Auth.Logout(w, r, session)
		return
	}
	if session.Role != "admin" && path != "state" && kind != "devices" {
		respond(w, 403, map[string]any{"error": domain.Error{Code: "access_denied"}})
		return
	}
	if kind == "devices" && session.Role != "admin" && id != "" {
		d, e := storage.Load[domain.Device](ctx, s.Store, "device", id)
		if e != nil || d.Owner != session.Issuer+"#"+session.Subject || !allows(session, d.ProfileID) {
			respond(w, 404, map[string]any{"error": domain.Error{Code: "not_found"}})
			return
		}
	}
	if path == "backup" && session.Role == "admin" && r.Method == "POST" {
		var in struct{}
		if !read(w, r, &in) {
			return
		}
		file, cleanup, e := s.Store.PrepareBackup(ctx)
		if e != nil {
			fail(w, domain.Invalid("backup_failed", "backup"))
			return
		}
		defer cleanup()
		w.Header().Set("Content-Type", "application/vnd.sqlite3")
		w.Header().Set("Content-Disposition", `attachment; filename="subweft-encrypted.sqlite"`)
		http.ServeFile(w, r, file)
		return
	}
	if path == "state" && r.Method == "GET" {
		snap, e := s.Store.Snapshot(ctx)
		if e != nil {
			fail(w, e)
			return
		}
		if session.Role != "admin" {
			a.userState(w, r, session, snap)
			return
		}
		users, e := storage.List[domain.User](ctx, s.Store, "user")
		if e != nil {
			fail(w, e)
			return
		}
		users = append(users, a.Auth.BootstrapUsers()...)
		publishStatus, _ := storage.List[app.PublishStatus](ctx, s.Store, "publish-status")
		sources := []sourceView{}
		for _, v := range snap.Sources {
			sources = append(sources, sourceMeta(v))
		}
		nodes := []domain.ManualNode{}
		for _, v := range snap.Nodes {
			nodes = append(nodes, nodeMeta(v))
		}
		devices := []domain.Device{}
		for _, v := range snap.Devices {
			devices = append(devices, deviceMeta(v))
		}
		releases := []domain.Release{}
		for _, v := range snap.Releases {
			releases = append(releases, releaseMeta(v))
		}
		seen, e := s.Store.SeenAll(ctx)
		if e != nil {
			fail(w, e)
			return
		}
		respond(w, 200, map[string]any{"build": buildinfo.Current(), "user": session.Name, "role": session.Role, "issuer": a.Auth.Issuer(), "users": users, "publishStatus": publishStatus, "csrf": session.CSRF, "sources": sources, "nodes": nodes, "profiles": snap.Profiles, "resources": resourceMetas(snap.Resources), "devices": devices, "releases": releases, "active": snap.Active, "seen": seen, "validator": s.Validator.Identity(), "capabilities": map[string]any{"protocols": domain.Protocols(), "groups": []string{"select", "url-test", "fallback", "load-balance"}, "automaticPublish": true}})
		return
	}
	switch kind {
	case "imports":
		if a.manageImport(w, r, id, action) {
			return
		}
	case "users":
		if a.manageUsers(w, r, id, action) {
			return
		}
	case "resources":
		if a.manageResources(w, r, id, action) {
			return
		}
	case "sources":
		if id != "" && action == "edit" && r.Method == "GET" {
			v, e := storage.Load[domain.Source](ctx, s.Store, "source", id)
			if e != nil {
				fail(w, e)
			} else {
				respond(w, 200, app.SourceInput{Meta: v.Meta, Name: v.Name, Kind: v.Kind, Enabled: v.Enabled, Tags: v.Tags, Interval: v.Interval, ExtractNodes: v.ExtractNodes, Fetch: v.Fetch, ClientProvider: v.ClientProvider, URL: v.URL})
			}
			return
		}
		if id != "" && action == "overrides" && r.Method == "POST" {
			var in struct {
				Revision int                 `json:"revision"`
				Override domain.NodeOverride `json:"override"`
				Remove   bool                `json:"remove"`
			}
			if !read(w, r, &in) {
				return
			}
			v, e := s.SaveOverride(ctx, id, in.Revision, in.Override, in.Remove)
			if e != nil {
				fail(w, e)
			} else {
				respond(w, 200, sourceMeta(v))
			}
			return
		}
		if id != "" && action == "refresh" && r.Method == "POST" {
			var in struct {
				Revision int `json:"revision"`
			}
			if !read(w, r, &in) {
				return
			}
			old, e := storage.Load[domain.Source](ctx, s.Store, "source", id)
			if e != nil {
				fail(w, e)
				return
			}
			if old.Revision != in.Revision {
				fail(w, domain.ErrConflict)
				return
			}
			v, e := s.Refresh(ctx, id)
			if e != nil {
				fail(w, e)
				return
			}
			respond(w, 200, sourceMeta(v))
			return
		}
		if id != "" && action == "nodes" && r.Method == "GET" {
			v, e := storage.Load[domain.Source](ctx, s.Store, "source", id)
			if e != nil {
				fail(w, e)
				return
			}
			counts := map[string]int{}
			overrides := map[string]domain.NodeOverride{}
			for _, n := range v.Nodes {
				counts[domain.NodeIdentity(n)]++
			}
			for _, o := range v.Overrides {
				overrides[o.Identity] = o
			}
			values := []map[string]any{}
			for _, n := range v.Nodes {
				values = append(values, map[string]any{"name": n.Name, "type": n.Type, "server": n.Server, "port": n.Port, "identity": domain.NodeIdentity(n), "ambiguous": counts[domain.NodeIdentity(n)] > 1, "override": overrides[domain.NodeIdentity(n)]})
			}
			respond(w, 200, values)
			return
		}
		if id != "" && action == "copy-node" && r.Method == "POST" {
			var in struct {
				Name    string `json:"name"`
				NewName string `json:"newName"`
			}
			if !read(w, r, &in) {
				return
			}
			v, e := s.CopyNode(ctx, id, in.Name, in.NewName)
			if e != nil {
				fail(w, e)
				return
			}
			respond(w, 201, nodeMeta(v))
			return
		}
		if action == "" && ((r.Method == "POST" && id == "") || (r.Method == "PUT" && id != "")) {
			var in app.SourceInput
			if !read(w, r, &in) {
				return
			}
			if in.ID != id {
				fail(w, domain.Invalid("id_mismatch", "id"))
				return
			}
			v, e := s.SaveSource(ctx, in)
			if e != nil {
				fail(w, e)
				return
			}
			respond(w, 200, sourceMeta(v))
			return
		}
	case "nodes":
		if id != "" && action == "edit" && r.Method == "GET" {
			value, e := storage.Load[domain.ManualNode](ctx, s.Store, "node", id)
			if e != nil {
				fail(w, e)
				return
			}
			respond(w, 200, value)
			return
		}
		if action == "" && ((r.Method == "POST" && id == "") || (r.Method == "PUT" && id != "")) {
			var in domain.ManualNode
			if !read(w, r, &in) {
				return
			}
			if in.ID != id {
				fail(w, domain.Invalid("id_mismatch", "id"))
				return
			}
			v, e := s.SaveNode(ctx, in)
			if e != nil {
				fail(w, e)
				return
			}
			respond(w, 200, nodeMeta(v))
			return
		}
	case "profiles":
		if action == "" && ((r.Method == "POST" && id == "") || (r.Method == "PUT" && id != "")) {
			var in domain.Profile
			if !read(w, r, &in) {
				return
			}
			if in.ID != id {
				fail(w, domain.Invalid("id_mismatch", "id"))
				return
			}
			v, e := s.SaveProfile(ctx, in)
			if e != nil {
				fail(w, e)
				return
			}
			respond(w, 200, v)
			return
		}
		if id != "" && action == "copy" && r.Method == "POST" {
			var in struct {
				Revision int `json:"revision"`
			}
			if !read(w, r, &in) {
				return
			}
			v, e := storage.Load[domain.Profile](ctx, s.Store, "profile", id)
			if e != nil {
				fail(w, e)
				return
			}
			if v.Revision != in.Revision {
				fail(w, domain.ErrConflict)
				return
			}
			v, e = s.SaveProfile(ctx, app.CloneProfile(v))
			if e != nil {
				fail(w, e)
				return
			}
			respond(w, 201, v)
			return
		}
		if id != "" && action == "preview" && r.Method == "POST" {
			var in struct {
				Revision   int  `json:"revision"`
				AllowStale bool `json:"allowStale"`
			}
			if !read(w, r, &in) {
				return
			}
			result, p, _, e := s.Preview(ctx, id, in.AllowStale)
			if e != nil {
				fail(w, e)
				return
			}
			if p.Revision != in.Revision {
				fail(w, domain.ErrConflict)
				return
			}
			respond(w, 200, map[string]any{"config": string(compile.Redact(result.Config)), "members": result.Members, "stale": result.Stale})
			return
		}
		if id != "" && action == "publish" && r.Method == "POST" {
			var in struct {
				Revision   int  `json:"revision"`
				AllowStale bool `json:"allowStale"`
			}
			if !read(w, r, &in) {
				return
			}
			v, e := s.Publish(ctx, id, in.Revision, in.AllowStale)
			if e != nil {
				fail(w, e)
				return
			}
			respond(w, 201, releaseMeta(v))
			return
		}
		if id != "" && action == "rollback" && r.Method == "POST" {
			var in struct {
				Revision  int    `json:"revision"`
				ReleaseID string `json:"releaseId"`
			}
			if !read(w, r, &in) {
				return
			}
			if e := s.Store.Activate(ctx, id, in.ReleaseID, in.Revision); e != nil {
				fail(w, e)
				return
			}
			w.WriteHeader(204)
			return
		}
	case "devices":
		if id != "" && action == "link" && r.Method == "GET" {
			v, e := storage.Load[domain.Device](ctx, s.Store, "device", id)
			if e != nil {
				fail(w, e)
				return
			}
			if v.State == "revoked" {
				fail(w, domain.Invalid("device_revoked", "state"))
				return
			}
			respond(w, 200, map[string]string{"url": a.BaseURL + "/s/" + v.Token + "/config.yaml"})
			return
		}
		if id != "" && action == "rotate" && r.Method == "POST" {
			var in struct {
				Revision int `json:"revision"`
			}
			if !read(w, r, &in) {
				return
			}
			v, e := s.Rotate(ctx, id, in.Revision)
			if e != nil {
				fail(w, e)
				return
			}
			respond(w, 200, deviceMeta(v))
			return
		}
		if action == "" && ((r.Method == "POST" && id == "") || (r.Method == "PUT" && id != "")) {
			var in struct {
				domain.Meta
				Name      string           `json:"name"`
				ProfileID string           `json:"profileId"`
				State     string           `json:"state"`
				ExpiresAt *json.RawMessage `json:"expiresAt,omitempty"`
			}
			if !read(w, r, &in) {
				return
			}
			if in.ID != id {
				fail(w, domain.Invalid("id_mismatch", "id"))
				return
			}
			v := domain.Device{Meta: in.Meta, Name: in.Name, ProfileID: in.ProfileID, State: in.State}
			if in.ExpiresAt != nil {
				if e = json.Unmarshal(*in.ExpiresAt, &v.ExpiresAt); e != nil {
					fail(w, domain.Invalid("invalid_expiration", "expiresAt"))
					return
				}
			}
			if !allows(session, v.ProfileID) {
				respond(w, 403, map[string]any{"error": domain.Error{Code: "access_denied"}})
				return
			}
			v, e = s.SaveDevice(ctx, v, session.Issuer+"#"+session.Subject)
			if e != nil {
				fail(w, e)
				return
			}
			respond(w, 200, deviceMeta(v))
			return
		}
	}
	if r.Method == "DELETE" && id != "" && action == "" {
		kinds := map[string]string{"sources": "source", "resources": "resource", "nodes": "node", "profiles": "profile", "devices": "device"}
		if target, ok := kinds[kind]; ok {
			var in struct {
				Revision int `json:"revision"`
			}
			if !read(w, r, &in) {
				return
			}
			if e := s.Delete(ctx, target, id, in.Revision); e != nil {
				fail(w, e)
				return
			}
			w.WriteHeader(204)
			return
		}
	}
	fail(w, domain.ErrNotFound)
}
func (a *API) download(w http.ResponseWriter, r *http.Request) {
	d, release, e := a.Service.Store.Download(r.Context(), r.PathValue("token"))
	if e != nil || !a.Auth.DeviceAllowed(r.Context(), d) {
		http.Error(w, "not_found", 404)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="subweft.yaml"`)
	w.Header().Set("X-Subweft-Release", release.ID)
	body := release.Config
	if len(release.Resources) > 0 {
		body, e = compile.DeviceConfig(body, a.BaseURL+"/s/"+d.Token+"/releases/"+release.ID+"/resources/")
		if e != nil {
			http.Error(w, "unavailable", 503)
			return
		}
	}
	_, _ = w.Write(body)
	a.Service.Store.Seen(r.Context(), d.ID)
}
