package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
