package server

import (
	"net/http"

	"github.com/fjzhangZzzzzz/subweft/internal/app"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
)

func resourceMeta(v domain.Resource) domain.Resource {
	v.URL = ""
	v.Headers = nil
	if v.Kind == "http" {
		v.LastError = ""
		v.Count = 0
		v.Digest = ""
	}
	return v
}
func resourceMetas(values []domain.Resource) []domain.Resource {
	out := []domain.Resource{}
	for _, v := range values {
		out = append(out, resourceMeta(v))
	}
	return out
}
func (a *API) manageResources(w http.ResponseWriter, r *http.Request, id, action string) bool {
	ctx := r.Context()
	s := a.Service
	if id != "" && action == "edit" && r.Method == "GET" {
		v, e := storage.Load[domain.Resource](ctx, s.Store, "resource", id)
		if e != nil {
			fail(w, e)
		} else {
			respond(w, 200, v)
		}
		return true
	}
	if id != "" && action == "entries" && r.Method == "GET" {
		values, e := s.ResourceEntries(ctx, id)
		if e != nil {
			fail(w, e)
		} else {
			respond(w, 200, values)
		}
		return true
	}
	if id != "" && action == "refresh" && r.Method == "POST" {
		var in struct {
			Revision int `json:"revision"`
		}
		if !read(w, r, &in) {
			return true
		}
		old, e := storage.Load[domain.Resource](ctx, s.Store, "resource", id)
		if e != nil {
			fail(w, e)
			return true
		}
		if old.Revision != in.Revision {
			fail(w, domain.ErrConflict)
			return true
		}
		v, e := s.RefreshResource(ctx, id)
		if e != nil {
			fail(w, e)
		} else {
			respond(w, 200, resourceMeta(v))
		}
		return true
	}
	if action == "" && (r.Method == "POST" && id == "" || r.Method == "PUT" && id != "") {
		var in app.ResourceInput
		if !read(w, r, &in) {
			return true
		}
		if in.ID != id {
			fail(w, domain.Invalid("id_mismatch", "id"))
			return true
		}
		v, e := s.SaveResource(ctx, in)
		if e != nil {
			fail(w, e)
		} else {
			respond(w, 200, resourceMeta(v))
		}
		return true
	}
	return false
}
func (a *API) resourceDownload(w http.ResponseWriter, r *http.Request) {
	select {
	case a.resourceGate <- struct{}{}:
		defer func() { <-a.resourceGate }()
	case <-r.Context().Done():
		return
	}
	data, e := a.Service.Store.ResourceDownload(r.Context(), r.PathValue("token"), r.PathValue("release"), r.PathValue("name"))
	if e != nil {
		http.Error(w, "not_found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}
