package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/fjzhangZzzzzz/subweft/internal/domain"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"golang.org/x/oauth2"
)

type Config struct {
	Issuer        string
	ClientID      string
	ClientSecret  string
	BaseURL       string
	AdminSubjects []string
	LocalHTTP     bool
}
type Auth struct {
	mu          sync.Mutex
	loginWindow time.Time
	loginCount  int
	cfg         Config
	store       *storage.Store
	oauth       oauth2.Config
	verifier    *oidc.IDTokenVerifier
}
type Session struct {
	Role       string   `json:"role"`
	ProfileIDs []string `json:"profileIds"`
	domain.Meta
	Issuer  string    `json:"issuer"`
	Subject string    `json:"subject"`
	Name    string    `json:"name"`
	CSRF    string    `json:"csrf"`
	Expires time.Time `json:"expires"`
}
type attempt struct {
	domain.Meta
	Nonce    string    `json:"nonce"`
	Verifier string    `json:"verifier"`
	Expires  time.Time `json:"expires"`
}

func New(ctx context.Context, cfg Config, store *storage.Store) (*Auth, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	provider, e := oidc.NewProvider(ctx, cfg.Issuer)
	if e != nil {
		return nil, domain.Invalid("oidc_discovery_failed", "issuer")
	}
	return &Auth{cfg: cfg, store: store, oauth: oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: cfg.BaseURL + "/auth/callback", Scopes: []string{oidc.ScopeOpenID, "profile"}}, verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID, SupportedSigningAlgs: []string{"RS256", "ES256"}})}, nil
}
func hash(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func (a *Auth) cookieName() string {
	if a.cfg.LocalHTTP {
		return "subweft_session"
	}
	return "__Host-subweft_session"
}
func (a *Auth) setCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: !a.cfg.LocalHTTP, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	if time.Since(a.loginWindow) > time.Minute {
		a.loginWindow = time.Now()
		a.loginCount = 0
	}
	a.loginCount++
	limited := a.loginCount > 20
	a.mu.Unlock()
	if limited {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "login_rate_limited", 429)
		return
	}
	if attempts, err := storage.List[attempt](r.Context(), a.store, "login"); err == nil {
		for _, old := range attempts {
			if time.Now().After(old.Expires) {
				_ = a.store.Delete(r.Context(), "login", old.ID, old.Revision)
			}
		}
	}
	state := domain.RandomToken()
	v := attempt{Meta: domain.Meta{ID: hash(state)}, Nonce: domain.RandomToken(), Verifier: oauth2.GenerateVerifier(), Expires: time.Now().Add(10 * time.Minute)}
	if e := storage.Save(r.Context(), a.store, "login", &v.Meta, &v, ""); e != nil {
		http.Error(w, "login_unavailable", 503)
		return
	}
	a.setCookie(w, "subweft_login", state, 600)
	http.Redirect(w, r, a.oauth.AuthCodeURL(state, oidc.Nonce(v.Nonce), oauth2.S256ChallengeOption(v.Verifier)), http.StatusFound)
}
func (a *Auth) Callback(w http.ResponseWriter, r *http.Request) {
	fail := func() { http.Error(w, "login_failed", http.StatusUnauthorized) }
	cookie, e := r.Cookie("subweft_login")
	state := r.URL.Query().Get("state")
	if e != nil || len(state) != 43 || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) != 1 {
		fail()
		return
	}
	a.setCookie(w, "subweft_login", "", -1)
	v, e := storage.Load[attempt](r.Context(), a.store, "login", hash(state))
	if e != nil || !time.Now().Before(v.Expires) {
		fail()
		return
	}
	if e = a.store.Delete(r.Context(), "login", v.ID, v.Revision); e != nil {
		fail()
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	token, e := a.oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(v.Verifier))
	if e != nil {
		fail()
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		fail()
		return
	}
	idToken, e := a.verifier.Verify(ctx, raw)
	if e != nil || subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(v.Nonce)) != 1 {
		fail()
		return
	}
	if _, e := a.Principal(ctx, idToken.Issuer, idToken.Subject); e != nil {
		http.Error(w, "access_denied", http.StatusForbidden)
		return
	}
	var claims struct {
		Name string `json:"name"`
	}
	_ = idToken.Claims(&claims)
	if len(claims.Name) > 160 {
		claims.Name = ""
	}
	if claims.Name == "" {
		claims.Name = "Administrator"
	}
	secret := domain.RandomToken()
	session := Session{Meta: domain.Meta{ID: hash(secret)}, Issuer: idToken.Issuer, Subject: idToken.Subject, Name: claims.Name, CSRF: domain.RandomToken(), Expires: time.Now().Add(8 * time.Hour)}
	if e = storage.Save(ctx, a.store, "session", &session.Meta, &session, ""); e != nil {
		http.Error(w, "login_unavailable", 503)
		return
	}
	a.setCookie(w, a.cookieName(), secret, 8*3600)
	http.Redirect(w, r, "/", http.StatusFound)
}
func (a *Auth) Session(r *http.Request) (Session, error) {
	c, e := r.Cookie(a.cookieName())
	if e != nil || len(c.Value) != 43 {
		return Session{}, domain.ErrNotFound
	}
	s, e := storage.Load[Session](r.Context(), a.store, "session", hash(c.Value))
	if e != nil || !time.Now().Before(s.Expires) || s.Issuer != a.cfg.Issuer {
		return Session{}, domain.ErrNotFound
	}
	user, e := a.Principal(r.Context(), s.Issuer, s.Subject)
	if e != nil {
		return Session{}, domain.ErrNotFound
	}
	s.Role = user.Role
	s.ProfileIDs = user.ProfileIDs
	return s, nil
}
func (a *Auth) CheckMutation(r *http.Request, s Session) bool {
	return r.Header.Get("Origin") == a.cfg.BaseURL && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(s.CSRF)) == 1 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")
}
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request, s Session) {
	_ = a.store.Delete(r.Context(), "session", s.ID, s.Revision)
	a.setCookie(w, a.cookieName(), "", -1)
	w.WriteHeader(http.StatusNoContent)
}

