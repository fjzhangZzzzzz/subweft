package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/fjzhangZzzzzz/subweft/internal/auth"
	"github.com/fjzhangZzzzzz/subweft/internal/configuration"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
)

type Editor struct {
	Service *configuration.Service
	Auth    *auth.Auth
	BaseURL string
	UI      http.Handler
}

func hideConfig(c configuration.Config) configuration.Config {
	c = configuration.Clone(c)
	for i := range c.Subscriptions {
		c.Subscriptions[i].URL = "[地址已隐藏]"
	}
	for i := range c.RuleSets {
		if c.RuleSets[i].Kind == "http" {
			c.RuleSets[i].URL = "[地址已隐藏]"
		}
	}
	return c
}
func (a *Editor) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if a.Service.Store.Ping(r.Context()) != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		respond(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /auth/login", a.Auth.Login)
	mux.HandleFunc("GET /auth/callback", a.Auth.Callback)
	mux.HandleFunc("GET /s/{token}/config.yaml", func(w http.ResponseWriter, r *http.Request) {
		data, e := a.Service.Download(r.Context(), r.PathValue("token"))
		if e != nil {
			http.Error(w, "not_found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="config.yaml"`)
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/api/v1/", a.manage)
	mux.Handle("/", a.UI)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}
func editorRead(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		fail(w, domain.Invalid("invalid_json", "request"))
		return false
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		fail(w, domain.Invalid("invalid_json", "request"))
		return false
	}
	return true
}
func (a *Editor) manage(w http.ResponseWriter, r *http.Request) {
	session, e := a.Auth.Session(r)
	if e != nil {
		respond(w, 401, map[string]any{"error": domain.Error{Code: "login_required"}})
		return
	}
	if !a.Auth.IsBootstrap(session.Issuer, session.Subject) {
		respond(w, 403, map[string]any{"error": domain.Error{Code: "access_denied"}})
		return
	}
	if r.Method != "GET" && !a.Auth.CheckMutation(r, session) {
		respond(w, 403, map[string]any{"error": domain.Error{Code: "csrf_rejected"}})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	ctx := r.Context()
	s := a.Service
	if path == "logout" && r.Method == "POST" {
		a.Auth.Logout(w, r, session)
		return
	}
	if (path == "import/preview" || path == "import") && r.Method == "POST" {
		var in struct {
			Revision int    `json:"revision"`
			Content  string `json:"content"`
		}
		if !editorRead(w, r, &in) {
			return
		}
		if path == "import/preview" {
			c, e := configuration.ImportPreview(in.Content)
			if e != nil {
				fail(w, e)
				return
			}
			respond(w, 200, map[string]any{"summary": configuration.ImportSummary(c), "config": hideConfig(c)})
			return
		}
		v, e := s.Import(ctx, in.Revision, in.Content)
		if e != nil {
			fail(w, e)
			return
		}
		v.Config = hideConfig(v.Config)
		respond(w, 200, v)
		return
	}
	if path == "state" && r.Method == "GET" {
		d, check, e := s.Read(ctx)
		if e != nil {
			fail(w, e)
			return
		}
		devices, e := s.Devices(ctx)
		if e != nil {
			fail(w, e)
			return
		}
		seen, e := s.Store.SeenAll(ctx)
		if e != nil {
			fail(w, e)
			return
		}
		releases := []configuration.Release{}
		activeConfig := configuration.Default()
		for _, v := range d.Releases {
			if v.Number == d.Active {
				activeConfig = v.Config
			}
			v.Release.Config = hideConfig(v.Config)
			releases = append(releases, v.Release)
		}
		draftYAML, _ := configuration.RenderDiff(d.Draft.Config, activeConfig, "新值")
		activeYAML, _ := configuration.RenderDiff(activeConfig, d.Draft.Config, "旧值")
		if d.Active == 0 {
			activeYAML = nil
		}
		changes := []string{}
		for _, v := range d.Draft.Config.Subscriptions {
			for _, old := range activeConfig.Subscriptions {
				if v.ID == old.ID && v.URL != old.URL {
					changes = append(changes, "订阅「"+v.Name+"」地址已修改")
				}
			}
		}
		for _, v := range d.Draft.Config.RuleSets {
			for _, old := range activeConfig.RuleSets {
				if v.ID == old.ID && v.URL != old.URL {
					changes = append(changes, "远程规则集「"+v.Name+"」地址已修改")
				}
			}
		}
		d.Draft.Config = hideConfig(d.Draft.Config)
		respond(w, 200, map[string]any{"draft": d.Draft, "active": d.Active, "releases": releases, "devices": devices, "seen": seen, "check": check, "csrf": session.CSRF, "name": session.Name, "draftYAML": string(draftYAML), "activeYAML": string(activeYAML), "sensitiveChanges": changes})
		return
	}
	if path == "draft" && r.Method == "PUT" {
		var in configuration.Draft
		if !editorRead(w, r, &in) {
			return
		}
		v, e := s.Save(ctx, in)
		if e != nil {
			fail(w, e)
			return
		}
		v.Config = hideConfig(v.Config)
		respond(w, 200, v)
		return
	}
	if (path == "check" || path == "publish" || path == "restore") && r.Method == "POST" {
		var in struct {
			Revision int    `json:"revision"`
			Number   int    `json:"number"`
			Note     string `json:"note"`
		}
		if !editorRead(w, r, &in) {
			return
		}
		switch path {
		case "check":
			v, e := s.Check(ctx, in.Revision)
			if e != nil {
				fail(w, e)
				return
			}
			respond(w, 200, v)
		case "publish":
			v, e := s.Publish(ctx, in.Revision, in.Note)
			if e != nil {
				fail(w, e)
				return
			}
			v.Config = hideConfig(v.Config)
			respond(w, 200, v)
		case "restore":
			v, e := s.Restore(ctx, in.Revision, in.Number)
			if e != nil {
				fail(w, e)
				return
			}
			v.Config = hideConfig(v.Config)
			respond(w, 200, v)
		}
		return
	}
	if strings.HasPrefix(path, "devices/") && r.Method == "GET" {
		parts := strings.Split(path, "/")
		if len(parts) == 3 && parts[2] == "link" {
			token, e := s.DeviceToken(ctx, parts[1])
			if e != nil {
				fail(w, e)
				return
			}
			respond(w, 200, map[string]string{"url": a.BaseURL + "/s/" + token + "/config.yaml"})
			return
		}
	}
	if path == "devices" && r.Method == "POST" {
		var in struct {
			Device configuration.Device `json:"device"`
			Action string               `json:"action"`
		}
		if !editorRead(w, r, &in) {
			return
		}
		v, e := s.SaveDevice(ctx, in.Device, in.Action)
		if e != nil {
			fail(w, e)
			return
		}
		respond(w, 200, v)
		return
	}
	if strings.HasPrefix(path, "releases/") && r.Method == "GET" {
		number, e := strconv.Atoi(strings.TrimPrefix(path, "releases/"))
		if e == nil {
			d, _, err := s.Read(ctx)
			if err != nil {
				fail(w, err)
				return
			}
			for _, v := range d.Releases {
				if v.Number == number {
					data, err := configuration.Render(v.Config, true)
					if err != nil {
						fail(w, err)
						return
					}
					respond(w, 200, map[string]string{"yaml": string(data)})
					return
				}
			}
		}
	}
	http.NotFound(w, r)
}
