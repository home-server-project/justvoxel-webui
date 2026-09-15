package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/home-server-project/justvoxel-webui/internal/api"
)

const (
	sessionCookie    = "jv_session"
	csrfCookie       = "jv_csrf"
	mustChangeCookie = "jv_must_change"
)

type API interface {
	Login(ctx context.Context, username, password string) (api.LoginResponse, error)
	Logout(ctx context.Context, session string) error
	ChangePassword(ctx context.Context, session, currentPassword, newPassword string) error
	Status(ctx context.Context, session string) (api.Status, error)
}

type Config struct {
	Version       string
	Commit        string
	ManagementAPI string
}

type App struct {
	api       API
	config    Config
	templates *template.Template
	static    http.Handler
}

type pageData struct {
	Title         string
	Version       string
	Commit        string
	ManagementAPI string
	Error         string
	CSRF          string
	Status        api.Status
}

func New(client API, cfg Config) (*App, error) {
	t, err := template.ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, err
	}
	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	return &App{api: client, config: cfg, templates: t, static: http.FileServer(http.FS(staticFS))}, nil
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", a.static))
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /login", a.loginPage)
	mux.HandleFunc("POST /login", a.login)
	mux.HandleFunc("POST /logout", a.logout)
	mux.HandleFunc("GET /password", a.passwordPage)
	mux.HandleFunc("POST /password", a.passwordChange)
	mux.HandleFunc("GET /", a.dashboard)
	return securityHeaders(mux)
}

func (a *App) ListenAndServeTLS(addr, certFile, keyFile string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           a.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return srv.ListenAndServeTLS(certFile, keyFile)
}

func (a *App) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (a *App) loginPage(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if _, err := a.api.Status(r.Context(), c.Value); err == nil {
			if mustChange(r) {
				http.Redirect(w, r, "/password", http.StatusSeeOther)
			} else {
				http.Redirect(w, r, "/", http.StatusSeeOther)
			}
			return
		}
	}
	a.render(w, "login.html", pageData{Title: "Sign in", Version: a.config.Version, ManagementAPI: a.config.ManagementAPI})
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	result, err := a.api.Login(r.Context(), username, password)
	if err != nil {
		status := http.StatusBadGateway
		msg := "Management service unavailable"
		if errors.Is(err, api.ErrUnauthorized) {
			status = http.StatusUnauthorized
			msg = "Invalid administrator credentials"
		}
		w.WriteHeader(status)
		a.render(w, "login.html", pageData{Title: "Sign in", Version: a.config.Version, ManagementAPI: a.config.ManagementAPI, Error: msg})
		return
	}
	if result.ManagementAPI != "" && result.ManagementAPI != a.config.ManagementAPI {
		http.Error(w, "management API compatibility mismatch", http.StatusServiceUnavailable)
		return
	}
	csrf, err := randomToken(32)
	if err != nil {
		http.Error(w, "could not create session", http.StatusInternalServerError)
		return
	}
	setCookie(w, sessionCookie, result.Session, true)
	setCookie(w, csrfCookie, csrf, false)
	if result.MustChange {
		setCookie(w, mustChangeCookie, "1", true)
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	clearCookie(w, mustChangeCookie, true)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if !validCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = a.api.Logout(r.Context(), c.Value)
	}
	clearSessionCookies(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (a *App) passwordPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := sessionFromRequest(r); !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	csrf := csrfFromRequest(r)
	a.render(w, "password.html", pageData{Title: "Change password", Version: a.config.Version, ManagementAPI: a.config.ManagementAPI, CSRF: csrf})
}

func (a *App) passwordChange(w http.ResponseWriter, r *http.Request) {
	if !validCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	session, ok := sessionFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")
	if newPassword != confirmPassword || len(newPassword) < 12 || len(newPassword) > 128 {
		w.WriteHeader(http.StatusBadRequest)
		a.render(w, "password.html", pageData{Title: "Change password", Version: a.config.Version, ManagementAPI: a.config.ManagementAPI, CSRF: csrfFromRequest(r), Error: "New password must match and be 12 to 128 characters."})
		return
	}
	if err := a.api.ChangePassword(r.Context(), session, currentPassword, newPassword); err != nil {
		if errors.Is(err, api.ErrUnauthorized) {
			clearSessionCookies(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		a.render(w, "password.html", pageData{Title: "Change password", Version: a.config.Version, ManagementAPI: a.config.ManagementAPI, CSRF: csrfFromRequest(r), Error: "Password change was rejected."})
		return
	}
	clearCookie(w, mustChangeCookie, true)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if mustChange(r) {
		http.Redirect(w, r, "/password", http.StatusSeeOther)
		return
	}
	session, ok := sessionFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	status, err := a.api.Status(r.Context(), session)
	if err != nil {
		if errors.Is(err, api.ErrUnauthorized) {
			clearSessionCookies(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		http.Error(w, "management service unavailable", http.StatusBadGateway)
		return
	}
	a.render(w, "dashboard.html", pageData{Title: "Dashboard", Version: a.config.Version, Commit: a.config.Commit, ManagementAPI: a.config.ManagementAPI, CSRF: csrfFromRequest(r), Status: status})
}

func (a *App) render(w http.ResponseWriter, name string, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := a.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func sessionFromRequest(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return "", false
	}
	return c.Value, true
}

func csrfFromRequest(r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil {
		return c.Value
	}
	return ""
}

func mustChange(r *http.Request) bool {
	c, err := r.Cookie(mustChangeCookie)
	return err == nil && c.Value == "1"
}

func randomToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func setCookie(w http.ResponseWriter, name, value string, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", Secure: true, HttpOnly: httpOnly, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
}

func clearCookie(w http.ResponseWriter, name string, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", Secure: true, HttpOnly: httpOnly, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

func clearSessionCookies(w http.ResponseWriter) {
	clearCookie(w, sessionCookie, true)
	clearCookie(w, csrfCookie, false)
	clearCookie(w, mustChangeCookie, true)
}

func validCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookie)
	if err != nil || cookie.Value == "" || r.FormValue("csrf") != cookie.Value {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host != r.Host {
		return false
	}
	return u.Scheme == "https"
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self'; form-action 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
