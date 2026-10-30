package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/auth"
	"github.com/fjzhangZzzzzz/subweft/internal/configuration"
	"github.com/fjzhangZzzzzz/subweft/internal/server"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
)

func editorHarness(t *testing.T, subject string) *harness {
	h := setup(t, subject, false)
	a, e := auth.New(context.Background(), auth.Config{Issuer: h.issuer.URL, BaseURL: h.server.URL, ClientID: testfixture.ClientID, ClientSecret: testfixture.ClientSecret, AdminSubjects: []string{testfixture.Subject}, LocalHTTP: true}, h.store)
	if e != nil {
		t.Fatal(e)
	}
	h.server.Config.Handler = (&server.Editor{Service: configuration.New(h.store, checker{}), Auth: a, BaseURL: h.server.URL, UI: http.NotFoundHandler()}).Handler()
	return h
}
func TestEditorAuthorizationDraftAndRemovedRoutes(t *testing.T) {
	h := editorHarness(t, testfixture.Subject)
	if code, _ := h.request(t, "GET", "/api/v1/state", nil, false); code != 401 {
		t.Fatal("anonymous management accepted")
	}
	if h.login(t) != 302 {
		t.Fatal("OIDC login failed")
	}
	code, b := h.request(t, "GET", "/api/v1/state", nil, false)
	if code != 200 {
		t.Fatal("state unavailable")
	}
	var state struct {
		CSRF  string              `json:"csrf"`
		Draft configuration.Draft `json:"draft"`
	}
	if json.Unmarshal(b, &state) != nil {
		t.Fatal("invalid state")
	}
	h.csrf = state.CSRF
	if code, _ = h.request(t, "PUT", "/api/v1/draft", state.Draft, false); code != 403 {
		t.Fatal("CSRF missing accepted")
	}
	state.Draft.Config.Subscriptions = append(state.Draft.Config.Subscriptions, configuration.Subscription{ID: "subscription", Name: "Provider", URL: "https://example.com/fixture-secret", Enabled: true, Pool: "quality", Interval: 3600, Proxy: "DIRECT"})
	if code, _ = h.request(t, "PUT", "/api/v1/draft", state.Draft, true); code != 200 {
		t.Fatal("draft save failed")
	}
	if code, _ = h.request(t, "PUT", "/api/v1/draft", state.Draft, true); code != 409 {
		t.Fatal("stale revision accepted")
	}
	code, b = h.request(t, "GET", "/api/v1/state", nil, false)
	if code != 200 || strings.Contains(string(b), "fixture-secret") {
		t.Fatal("state exposed secret")
	}
	for _, path := range []string{"/api/v1/sources", "/api/v1/users", "/api/v1/profiles", "/api/v1/nodes", "/api/v1/resources"} {
		if code, _ = h.request(t, "GET", path, nil, false); code != 404 {
			t.Fatal("old business route remains available")
		}
	}
	if code, _ = h.request(t, "GET", "/s/invalid/config.yaml", nil, false); code != 404 {
		t.Fatal("invalid device request accepted")
	}
}
func TestEditorUnknownFieldsRejected(t *testing.T) {
	h := editorHarness(t, testfixture.Subject)
	h.login(t)
	_, b := h.request(t, "GET", "/api/v1/state", nil, false)
	var state struct {
		CSRF string `json:"csrf"`
	}
	json.Unmarshal(b, &state)
	h.csrf = state.CSRF
	if code, _ := h.request(t, "PUT", "/api/v1/draft", map[string]any{"revision": 1, "unexpected": true}, true); code != 422 {
		t.Fatal("unknown fields discarded")
	}
}

func TestEditorDoesNotGrantAdministrationOnLogin(t *testing.T) {
	h := editorHarness(t, "fixture-member")
	if h.login(t) != 403 {
		t.Fatal("unlisted identity was not rejected at callback")
	}
	if code, _ := h.request(t, "GET", "/api/v1/state", nil, false); code != 401 {
		t.Fatal("non-admin identity granted management")
	}
}
