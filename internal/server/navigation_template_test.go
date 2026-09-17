package server

import (
	"strings"
	"testing"
)

func TestDashboardHeaderNavigationUX(t *testing.T) {
	dashboard, err := assets.ReadFile("templates/dashboard.html")
	if err != nil {
		t.Fatal(err)
	}
	markup := string(dashboard)
	for _, want := range []string{
		`class="brand-link" href="/"`,
		`aria-label="JustVoxel dashboard"`,
		`aria-current="page"`,
		`aria-label="Primary navigation"`,
		`href="/activity"`,
		`href="/operations"`,
		`href="/settings/activity"`,
		`href="/settings/users"`,
		`href="/settings/authentication"`,
		`href="/password"`,
	} {
		if !strings.Contains(markup, want) {
			t.Fatalf("dashboard header missing %q", want)
		}
	}

	css, err := assets.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	styles := string(css)
	for _, want := range []string{
		`.brand-link`,
		`header nav>a:hover`,
		`transform:scale(1.05)`,
		`header nav>a[aria-current="page"]`,
		`header nav form button:hover`,
		`:focus-visible`,
	} {
		if !strings.Contains(styles, want) {
			t.Fatalf("navigation styling missing %q", want)
		}
	}
}
