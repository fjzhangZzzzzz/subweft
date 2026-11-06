package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

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
	if code, body := h.request(t, "GET", "/", nil, false); code != 200 || !strings.Contains(string(body), "继续登录") {
		t.Fatal("anonymous login page unavailable")
	}
	if code, body := h.request(t, "GET", "/auth/callback?error=private-provider-detail", nil, false); code != 401 || !strings.Contains(string(body), "登录未完成，请重试。") || strings.Contains(string(body), "private-provider-detail") {
		t.Fatal("failed callback lost safe retry presentation")
	}
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
	state.Draft.Config.Subscriptions = append(state.Draft.Config.Subscriptions, configuration.Subscription{ID: "subscription", Name: "Provider", URL: "https://example.com/fixture-secret", Enabled: true, Tags: []string{"高质量"}, Interval: 3600, Proxy: "DIRECT"})
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

func TestEditorSubscriptionDownloadFilename(t *testing.T) {
	h := editorHarness(t, testfixture.Subject)
	svc := configuration.New(h.store, checker{})
	ctx := context.Background()
	d, _, err := svc.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Check(ctx, d.Draft.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Publish(ctx, d.Draft.Revision, ""); err != nil {
		t.Fatal(err)
	}
	device, err := svc.SaveDevice(ctx, configuration.Device{Name: "test device"}, "save")
	if err != nil {
		t.Fatal(err)
	}
	token, err := svc.DeviceToken(ctx, device.ID)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(h.server.URL + "/s/" + token + "/config.yaml")
	if err != nil {
		t.Fatal("subscription request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal("subscription download failed")
	}
	if response.Header.Get("Content-Disposition") != "attachment; filename=Subweft" {
		t.Fatal("subscription filename must identify Subweft")
	}
	header := response.Header.Get("Content-Disposition")
	_, params, err := mime.ParseMediaType(header)
	if err != nil || params["filename"] != "Subweft" {
		t.Fatal("standard filename parsing failed")
	}
	// Clash Verge Rev 2.5.8 parses a debug-escaped HeaderValue then trims quotes.
	// For this ASCII header strconv.Quote produces the same quoted representation.
	parseClientName := func(header string) string {
		for _, part := range strings.Split(strings.Trim(strconv.Quote(header), `"`), ";") {
			key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
			if ok && key == "filename" {
				return strings.Trim(value, `"`)
			}
		}
		return ""
	}
	if parseClientName(`attachment; filename="Subweft.yaml"`) == "Subweft.yaml" {
		t.Fatal("client regression model did not reproduce quoted filename bug")
	}
	if parseClientName(header) != "Subweft" {
		t.Fatal("client subscription name contains escaping or extension")
	}
	body, err := io.ReadAll(response.Body)
	if err != nil || !strings.Contains(string(body), "rules:") {
		t.Fatal("configuration body missing")
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

// A Connection: close client may send headers and body in separate TCP writes.
// Rejecting before consuming a small body must still deliver the HTTP error.
func TestEditorCSRFRejectionWithDelayedBody(t *testing.T) {
	h := editorHarness(t, testfixture.Subject)
	if h.login(t) != 302 {
		t.Fatal("login failed")
	}
	u, _ := url.Parse(h.server.URL)
	cookies := h.client.Jar.Cookies(u)
	request := &http.Request{Header: make(http.Header)}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	for i := 0; i < 10; i++ {
		conn, err := net.DialTimeout("tcp", u.Host, time.Second)
		if err != nil {
			t.Fatal("connection failed")
		}
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		body := `{"revision":1}`
		_, err = fmt.Fprintf(conn, "PUT /api/v1/draft HTTP/1.1\r\nHost: %s\r\nOrigin: %s\r\nCookie: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", u.Host, h.server.URL, request.Header.Get("Cookie"), len(body))
		if err != nil {
			conn.Close()
			t.Fatal("header send failed")
		}
		time.Sleep(20 * time.Millisecond)
		if _, err = io.WriteString(conn, body); err != nil {
			conn.Close()
			t.Fatal("server closed before request body")
		}
		response, err := http.ReadResponse(bufio.NewReader(conn), request)
		if err != nil {
			conn.Close()
			t.Fatal("HTTP rejection lost to connection reset")
		}
		_, err = io.ReadAll(response.Body)
		response.Body.Close()
		conn.Close()
		if err != nil || response.StatusCode != 403 {
			t.Fatal("missing complete CSRF rejection")
		}
	}
}
