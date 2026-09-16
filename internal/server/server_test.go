package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/home-server-project/justvoxel-webui/internal/api"
)

type fakeAPI struct {
	authenticated bool
	mustChange    bool
}

func (f *fakeAPI) Login(_ context.Context, username, password string) (api.LoginResponse, error) {
	if username == "admin" && password == "secret" {
		f.authenticated = true
		return api.LoginResponse{Session: "session-token", ManagementAPI: "v1", MustChange: f.mustChange}, nil
	}
	return api.LoginResponse{}, api.ErrUnauthorized
}
func (f *fakeAPI) Logout(_ context.Context, _ string) error { f.authenticated = false; return nil }
func (f *fakeAPI) ChangePassword(_ context.Context, session, currentPassword, newPassword string) error {
	if session != "session-token" || currentPassword != "secret" || len(newPassword) < 12 {
		return api.ErrUnauthorized
	}
	f.mustChange = false
	return nil
}
func (f *fakeAPI) Status(_ context.Context, session string) (api.Status, error) {
	if session != "session-token" {
		return api.Status{}, api.ErrUnauthorized
	}
	var s api.Status
	s.System.Variant = "VM"
	s.System.Health = "Healthy"
	s.Minecraft.State = "Running"
	s.Minecraft.Players = 2
	s.Minecraft.MaxPlayers = 10
	s.JustVoxel.Version = "10-test"
	return s, nil
}

func TestUnauthenticatedDashboardRedirects(t *testing.T) {
	app, err := New(&fakeAPI{}, Config{Version: "1.0.0", ManagementAPI: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example/", nil)
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("got %d", rr.Code)
	}
	if rr.Header().Get("Location") != "/login" {
		t.Fatalf("unexpected redirect %q", rr.Header().Get("Location"))
	}
}

func TestLoginSetsLocalHTTPSession(t *testing.T) {
	app, err := New(&fakeAPI{}, Config{Version: "1.0.0", ManagementAPI: "v1", ExternalScheme: "http"})
	if err != nil {
		t.Fatal(err)
	}
	form := strings.NewReader("username=admin&password=secret")
	req := httptest.NewRequest(http.MethodPost, "http://example/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("got %d", rr.Code)
	}
	found := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == sessionCookie {
			found = true
			if c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
				t.Fatalf("local HTTP session cookie has unexpected security attributes: %#v", c)
			}
		}
	}
	if !found {
		t.Fatal("session cookie not set")
	}
}

func TestCSRFAcceptsSameHTTPOrigin(t *testing.T) {
	app, err := New(&fakeAPI{}, Config{ExternalScheme: "http"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://example/logout", strings.NewReader("csrf=token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://example")
	req.AddCookie(&http.Cookie{Name: csrfCookie, Value: "token"})
	if !app.validCSRF(req) {
		t.Fatal("same-origin HTTP CSRF token was rejected")
	}
}

func TestCSRFAcceptsNullOriginWithValidToken(t *testing.T) {
	app, err := New(&fakeAPI{}, Config{ExternalScheme: "http"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://example/logout", strings.NewReader("csrf=token"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "null")
	req.AddCookie(&http.Cookie{Name: csrfCookie, Value: "token"})
	if !app.validCSRF(req) {
		t.Fatal("null Origin with valid CSRF token was rejected")
	}
}

func TestCSRFRejectsForeignOrigin(t *testing.T) {
	app, err := New(&fakeAPI{}, Config{ExternalScheme: "http"})
	if err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"https://example", "http://other.example"} {
		req := httptest.NewRequest(http.MethodPost, "http://example/logout", strings.NewReader("csrf=token"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", origin)
		req.AddCookie(&http.Cookie{Name: csrfCookie, Value: "token"})
		if app.validCSRF(req) {
			t.Fatalf("foreign Origin %q was accepted", origin)
		}
	}
}

func TestCSRFRejectsMissingOrWrongToken(t *testing.T) {
	app, err := New(&fakeAPI{}, Config{ExternalScheme: "http"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		form       string
		cookieValue string
	}{
		{name: "missing form token", form: "", cookieValue: "token"},
		{name: "wrong form token", form: "csrf=wrong", cookieValue: "token"},
		{name: "missing cookie", form: "csrf=token", cookieValue: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://example/logout", strings.NewReader(tc.form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", "null")
			if tc.cookieValue != "" {
				req.AddCookie(&http.Cookie{Name: csrfCookie, Value: tc.cookieValue})
			}
			if app.validCSRF(req) {
				t.Fatal("invalid CSRF request was accepted")
			}
		})
	}
}

func TestFirstLoginRequiresPasswordChange(t *testing.T) {
	app, err := New(&fakeAPI{mustChange: true}, Config{Version: "1.0.0", ManagementAPI: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	form := strings.NewReader("username=admin&password=secret")
	req := httptest.NewRequest(http.MethodPost, "http://example/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/password" {
		t.Fatalf("expected password redirect, got %d %q", rr.Code, rr.Header().Get("Location"))
	}
}

func TestDashboardRendersStatus(t *testing.T) {
	app, err := New(&fakeAPI{}, Config{Version: "1.0.0", Commit: "abc123", ManagementAPI: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://example/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session-token"})
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"Running", "2 / 10", "10-test", "1.0.0"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in response", want)
		}
	}
}
