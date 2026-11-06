package server

import (
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/migrate"
	"net/http"
)

func (a *API) manageImport(w http.ResponseWriter, r *http.Request, id, action string) bool {
	if id == "preview" && action == "" && r.Method == "POST" {
		var input migrate.Input
		if !read(w, r, &input) {
			return true
		}
		candidate, e := a.Service.PreviewImport(r.Context(), input)
		if e != nil {
			fail(w, e)
			return true
		}
		plan := candidate.Plan
		sources := []sourceView{}
		for _, v := range plan.Sources {
			sources = append(sources, sourceMeta(v))
		}
		nodes := []domain.ManualNode{}
		for _, v := range plan.Nodes {
			nodes = append(nodes, nodeMeta(v))
		}
		resources := []domain.Resource{}
		for _, v := range plan.Resources {
			resources = append(resources, resourceMeta(v.Resource))
		}
		groups := []map[string]any{}
		for _, g := range plan.Profile.Groups {
			groups = append(groups, map[string]any{"name": g.Name, "type": g.Type, "sourceIds": g.SourceIDs, "tags": g.Tags, "allSources": g.AllSources, "manualNodes": len(g.NodeIDs)})
		}
		respond(w, 200, map[string]any{"id": candidate.ID, "revision": candidate.Revision, "expires": candidate.Expires, "name": plan.Profile.Name, "sources": sources, "nodes": nodes, "resources": resources, "groups": groups, "ruleCount": len(plan.Profile.Rules), "dnsEnabled": plan.Profile.Settings.DNSEnabled, "warnings": plan.Warnings})
		return true
	}
	if id != "" && action == "commit" && r.Method == "POST" {
		var in struct {
			Revision int `json:"revision"`
		}
		if !read(w, r, &in) {
			return true
		}
		p, e := a.Service.CommitImport(r.Context(), id, in.Revision)
		if e != nil {
			fail(w, e)
		} else {
			respond(w, 201, p)
		}
		return true
	}
	if id != "" && action == "" && r.Method == "DELETE" {
		var in struct {
			Revision int `json:"revision"`
		}
		if !read(w, r, &in) {
			return true
		}
		if e := a.Service.Store.Delete(r.Context(), "import-candidate", id, in.Revision); e != nil {
			fail(w, e)
		} else {
			w.WriteHeader(204)
		}
		return true
	}
	return false
}