func (a *Auth) Issuer() string { return a.cfg.Issuer }
func (a *Auth) IsBootstrap(issuer, subject string) bool {
	if issuer != a.cfg.Issuer {
		return false
	}
	for _, sub := range a.cfg.AdminSubjects {
		if sub == subject {
			return true
		}
	}
	return false
}
func (a *Auth) BootstrapUsers() []domain.User {
	out := []domain.User{}
	for _, sub := range a.cfg.AdminSubjects {
		out = append(out, domain.User{Meta: domain.Meta{ID: domain.UserID(a.cfg.Issuer, sub)}, Issuer: a.cfg.Issuer, Subject: sub, Name: "Bootstrap administrator", Role: "admin", Enabled: true, Bootstrap: true})
	}
	return out
}
func (a *Auth) Principal(ctx context.Context, issuer, subject string) (domain.User, error) {
	if a.IsBootstrap(issuer, subject) {
		return domain.User{Issuer: issuer, Subject: subject, Role: "admin", Enabled: true}, nil
	}
	if issuer != a.cfg.Issuer {
		return domain.User{}, domain.ErrNotFound
	}
	u, e := storage.Load[domain.User](ctx, a.store, "user", domain.UserID(issuer, subject))
	if e != nil || !u.Enabled {
		return domain.User{}, domain.ErrNotFound
	}
	return u, nil
}
func (a *Auth) DeviceAllowed(ctx context.Context, d domain.Device) bool {
	parts := strings.SplitN(d.Owner, "#", 2)
	if len(parts) != 2 {
		return false
	}
	u, e := a.Principal(ctx, parts[0], parts[1])
	return e == nil && u.Allows(d.ProfileID)
}
