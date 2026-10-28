// Package testfixture provides an explicit loopback-only OIDC issuer for tests.
// It is never linked into the Subweft server executable.
package testfixture

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/go-jose/go-jose/v4"
)

const ClientID = "subweft-local"
const ClientSecret = "fixture-client-secret-not-for-production"
const Subject = "fixture-admin"
const ProviderYAML = "proxies:\n  - name: HQ-01\n    type: ss\n    server: 192.0.2.10\n    port: 443\n    cipher: aes-128-gcm\n    password: fixture-proxy-password\n    udp: true\n  - name: QUOTA-01\n    type: ss\n    server: 198.51.100.10\n    port: 443\n    cipher: aes-256-gcm\n    password: fixture-proxy-password\n    udp: true\n"

type authorization struct {
	Nonce, Challenge, Subject string
	Expires                   time.Time
}
type Issuer struct {
	URL, Redirect, Subject string
	BadNonce               bool
	MemberSubject          string
	Country                []byte
	key                    *rsa.PrivateKey
	mu                     sync.Mutex
	codes                  map[string]authorization
}

func New(issuer, redirect, subject string) (*Issuer, error) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		return nil, e
	}
	return &Issuer{URL: issuer, Redirect: redirect, Subject: subject, key: key, codes: map[string]authorization{}}, nil
}
func (f *Issuer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]any{"issuer": f.URL, "authorization_endpoint": f.URL + "/authorize", "token_endpoint": f.URL + "/token", "jwks_uri": f.URL + "/jwks", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}, "code_challenge_methods_supported": []string{"S256"}})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, r *http.Request) {
		write(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &f.key.PublicKey, KeyID: "fixture-key", Algorithm: "RS256", Use: "sig"}}})
	})
	valid := func(v url.Values) bool {
		return v.Get("client_id") == ClientID && v.Get("redirect_uri") == f.Redirect && v.Get("response_type") == "code" && v.Get("code_challenge_method") == "S256" && v.Get("code_challenge") != "" && v.Get("nonce") != "" && len(v.Get("state")) == 43
	}
	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		if !valid(r.URL.Query()) {
			http.Error(w, "invalid_fixture_request", 400)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl := template.Must(template.New("login").Parse(`<!doctype html><html lang="en"><head><meta name="viewport" content="width=device-width, initial-scale=1"><title>Subweft local OIDC fixture</title></head><body><h1>Local test identity</h1><p>This loopback fixture is for development only. It does not authenticate a real Pocket ID user.</p><form method="post" action="/approve">{{range $key,$values:=.Params}}{{range $values}}<input type="hidden" name="{{$key}}" value="{{.}}">{{end}}{{end}}<button type="submit">Continue as test administrator</button>{{if .Member}}<button type="submit" name="account" value="member">Continue as test member</button>{{end}}</form></body></html>`))
		_ = tmpl.Execute(w, struct {
			Params url.Values
			Member bool
		}{r.URL.Query(), f.MemberSubject != ""})
	})
	mux.HandleFunc("POST /approve", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		if r.ParseForm() != nil || !valid(r.PostForm) {
			http.Error(w, "invalid_fixture_request", 400)
			return
		}
		code := domain.RandomToken()
		f.mu.Lock()
		for key, a := range f.codes {
			if time.Now().After(a.Expires) {
				delete(f.codes, key)
			}
		}
		subject := f.Subject
		if r.FormValue("account") == "member" && f.MemberSubject != "" {
			subject = f.MemberSubject
		}
		f.codes[code] = authorization{r.FormValue("nonce"), r.FormValue("code_challenge"), subject, time.Now().Add(time.Minute)}
		f.mu.Unlock()
		u, _ := url.Parse(f.Redirect)
		q := u.Query()
		q.Set("state", r.FormValue("state"))
		q.Set("code", code)
		u.RawQuery = q.Encode()
		http.Redirect(w, r, u.String(), 302)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		if r.ParseForm() != nil {
			http.Error(w, "invalid_request", 400)
			return
		}
		id, secret, ok := r.BasicAuth()
		if !ok {
			id = r.FormValue("client_id")
			secret = r.FormValue("client_secret")
		}
		if id != ClientID || secret != ClientSecret || r.FormValue("redirect_uri") != f.Redirect || r.FormValue("grant_type") != "authorization_code" {
			http.Error(w, "invalid_client", 401)
			return
		}
		f.mu.Lock()
		a, exists := f.codes[r.FormValue("code")]
		delete(f.codes, r.FormValue("code"))
		f.mu.Unlock()
		challenge := sha256.Sum256([]byte(r.FormValue("code_verifier")))
		if !exists || time.Now().After(a.Expires) || base64.RawURLEncoding.EncodeToString(challenge[:]) != a.Challenge {
			http.Error(w, "invalid_grant", 400)
			return
		}
		nonce := a.Nonce
		if f.BadNonce {
			nonce = "wrong-nonce"
		}
		displayName := "Test administrator"
		if a.Subject != f.Subject {
			displayName = "Test member"
		}
		claims, _ := json.Marshal(map[string]any{"iss": f.URL, "aud": ClientID, "sub": a.Subject, "exp": time.Now().Add(10 * time.Minute).Unix(), "iat": time.Now().Unix(), "nonce": nonce, "name": displayName})
		signer, e := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, (&jose.SignerOptions{}).WithHeader("kid", "fixture-key"))
		if e != nil {
			http.Error(w, "fixture_failed", 500)
			return
		}
		object, e := signer.Sign(claims)
		if e != nil {
			http.Error(w, "fixture_failed", 500)
			return
		}
		token, e := object.CompactSerialize()
		if e != nil {
			http.Error(w, "fixture_failed", 500)
			return
		}
		write(w, map[string]any{"access_token": "fixture-access-token", "token_type": "Bearer", "expires_in": 600, "id_token": token})
	})
	mux.HandleFunc("GET /provider.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		_, _ = w.Write([]byte(ProviderYAML))
	})
	mux.HandleFunc("GET /all.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		_, _ = w.Write([]byte(ProviderYAML + strings.TrimPrefix(ProtocolProviderYAML, "proxies:\n") + strings.TrimPrefix(RemainingProtocolProviderYAML, "proxies:\n")))
	})
	mux.HandleFunc("GET /rules.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		_, _ = w.Write([]byte("payload:\n  - +.example.invalid\n"))
	})
	mux.HandleFunc("GET /country.mmdb", func(w http.ResponseWriter, r *http.Request) {
		if len(f.Country) == 0 {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(f.Country)
	})
	mux.HandleFunc("GET /migration.yaml", func(w http.ResponseWriter, r *http.Request) {
		input := strings.ReplaceAll(MigrationConfig, "https://subscriptions.example.invalid/primary?credential=synthetic-primary-secret", f.URL+"/provider.yaml?source=primary")
		input = strings.ReplaceAll(input, "https://subscriptions.example.invalid/quota?credential=synthetic-quota-secret", f.URL+"/all.yaml?source=quota")
		input = strings.ReplaceAll(input, "https://rules.example.invalid/public.yaml", f.URL+"/rules.yaml")
		input += "\ngeox-url:\n  mmdb: " + f.URL + "/country.mmdb\n"
		w.Header().Set("Content-Type", "text/yaml")
		_, _ = w.Write([]byte(input))
	})
	return mux
}
func write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
