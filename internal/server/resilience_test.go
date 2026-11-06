package server_test

import (
	"bytes"
	"context"
	"github.com/fjzhangZzzzzz/subweft/internal/app"
	"github.com/fjzhangZzzzzz/subweft/internal/auth"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/server"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestColdRestartWithUnavailableIdentityAndHTTPBackup(t *testing.T) {
	h := setup(t, testfixture.Subject, false)
	if h.login(t) != 302 {
		t.Fatal("login")
	}
	_, b := h.request(t, "GET", "/api/v1/state", nil, false)
	h.csrf = decode[struct {
		CSRF string `json:"csrf"`
	}](t, b).CSRF
	code, b := h.request(t, "POST", "/api/v1/backup", struct{}{}, true)
	if code != 200 || !bytes.HasPrefix(b, []byte("SQLite format 3")) {
		t.Fatal("HTTP backup not a database")
	}
	backup := filepath.Join(t.TempDir(), "backup.sqlite")
	if e := os.WriteFile(backup, b, 0600); e != nil {
		t.Fatal(e)
	}
	svc := app.New(h.store, source.NewFetcher(nil), checker{})
	ctx := context.Background()
	n, e := svc.SaveNode(ctx, domain.ManualNode{Node: domain.Node{Name: "fixture", Type: "ss", Server: "192.0.2.1", Port: 443, Cipher: "aes-128-gcm", Password: "fixture-password"}, Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	p, e := svc.SaveProfile(ctx, domain.Profile{Name: "resilient", Groups: []domain.Group{{Name: "Proxy", Type: "select", NodeIDs: []string{n.ID}}}, Rules: []domain.Rule{{Type: "MATCH", Target: "Proxy", Enabled: true}}, Settings: domain.Settings{MixedPort: 7890}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = svc.Publish(ctx, p.ID, p.Revision, false); e != nil {
		t.Fatal(e)
	}
	d, e := svc.SaveDevice(ctx, domain.Device{Name: "device", ProfileID: p.ID, State: "active"}, h.issuer.URL+"#"+testfixture.Subject)
	if e != nil {
		t.Fatal(e)
	}
	h.issuer.Close()
	a, e := auth.New(ctx, auth.Config{Issuer: h.issuer.URL, BaseURL: h.server.URL, ClientID: testfixture.ClientID, ClientSecret: testfixture.ClientSecret, AdminSubjects: []string{testfixture.Subject}, LocalHTTP: true}, h.store)
	if e != nil {
		t.Fatal("offline issuer blocked restart")
	}
	web := httptest.NewServer((&server.API{Service: svc, Auth: a, BaseURL: h.server.URL, UI: http.NotFoundHandler()}).Handler())
	defer web.Close()
	r, e := http.Get(web.URL + "/s/" + d.Token + "/config.yaml")
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatal("cached download needs issuer")
	}
	r, e = http.Get(web.URL + "/auth/login")
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 503 {
		t.Fatal("offline login should fail closed")
	}
	if _, e = h.store.ReleaseSummaries(ctx); e != nil {
		t.Fatal(e)
	}
}
func TestEmptyGroupSelectionsHaveArrayWireShape(t *testing.T) {
	h := setup(t, testfixture.Subject, false)
	if h.login(t) != 302 {
		t.Fatal("login")
	}
	_, b := h.request(t, "GET", "/api/v1/state", nil, false)
	h.csrf = decode[struct {
		CSRF string `json:"csrf"`
	}](t, b).CSRF
	p := domain.Profile{Name: "empty", Groups: []domain.Group{{Name: "Proxy", Type: "select", Builtins: []string{"DIRECT"}}}, Rules: []domain.Rule{{Type: "MATCH", Target: "Proxy", Enabled: true}}}
	code, b := h.request(t, "POST", "/api/v1/profiles", p, true)
	if code != 200 {
		t.Fatal("save")
	}
	for _, field := range []string{"sourceIds", "tags", "nodeIds", "groups"} {
		if !bytes.Contains(b, []byte("\""+field+"\":[]")) {
			t.Fatalf("selection %s is not an array", field)
		}
	}
}
