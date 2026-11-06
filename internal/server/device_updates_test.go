package server_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/fjzhangZzzzzz/subweft/internal/app"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"github.com/fjzhangZzzzzz/subweft/internal/testfixture"
)

func TestRemoteEditorRoundTripAndMigrationMetadata(t *testing.T) {
	h := setup(t, testfixture.Subject, false)
	if h.login(t) != 302 {
		t.Fatal("login")
	}
	_, b := h.request(t, "GET", "/api/v1/state", nil, false)
	h.csrf = decode[struct {
		CSRF string `json:"csrf"`
	}](t, b).CSRF
	input := app.SourceInput{Name: "remote", Kind: "http", URL: "https://example.invalid/sub?token=fixture-secret", Enabled: true, Interval: 3600, Update: domain.DeviceUpdate{UserAgent: "fixture-agent", Headers: map[string]string{"Authorization": "fixture-header"}, Renames: []domain.NameReplacement{{Pattern: "old", Replacement: "new"}}}}
	code, b := h.request(t, "POST", "/api/v1/sources", input, true)
	if code != 200 {
		t.Fatal("create", code)
	}
	created := decode[domain.Source](t, b)
	code, b = h.request(t, "GET", "/api/v1/sources/"+created.ID+"/edit", nil, false)
	if code != 200 {
		t.Fatal("edit")
	}
	edit := decode[app.SourceInput](t, b)
	if edit.Update.UserAgent != "fixture-agent" || edit.Update.Headers["Authorization"] != "fixture-header" || len(edit.Update.Renames) != 1 {
		t.Fatal("editor lost update settings")
	}
	edit.Name = "renamed"
	if code, _ = h.request(t, "PUT", "/api/v1/sources/"+created.ID, edit, true); code != 200 {
		t.Fatal("save", code)
	}
	src, e := storage.Load[domain.Source](context.Background(), h.store, "source", created.ID)
	if e != nil {
		t.Fatal(e)
	}
	src.Overrides = []domain.NodeOverride{{Identity: strings.Repeat("1", 64), Name: "missing cached node"}}
	if e = storage.Save(context.Background(), h.store, "source", &src.Meta, &src, ""); e != nil {
		t.Fatal(e)
	}
	code, b = h.request(t, "GET", "/api/v1/state", nil, false)
	if code != 200 || !bytes.Contains(b, []byte("missing cached node")) {
		t.Fatal("unmatched override missing from migration metadata")
	}
	for _, secret := range []string{"fixture-secret", "fixture-header"} {
		if bytes.Contains(b, []byte(secret)) {
			t.Fatal("metadata leaked credential")
		}
	}
	if code, _ = h.request(t, "POST", "/api/v1/sources/"+src.ID+"/refresh", map[string]any{"revision": src.Revision}, true); code != 410 {
		t.Fatal("refresh not retired", code)
	}
}
