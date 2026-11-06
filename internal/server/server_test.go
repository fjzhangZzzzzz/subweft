package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/app"
	"github.com/fjzhangZzzzzz/subweft/internal/auth"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/server"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
)

type checker struct{}

func (checker) Check(context.Context, []byte) error { return nil }
func (checker) Identity() string                    { return "test-validator" }

type harness struct {
	server, issuer *httptest.Server
	client         *http.Client
	store          *storage.Store
	csrf           string
}

func setup(t *testing.T, subject string, badNonce bool) *harness {
	t.Helper()
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	data := filepath.Join(dir, "state")
	if e := storage.Initialize(data, key); e != nil {
		t.Fatal(e)
	}
	store, e := storage.Open(data, key)
	if e != nil {
		t.Fatal(e)
	}
	h := &harness{store: store, server: httptest.NewUnstartedServer(nil), issuer: httptest.NewUnstartedServer(nil)}
	base := "http://" + h.server.Listener.Addr().String()
	issuerURL := "http://" + h.issuer.Listener.Addr().String()
	fixture, e := testfixture.New(issuerURL, base+"/auth/callback", subject)
	if e != nil {
		t.Fatal(e)
	}
	fixture.BadNonce = badNonce
	h.issuer.Config.Handler = fixture.Handler()
	h.issuer.Start()
	authn, e := auth.New(context.Background(), auth.Config{Issuer: issuerURL, BaseURL: base, ClientID: testfixture.ClientID, ClientSecret: testfixture.ClientSecret, AdminSubjects: []string{testfixture.Subject}, LocalHTTP: true}, store)
	if e != nil {
		t.Fatal(e)
	}
	svc := app.New(store, source.NewFetcher([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}), checker{})
	h.server.Config.Handler = (&server.API{Service: svc, Auth: authn, BaseURL: base, UI: http.NotFoundHandler()}).Handler()
	h.server.Start()
	jar, _ := cookiejar.New(nil)
	h.client = &http.Client{Jar: jar, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	t.Cleanup(func() { h.server.Close(); h.issuer.Close(); store.Close() })
	return h
}
func (h *harness) login(t *testing.T) int {
	t.Helper()
	r, e := h.client.Get(h.server.URL + "/auth/login")
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	u, e := url.Parse(r.Header.Get("Location"))
	if e != nil || u.Host == "" {
		t.Fatal("missing authorization redirect")
	}
	page, e := h.client.Get(u.String())
	if e != nil {
		t.Fatal(e)
	}
	io.Copy(io.Discard, page.Body)
	page.Body.Close()
	if page.StatusCode != 200 {
		t.Fatal("authorization page failed")
	}
	r, e = h.client.PostForm(h.issuer.URL+"/approve", u.Query())
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	callback := r.Header.Get("Location")
	if callback == "" {
		t.Fatal("missing callback")
	}
	r, e = h.client.Get(callback)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	return r.StatusCode
}
func (h *harness) request(t *testing.T, method, path string, body any, csrf bool) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
		reader = bytes.NewReader(b)
	}
	req, e := http.NewRequest(method, h.server.URL+path, reader)
	if e != nil {
		t.Fatal(e)
	}
	if method != "GET" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", h.server.URL)
		if csrf {
			req.Header.Set("X-CSRF-Token", h.csrf)
		}
	}
	r, e := h.client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	b, e := io.ReadAll(r.Body)
	if e != nil {
		t.Fatal(e)
	}
	return r.StatusCode, b
}
func decode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if e := json.Unmarshal(b, &v); e != nil {
		t.Fatal("invalid JSON response")
	}
	return v
}
func TestAuthenticatedHTTPWorkflowAndBoundaries(t *testing.T) {
	h := setup(t, testfixture.Subject, false)
	if code, _ := h.request(t, "GET", "/api/v1/state", nil, false); code != 401 {
		t.Fatal("unauthenticated API accepted")
	}
	if h.login(t) != 302 {
		t.Fatal("OIDC login failed")
	}
	status, b := h.request(t, "GET", "/api/v1/state", nil, false)
	if status != 200 {
		t.Fatal("session not usable")
	}
	state := decode[struct {
		CSRF string `json:"csrf"`
	}](t, b)
	h.csrf = state.CSRF
	input := app.SourceInput{Name: "fixture", Kind: "local", Content: testfixture.ProviderYAML, Enabled: true, Interval: 3600}
	if code, _ := h.request(t, "POST", "/api/v1/sources", input, false); code != 403 {
		t.Fatal("CSRF accepted")
	}
	status, b = h.request(t, "POST", "/api/v1/sources", input, true)
	if status != 200 {
		t.Fatal("source create failed")
	}
	src := decode[domain.Source](t, b)
	p := domain.Profile{Name: "daily", Groups: []domain.Group{{Name: "Proxy", Type: "select", SourceIDs: []string{src.ID}}}, Rules: []domain.Rule{{Type: "DOMAIN-SUFFIX", Value: "example.invalid", Target: "Proxy", Enabled: true}, {Type: "MATCH", Target: "Proxy", Enabled: true}}, Settings: domain.Settings{MixedPort: 7890}}
	status, b = h.request(t, "POST", "/api/v1/profiles", p, true)
	if status != 200 {
		t.Fatal("profile create failed")
	}
	p = decode[domain.Profile](t, b)
	status, b = h.request(t, "POST", "/api/v1/profiles/"+p.ID+"/preview", map[string]any{"revision": p.Revision, "allowStale": false}, true)
	if status != 200 || bytes.Contains(b, []byte("fixture-proxy-password")) {
		t.Fatal("preview leaked credential or failed")
	}
	status, _ = h.request(t, "POST", "/api/v1/profiles/"+p.ID+"/publish", map[string]any{"revision": p.Revision, "allowStale": false}, true)
	if status != 201 {
		t.Fatal("publish failed")
	}
	status, b = h.request(t, "POST", "/api/v1/devices", map[string]any{"name": "laptop", "profileId": p.ID, "state": "active"}, true)
	if status != 200 {
		t.Fatal("device create failed")
	}
	d := decode[domain.Device](t, b)
	if d.Token != "" {
		t.Fatal("creation response leaked token")
	}
	status, b = h.request(t, "GET", "/api/v1/devices/"+d.ID+"/link", nil, false)
	if status != 200 {
		t.Fatal("link failed")
	}
	link := decode[struct {
		URL string `json:"url"`
	}](t, b)
	r, e := http.Get(link.URL)
	if e != nil {
		t.Fatal(e)
	}
	download, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || !bytes.Contains(download, []byte("fixture-proxy-password")) {
		t.Fatal("download failed")
	}
	status, b = h.request(t, "GET", "/api/v1/state", nil, false)
	if status != 200 || bytes.Contains(b, []byte("fixture-proxy-password")) || bytes.Contains(b, []byte(strings.TrimPrefix(link.URL, h.server.URL+"/s/"))) {
		t.Fatal("state leaked credentials")
	}
	status, _ = h.request(t, "POST", "/api/v1/devices/"+d.ID+"/rotate", map[string]any{"revision": d.Revision}, true)
	if status != 200 {
		t.Fatal("rotation failed")
	}
	r, e = http.Get(link.URL)
	if e != nil {
		t.Fatal(e)
	}
	r.Body.Close()
	if r.StatusCode != 404 {
		t.Fatal("old token accepted")
	}
	status, _ = h.request(t, "POST", "/api/v1/logout", map[string]any{}, true)
	if status != 204 {
		t.Fatal("logout failed")
	}
	if code, _ := h.request(t, "GET", "/api/v1/state", nil, false); code != 401 {
		t.Fatal("logout did not revoke session")
	}
}
func TestOIDCRejectsWrongNonceAndUnauthorizedSubject(t *testing.T) {
	for _, test := range []struct {
		name, subject string
		badNonce      bool
		status        int
	}{{"wrong nonce", testfixture.Subject, true, 401}, {"not an administrator", "unlisted-subject", false, 403}} {
		t.Run(test.name, func(t *testing.T) {
			h := setup(t, test.subject, test.badNonce)
			if code := h.login(t); code != test.status {
				t.Fatal("invalid identity accepted")
			}
			if code, _ := h.request(t, "GET", "/api/v1/state", nil, false); code != 401 {
				t.Fatal("invalid session created")
			}
		})
	}
}
func TestCallbackRejectsMissingState(t *testing.T) {
	h := setup(t, testfixture.Subject, false)
	if status, _ := h.request(t, "GET", "/auth/callback?code=invalid", nil, false); status != 401 {
		t.Fatal("missing state accepted")
	}
}

