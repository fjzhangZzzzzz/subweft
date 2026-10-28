package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/fjzhangZzzzzz/subweft/internal/buildinfo"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/fjzhangZzzzzz/subweft/internal/app"
	"github.com/fjzhangZzzzzz/subweft/internal/auth"
	"github.com/fjzhangZzzzzz/subweft/internal/server"
	"github.com/fjzhangZzzzzz/subweft/internal/source"
	"github.com/fjzhangZzzzzz/subweft/internal/storage"
	"github.com/fjzhangZzzzzz/subweft/internal/validator"
	"github.com/fjzhangZzzzzz/subweft/internal/webui"
)

func main() {
	if e := run(); e != nil {
		slog.Error("startup_failed", "code", e.Error())
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		return json.NewEncoder(os.Stdout).Encode(buildinfo.Current())
	}
	if len(os.Args) < 2 {
		return errors.New("usage_subweft_init_or_serve")
	}
	flags := flag.NewFlagSet(os.Args[1], flag.ContinueOnError)
	data := flags.String("data-dir", "", "local persistent directory")
	backupOutput := flags.String("output", "", "new absolute encrypted backup destination")
	backupInput := flags.String("backup-file", "", "existing encrypted SQLite backup")
	key := flags.String("key-file", "", "existing 32-byte key outside SQLite")
	if e := flags.Parse(os.Args[2:]); e != nil {
		return errors.New("invalid_flags")
	}
	if *data == "" || *key == "" {
		return errors.New("data_dir_and_key_file_required")
	}
	if os.Args[1] == "backup" {
		return storage.BackupDatabase(context.Background(), *data, *backupOutput)
	}
	if os.Args[1] == "restore" {
		return storage.RestoreDatabase(context.Background(), *backupInput, *data, *key)
	}
	if os.Args[1] == "init" {
		if e := storage.Initialize(*data, *key); e != nil {
			return e
		}
		fmt.Println("Initialized encrypted state; preserve the separate key with encrypted backups.")
		return nil
	}
	if os.Args[1] != "serve" {
		return errors.New("unknown_command")
	}
	if !webui.Available {
		return errors.New("web_ui_not_built_use_webui_tag")
	}
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(48 << 20)
	}
	base := strings.TrimSuffix(os.Getenv("SUBWEFT_BASE_URL"), "/")
	issuer := os.Getenv("SUBWEFT_OIDC_ISSUER")
	listen := os.Getenv("SUBWEFT_LISTEN")
	if listen == "" {
		listen = "127.0.0.1:8080"
	}
	local := os.Getenv("SUBWEFT_LOCAL_HTTP") == "true"
	if e := validateURLs(base, issuer, listen, local); e != nil {
		return e
	}
	clientID := os.Getenv("SUBWEFT_OIDC_CLIENT_ID")
	secret, secretErr := configuredSecret("SUBWEFT_OIDC_CLIENT_SECRET")
	if secretErr != nil {
		return secretErr
	}
	adminRaw := os.Getenv("SUBWEFT_ADMIN_SUBJECTS")
	if clientID == "" || secret == "" || adminRaw == "" {
		return errors.New("oidc_credentials_and_admin_subjects_required")
	}
	store, e := storage.Open(*data, *key)
	if e != nil {
		return e
	}
	defer store.Close()
	validatorSHA, supported := validator.ExpectedSHA(runtime.GOOS, runtime.GOARCH)
	if !supported {
		return errors.New("unsupported_validator_platform")
	}
	validationDir := os.Getenv("SUBWEFT_VALIDATION_DIR")
	if validationDir == "" {
		if !local {
			return errors.New("validation_tmpfs_required")
		}
		validationDir = filepath.Join(*data, "validation")
	}
	if !local {
		if e = validator.RequireVolatileDirectory(validationDir); e != nil {
			return e
		}
	}
	checker, e := validator.New(os.Getenv("SUBWEFT_MIHOMO_PATH"), validatorSHA, validationDir)
	if e != nil {
		return e
	}
	allowed := []netip.Prefix{}
	if raw := os.Getenv("SUBWEFT_FETCH_ALLOW_CIDRS"); raw != "" {
		for _, value := range strings.Split(raw, ",") {
			p, e := netip.ParsePrefix(strings.TrimSpace(value))
			if e != nil {
				return errors.New("invalid_fetch_allowlist")
			}
			allowed = append(allowed, p)
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	authn, e := auth.New(ctx, auth.Config{Issuer: issuer, ClientID: clientID, ClientSecret: secret, BaseURL: base, AdminSubjects: strings.Split(adminRaw, ","), LocalHTTP: local}, store)
	if e != nil {
		return e
	}
	svc := app.New(store, source.NewFetcher(allowed), checker)
	go svc.Schedule(ctx)
	api := &server.API{Service: svc, Auth: authn, BaseURL: base, UI: webui.Handler()}
	httpServer := &http.Server{Addr: listen, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()
	slog.Info("subweft_started", "version", buildinfo.Current().Version, "commit", buildinfo.Current().Commit, "local_http", local)
	select {
	case e := <-errCh:
		if !errors.Is(e, http.ErrServerClosed) {
			return errors.New("http_server_failed")
		}
	case <-ctx.Done():
		shutdown, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		return httpServer.Shutdown(shutdown)
	}
	return nil
}
func validateURLs(base, issuer, listen string, local bool) error {
	b, e := url.Parse(base)
	if e != nil || b.Hostname() == "" || b.User != nil || b.Path != "" || b.RawQuery != "" || b.Fragment != "" {
		return errors.New("invalid_base_url")
	}
	i, e := url.Parse(issuer)
	if e != nil || i.Hostname() == "" || i.User != nil || i.RawQuery != "" || i.Fragment != "" {
		return errors.New("invalid_issuer")
	}
	if !local {
		if b.Scheme != "https" || i.Scheme != "https" {
			return errors.New("https_required")
		}
		return nil
	}
	host, _, e := net.SplitHostPort(listen)
	if e != nil || !loopback(host) || !loopback(b.Hostname()) || (i.Scheme != "https" && (i.Scheme != "http" || !loopback(i.Hostname()))) || b.Scheme != "http" {
		return errors.New("local_http_requires_loopback_only")
	}
	return nil
}
func loopback(host string) bool { ip := net.ParseIP(host); return ip != nil && ip.IsLoopback() }

func configuredSecret(name string) (string, error) {
	value := os.Getenv(name)
	file := os.Getenv(name + "_FILE")
	if file == "" {
		return value, nil
	}
	if value != "" {
		return "", errors.New("conflicting_secret_inputs")
	}
	info, e := os.Lstat(file)
	if e != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return "", errors.New("secret_file_unavailable")
	}
	b, e := os.ReadFile(file)
	if e != nil {
		return "", errors.New("secret_file_unavailable")
	}
	value = strings.TrimRight(string(b), "\r\n")
	if value == "" {
		return "", errors.New("secret_file_empty")
	}
	return value, nil
}
