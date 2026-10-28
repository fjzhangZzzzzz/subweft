package server_test

import (
	"bytes"
	"context"
	"github.com/fjzhangZzzzzz/subweft/internal/app"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
	"io"
	"net/http"
	"testing"
)

func TestUserGrantsOwnDevicesAndImmediateRevocation(t *testing.T) {
	h := setup(t, "ordinary-fixture-subject", false)
	ctx := context.Background()
	svc := app.New(h.store, source.NewFetcher(nil), checker{})
	src, e := svc.SaveSource(ctx, app.SourceInput{Name: "private-source", Kind: "local", Content: testfixture.ProviderYAML, Enabled: true, Interval: 3600})
	if e != nil {
		t.Fatal(e)
	}
	p, e := svc.SaveProfile(ctx, domain.Profile{Name: "shared profile", Groups: []domain.Group{{Name: "Proxy", Type: "select", SourceIDs: []string{src.ID}}}, Rules: []domain.Rule{{Type: "MATCH", Target: "Proxy", Enabled: true}}, Settings: domain.Settings{MixedPort: 7890, Options: map[string]any{"secret": "fixture-controller-secret"}}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Publish(ctx, p.ID, p.Revision, false); e != nil {
		t.Fatal(e)
	}
	user := domain.User{Meta: domain.Meta{ID: domain.UserID(h.issuer.URL, "ordinary-fixture-subject")}, Issuer: h.issuer.URL, Subject: "ordinary-fixture-subject", Name: "Household user", Role: "user", Enabled: true, ProfileIDs: []string{p.ID}}
	if e = storage.Save(ctx, h.store, "user", &user.Meta, &user, user.Owner()); e != nil {
		t.Fatal(e)
	}
	other, e := svc.SaveDevice(ctx, domain.Device{Name: "another user's device", ProfileID: p.ID, State: "active"}, h.issuer.URL+"#"+testfixture.Subject)
	if e != nil {
		t.Fatal(e)
	}
	if h.login(t) != 302 {
		t.Fatal("approved user could not log in")
	}
	code, b := h.request(t, "GET", "/api/v1/state", nil, false)
	if code != 200 || bytes.Contains(b, []byte("fixture-proxy-password")) || bytes.Contains(b, []byte("fixture-controller-secret")) || bytes.Contains(b, []byte("private-source")) || bytes.Contains(b, []byte(other.ID)) {
		t.Fatal("user state leaked management data")
	}
	state := decode[struct {
		CSRF string `json:"csrf"`
		Role string `json:"role"`
	}](t, b)
	h.csrf = state.CSRF
	if state.Role != "user" {
		t.Fatal("role elevated")
	}
	if code, _ = h.request(t, "POST", "/api/v1/sources", app.SourceInput{}, true); code != 403 {
		t.Fatal("user could manage shared resources")
	}
	if code, _ = h.request(t, "GET", "/api/v1/devices/"+other.ID+"/link", nil, false); code != 404 {
		t.Fatal("another user's link accessible")
	}
	code, b = h.request(t, "POST", "/api/v1/devices", map[string]any{"name": "own device", "profileId": p.ID, "state": "active"}, true)
	if code != 200 {
		t.Fatal("user could not create own device")
	}
	device := decode[domain.Device](t, b)
	code, b = h.request(t, "GET", "/api/v1/devices/"+device.ID+"/link", nil, false)
	if code != 200 {
		t.Fatal("own link unavailable")
	}
	link := decode[struct {
		URL string `json:"url"`
	}](t, b)
	response, e := http.Get(link.URL)
	if e != nil {
		t.Fatal(e)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("granted device download unavailable")
	}
	user.ProfileIDs = nil
	if e = storage.Save(ctx, h.store, "user", &user.Meta, &user, user.Owner()); e != nil {
		t.Fatal(e)
	}
	response, e = http.Get(link.URL)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatal("removed grant still downloadable")
	}
	if code, _ = h.request(t, "GET", "/api/v1/devices/"+device.ID+"/link", nil, false); code != 404 {
		t.Fatal("removed grant still manageable")
	}
	user.Enabled = false
	if e = storage.Save(ctx, h.store, "user", &user.Meta, &user, user.Owner()); e != nil {
		t.Fatal(e)
	}
	if code, _ = h.request(t, "GET", "/api/v1/state", nil, false); code != 401 {
		t.Fatal("disabled user retained session access")
	}
}