func TestProtocolEditingKeepsSecretsOutOfListsAndPreviews(t *testing.T) {
	h := setup(t, testfixture.Subject, false)
	if h.login(t) != 302 {
		t.Fatal("login failed")
	}
	status, b := h.request(t, "GET", "/api/v1/state", nil, false)
	if status != 200 {
		t.Fatal("state failed")
	}
	h.csrf = decode[struct {
		CSRF string `json:"csrf"`
	}](t, b).CSRF
	nodes, e := source.Parse([]byte(testfixture.ProtocolProviderYAML))
	if e != nil {
		t.Fatal(e)
	}
	ids := []string{}
	for _, node := range nodes {
		status, b = h.request(t, "POST", "/api/v1/nodes", domain.ManualNode{Node: node, Enabled: true}, true)
		if status != 200 {
			t.Fatal("protocol node create failed")
		}
		saved := decode[domain.ManualNode](t, b)
		ids = append(ids, saved.ID)
		if saved.Node.Password != "" || saved.Node.UUID != "" || saved.Node.WS != nil {
			t.Fatal("create response exposed credentials")
		}
		status, b = h.request(t, "GET", "/api/v1/nodes/"+saved.ID+"/edit", nil, false)
		if status != 200 {
			t.Fatal("explicit edit failed")
		}
		editable := decode[domain.ManualNode](t, b)
		if editable.Node.Password != node.Password || editable.Node.UUID != node.UUID {
			t.Fatal("edit did not recover encrypted credentials")
		}
		editable.Node.Name += " edited"
		status, _ = h.request(t, "PUT", "/api/v1/nodes/"+saved.ID, editable, true)
		if status != 200 {
			t.Fatal("protocol node edit failed")
		}
	}
	profile := domain.Profile{Name: "protocols", Groups: []domain.Group{{Name: "Proxy", Type: "select", NodeIDs: ids}}, Rules: []domain.Rule{{Type: "MATCH", Target: "Proxy", Enabled: true}}, Settings: domain.Settings{MixedPort: 7890}}
	status, b = h.request(t, "POST", "/api/v1/profiles", profile, true)
	if status != 200 {
		t.Fatal("profile failed")
	}
	profile = decode[domain.Profile](t, b)
	checkSecrets := func(data []byte) {
		t.Helper()
		for _, secret := range []string{"00000000-0000-4000-8000-000000000001", "fixture-http-user", "fixture-socks-password", "fixture-private-header", "fixture-private-path", "fixture-private-service", "fixture-trojan-password", "0123456789abcdef"} {
			if bytes.Contains(data, []byte(secret)) {
				t.Fatal("sensitive node parameter was exposed")
			}
		}
	}
	status, b = h.request(t, "GET", "/api/v1/state", nil, false)
	if status != 200 {
		t.Fatal("state failed")
	}
	checkSecrets(b)
	status, b = h.request(t, "POST", "/api/v1/profiles/"+profile.ID+"/preview", map[string]any{"revision": profile.Revision}, true)
	if status != 200 {
		t.Fatal("preview failed")
	}
	checkSecrets(b)
	response, e := http.Get(h.server.URL + "/api/v1/nodes/" + ids[0] + "/edit")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("unauthenticated credential edit accepted")
	}
}
