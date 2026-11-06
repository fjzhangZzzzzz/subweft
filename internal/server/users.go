package server

import (
	"github.com/fjzhangZzzzzz/subweft/internal/buildinfo"
	"net/http"
	"strings"

	"github.com/fjzhangZzzzzz/subweft/internal/auth"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
)

func allows(s auth.Session, profile string) bool {
	if s.Role == "admin" {
		return true
	}
	for _, id := range s.ProfileIDs {
		if id == profile {
			return true
		}
	}
	return false
}
func (a *API) manageUsers(w http.ResponseWriter, r *http.Request, id, action string) bool {
	if action != "" || !(r.Method == "POST" && id == "" || r.Method == "PUT" && id != "") {
		return false
	}
	var v domain.User
	if !read(w, r, &v) {
		return true
	}
	if v.Issuer != a.Auth.Issuer() || v.Subject == "" || len(v.Subject) > 512 || strings.ContainsAny(v.Subject, "\r\n\x00") || !domain.ValidName(v.Name) || v.Role != "admin" && v.Role != "user" {
		fail(w, domain.Invalid("invalid_user", "user"))
		return true
	}
	if a.Auth.IsBootstrap(v.Issuer, v.Subject) || v.Bootstrap {
		fail(w, domain.Invalid("bootstrap_identity_immutable", "subject"))
		return true
	}
	expected := domain.UserID(v.Issuer, v.Subject)
	if id != "" && (v.ID != id || expected != id) {
		fail(w, domain.Invalid("identity_immutable", "subject"))
		return true
	}
	if id == "" {
		if v.ID != "" || v.Revision != 0 {
			fail(w, domain.Invalid("id_mismatch", "id"))
			return true
		}
		if _, e := storage.Load[domain.User](r.Context(), a.Service.Store, "user", expected); e == nil {
			fail(w, domain.ErrConflict)
			return true
		}
		v.ID = expected
	}
	for _, profile := range v.ProfileIDs {
		if _, e := storage.Load[domain.Profile](r.Context(), a.Service.Store, "profile", profile); e != nil {
			fail(w, domain.Invalid("profile_missing", "profileIds"))
			return true
		}
	}
	if e := storage.Save(r.Context(), a.Service.Store, "user", &v.Meta, &v, v.Owner()); e != nil {
		fail(w, e)
	} else {
		respond(w, 200, v)
	}
	return true
}
func (a *API) userState(w http.ResponseWriter, r *http.Request, session auth.Session, snap domain.Snapshot) {
	profiles := []map[string]any{}
	active := map[string]string{}
	for _, p := range snap.Profiles {
		if allows(session, p.ID) {
			profiles = append(profiles, map[string]any{"id": p.ID, "revision": p.Revision, "name": p.Name})
			if id := snap.Active[p.ID]; id != "" {
				active[p.ID] = id
			}
		}
	}
	devices := []domain.Device{}
	seen := map[string]string{}
	allSeen, e := a.Service.Store.SeenAll(r.Context())
	if e != nil {
		fail(w, e)
		return
	}
	for _, d := range snap.Devices {
		if d.Owner == session.Issuer+"#"+session.Subject && allows(session, d.ProfileID) {
			devices = append(devices, deviceMeta(d))
			seen[d.ID] = allSeen[d.ID]
		}
	}
	releases := []domain.Release{}
	for _, v := range snap.Releases {
		if allows(session, v.ProfileID) {
			releases = append(releases, releaseMeta(v))
		}
	}
	respond(w, 200, map[string]any{"build": buildinfo.Current(), "user": session.Name, "role": "user", "csrf": session.CSRF, "profiles": profiles, "active": active, "devices": devices, "releases": releases, "seen": seen, "sources": []any{}, "nodes": []any{}, "resources": []any{}, "users": []any{}, "validator": a.Service.Validator.Identity(), "capabilities": map[string]any{"protocols": []string{}, "groups": []string{}, "automaticPublish": false}})
}
