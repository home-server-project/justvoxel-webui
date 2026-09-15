package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/home-server-project/justvoxel-webui/internal/api"
)

type fakeAPI struct{ authenticated bool }

func (f *fakeAPI) Login(_ context.Context, username, password string) (api.LoginResponse, error) {
	if username == "admin" && password == "secret" {
		f.authenticated = true
		return api.LoginResponse{Session: "session-token", ManagementAPI: "v1"}, nil
	}
	return api.LoginResponse{}, api.ErrUnauthorized
}
func (f *fakeAPI) Logout(_ context.Context, _ string) error { f.authenticated = false; return nil }
func (f *fakeAPI) Status(_ context.Context, session string) (api.Status, error) {
	if session != "session-token" { return api.Status{}, api.ErrUnauthorized }
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
	if err != nil { t.Fatal(err) }
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "https://example/", nil)
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther { t.Fatalf("got %d", rr.Code) }
	if rr.Header().Get("Location") != "/login" { t.Fatalf("unexpected redirect %q", rr.Header().Get("Location")) }
}

func TestLoginSetsSecureSession(t *testing.T) {
	app, err := New(&fakeAPI{}, Config{Version: "1.0.0", ManagementAPI: "v1"})
	if err != nil { t.Fatal(err) }
	form := strings.NewReader("username=admin&password=secret")
	req := httptest.NewRequest(http.MethodPost, "https://example/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther { t.Fatalf("got %d", rr.Code) }
	found := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == sessionCookie {
			found = true
			if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode { t.Fatalf("session cookie is not hardened: %#v", c) }
		}
	}
	if !found { t.Fatal("session cookie not set") }
}

func TestDashboardRendersStatus(t *testing.T) {
	app, err := New(&fakeAPI{}, Config{Version: "1.0.0", Commit: "abc123", ManagementAPI: "v1"})
	if err != nil { t.Fatal(err) }
	req := httptest.NewRequest(http.MethodGet, "https://example/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session-token"})
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK { t.Fatalf("got %d", rr.Code) }
	body := rr.Body.String()
	for _, want := range []string{"Running", "2 / 10", "10-test", "1.0.0"} {
		if !strings.Contains(body, want) { t.Fatalf("missing %q in response", want) }
	}
}
