package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetupWizardShowsWelcomeForUnconfiguredAdministrator(t *testing.T) {
	client := &fakeDiscoveryAPI{}
	app, err := New(client, Config{Version: "test", ManagementAPI: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	defer firstRunSetupDrafts.delete(app, "session-token")

	rr := httptestResponse(app, authenticatedAdminRequest(http.MethodGet, "http://example/setup", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("setup welcome returned %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"Welcome to JustVoxel", "Start setup", "Safe to explore", "Server", "Minecraft", "Storage", "Backups", "Review", "mjust setup",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("setup welcome missing %q: %s", want, body)
		}
	}
	if client.configurationHit != 1 {
		t.Fatalf("configuration discovery calls = %d, want 1", client.configurationHit)
	}
}

func TestSetupWizardDraftSurvivesNavigationAndRefresh(t *testing.T) {
	client := &fakeDiscoveryAPI{}
	app, err := New(client, Config{Version: "test", ManagementAPI: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	defer firstRunSetupDrafts.delete(app, "session-token")

	start := httptestResponse(app, authenticatedAdminRequest(http.MethodPost, "http://example/setup/start", "csrf=csrf-token"))
	if start.Code != http.StatusSeeOther || start.Header().Get("Location") != "/setup" {
		t.Fatalf("start returned %d %q", start.Code, start.Header().Get("Location"))
	}

	first := httptestResponse(app, authenticatedAdminRequest(http.MethodGet, "http://example/setup", ""))
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), "Step 1 of 5") || !strings.Contains(first.Body.String(), "Server configuration") {
		t.Fatalf("step one not preserved: %d %s", first.Code, first.Body.String())
	}

	next := httptestResponse(app, authenticatedAdminRequest(http.MethodPost, "http://example/setup/navigate", "csrf=csrf-token&direction=next"))
	if next.Code != http.StatusSeeOther {
		t.Fatalf("next returned %d", next.Code)
	}

	refreshed := httptestResponse(app, authenticatedAdminRequest(http.MethodGet, "http://example/setup", ""))
	if refreshed.Code != http.StatusOK || !strings.Contains(refreshed.Body.String(), "Step 2 of 5") || !strings.Contains(refreshed.Body.String(), "Minecraft configuration") {
		t.Fatalf("step two not preserved after refresh: %d %s", refreshed.Code, refreshed.Body.String())
	}
}

func TestSetupWizardCancelDiscardsDraft(t *testing.T) {
	client := &fakeDiscoveryAPI{}
	app, err := New(client, Config{Version: "test", ManagementAPI: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	defer firstRunSetupDrafts.delete(app, "session-token")

	_ = httptestResponse(app, authenticatedAdminRequest(http.MethodPost, "http://example/setup/start", "csrf=csrf-token"))
	cancel := httptestResponse(app, authenticatedAdminRequest(http.MethodPost, "http://example/setup/cancel", "csrf=csrf-token"))
	if cancel.Code != http.StatusSeeOther || cancel.Header().Get("Location") != "/setup" {
		t.Fatalf("cancel returned %d %q", cancel.Code, cancel.Header().Get("Location"))
	}

	welcome := httptestResponse(app, authenticatedAdminRequest(http.MethodGet, "http://example/setup", ""))
	if welcome.Code != http.StatusOK || !strings.Contains(welcome.Body.String(), "Welcome to JustVoxel") {
		t.Fatalf("cancel did not discard draft: %d %s", welcome.Code, welcome.Body.String())
	}
	if strings.Contains(welcome.Body.String(), "Step 1 of 5") {
		t.Fatal("cancelled draft still rendered as active setup")
	}
}

func TestSetupWizardRejectsMutationWithoutCSRF(t *testing.T) {
	client := &fakeDiscoveryAPI{}
	app, err := New(client, Config{Version: "test", ManagementAPI: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	defer firstRunSetupDrafts.delete(app, "session-token")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://example/setup/start", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: "session-token"})
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("setup start without CSRF status = %d, want 403", rr.Code)
	}
	if _, ok := firstRunSetupDrafts.get(app, "session-token"); ok {
		t.Fatal("setup draft was created without CSRF")
	}
}

func TestSetupWizardIsAdministratorOnly(t *testing.T) {
	client := &fakeDiscoveryAPI{role: "operator"}
	app, err := New(client, Config{Version: "test", ManagementAPI: "v1"})
	if err != nil {
		t.Fatal(err)
	}

	rr := httptestResponse(app, authenticatedAdminRequest(http.MethodGet, "http://example/setup", ""))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operator setup status = %d, want 403", rr.Code)
	}
	if client.configurationHit != 0 {
		t.Fatalf("privileged configuration discovery ran for operator %d times", client.configurationHit)
	}
}

func TestConfiguredApplianceCannotEnterFirstRunWizard(t *testing.T) {
	client := &fakeDiscoveryAPI{}
	client.configuration.Configured = true
	app, err := New(client, Config{Version: "test", ManagementAPI: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	defer firstRunSetupDrafts.delete(app, "session-token")

	rr := httptestResponse(app, authenticatedAdminRequest(http.MethodGet, "http://example/setup", ""))
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/" {
		t.Fatalf("configured setup returned %d %q", rr.Code, rr.Header().Get("Location"))
	}
}

func TestDashboardIncludesFirstRunRedirectForUnconfiguredAdministrator(t *testing.T) {
	dashboard, err := assets.ReadFile("templates/dashboard.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dashboard), `/static/first-run.js`) {
		t.Fatal("dashboard does not load first-run detection")
	}

	script, err := assets.ReadFile("static/first-run.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(script)
	for _, want := range []string{`role !== "administrator"`, `configured !== false`, `window.location.replace("/setup")`} {
		if !strings.Contains(content, want) {
			t.Fatalf("first-run redirect missing %q", want)
		}
	}
}
