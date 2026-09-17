package server

import (
	"crypto/sha256"
	"net/http"
	"sync"
	"time"

	"github.com/home-server-project/justvoxel-webui/internal/api"
)

const setupDraftLifetime = 24 * time.Hour

type setupDraftKey struct {
	app     *App
	session [32]byte
}

type setupDraft struct {
	Started     bool
	CurrentStep int
	UpdatedAt   time.Time
}

type setupDraftStore struct {
	mu     sync.Mutex
	drafts map[setupDraftKey]setupDraft
}

var firstRunSetupDrafts = setupDraftStore{drafts: make(map[setupDraftKey]setupDraft)}

type setupWizardStepView struct {
	Number      int
	Name        string
	Description string
	Active      bool
	Complete    bool
}

type setupWizardPageData struct {
	Title         string
	Version       string
	ManagementAPI string
	CSRF          string
	Identity      api.SessionInfo
	Started       bool
	CurrentStep   int
	Current       setupWizardStepView
	Steps         []setupWizardStepView
	CanBack       bool
	CanNext       bool
}

var setupWizardSteps = []setupWizardStepView{
	{Number: 1, Name: "Server", Description: "Server identity, player limit and Bedrock choice will be configured here."},
	{Number: 2, Name: "Minecraft", Description: "Minecraft memory, ports and version policy will be configured here."},
	{Number: 3, Name: "Storage", Description: "Minecraft data storage will be selected here."},
	{Number: 4, Name: "Backups", Description: "Backup location, retention and schedule will be configured here."},
	{Number: 5, Name: "Review", Description: "The complete setup plan and Minecraft EULA acceptance will be reviewed here before any changes are applied."},
}

func (a *App) registerSetupWizardRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /setup", a.setupWizardPage)
	mux.HandleFunc("POST /setup/start", a.setupWizardStart)
	mux.HandleFunc("POST /setup/navigate", a.setupWizardNavigate)
	mux.HandleFunc("POST /setup/cancel", a.setupWizardCancel)
}

func (a *App) setupWizardPage(w http.ResponseWriter, r *http.Request) {
	session, _, identity, ok := a.setupWizardRequest(w, r, false)
	if !ok {
		return
	}
	draft, exists := firstRunSetupDrafts.get(a, session)
	if !exists {
		draft = setupDraft{}
	}
	a.renderSetupWizard(w, identity, draft, csrfFromRequest(r))
}

func (a *App) setupWizardStart(w http.ResponseWriter, r *http.Request) {
	session, _, _, ok := a.setupWizardRequest(w, r, true)
	if !ok {
		return
	}
	firstRunSetupDrafts.start(a, session)
	http.Redirect(w, r, "/setup", http.StatusSeeOther)
}

func (a *App) setupWizardNavigate(w http.ResponseWriter, r *http.Request) {
	session, _, _, ok := a.setupWizardRequest(w, r, true)
	if !ok {
		return
	}
	direction := r.FormValue("direction")
	if direction != "next" && direction != "back" {
		http.Error(w, "invalid setup navigation", http.StatusBadRequest)
		return
	}
	if !firstRunSetupDrafts.navigate(a, session, direction) {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/setup", http.StatusSeeOther)
}

func (a *App) setupWizardCancel(w http.ResponseWriter, r *http.Request) {
	session, _, _, ok := a.setupWizardRequest(w, r, true)
	if !ok {
		return
	}
	firstRunSetupDrafts.delete(a, session)
	http.Redirect(w, r, "/setup", http.StatusSeeOther)
}

func (a *App) setupWizardRequest(w http.ResponseWriter, r *http.Request, requireCSRF bool) (string, adminDiscoveryAPI, api.SessionInfo, bool) {
	if requireCSRF && !a.validCSRF(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return "", nil, api.SessionInfo{}, false
	}
	session, client, identity, ok := a.adminDiscoveryRequest(w, r)
	if !ok {
		return "", nil, api.SessionInfo{}, false
	}
	configuration, err := client.AdminConfiguration(r.Context(), session)
	if err != nil {
		a.handleAdminDiscoveryError(w, r, err)
		return "", nil, api.SessionInfo{}, false
	}
	if configuration.Configured {
		firstRunSetupDrafts.delete(a, session)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return "", nil, api.SessionInfo{}, false
	}
	return session, client, identity, true
}

func (a *App) renderSetupWizard(w http.ResponseWriter, identity api.SessionInfo, draft setupDraft, csrf string) {
	steps := make([]setupWizardStepView, len(setupWizardSteps))
	copy(steps, setupWizardSteps)
	current := setupWizardStepView{}
	if draft.Started {
		if draft.CurrentStep < 1 || draft.CurrentStep > len(steps) {
			draft.CurrentStep = 1
		}
		for i := range steps {
			steps[i].Active = steps[i].Number == draft.CurrentStep
			steps[i].Complete = steps[i].Number < draft.CurrentStep
			if steps[i].Active {
				current = steps[i]
			}
		}
	}
	data := setupWizardPageData{
		Title: "First setup", Version: a.config.Version, ManagementAPI: a.config.ManagementAPI,
		CSRF: csrf, Identity: identity, Started: draft.Started, CurrentStep: draft.CurrentStep,
		Current: current, Steps: steps,
		CanBack: draft.Started && draft.CurrentStep > 1,
		CanNext: draft.Started && draft.CurrentStep < len(steps),
	}
	a.renderAdminDiscovery(w, "setup_wizard.html", data)
}

func setupKey(app *App, session string) setupDraftKey {
	return setupDraftKey{app: app, session: sha256.Sum256([]byte(session))}
}

func (s *setupDraftStore) pruneLocked(now time.Time) {
	for key, draft := range s.drafts {
		if now.Sub(draft.UpdatedAt) > setupDraftLifetime {
			delete(s.drafts, key)
		}
	}
}

func (s *setupDraftStore) get(app *App, session string) (setupDraft, bool) {
	now := time.Now()
	key := setupKey(app, session)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	draft, ok := s.drafts[key]
	return draft, ok
}

func (s *setupDraftStore) start(app *App, session string) {
	now := time.Now()
	key := setupKey(app, session)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	s.drafts[key] = setupDraft{Started: true, CurrentStep: 1, UpdatedAt: now}
}

func (s *setupDraftStore) navigate(app *App, session, direction string) bool {
	now := time.Now()
	key := setupKey(app, session)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	draft, ok := s.drafts[key]
	if !ok || !draft.Started {
		return false
	}
	switch direction {
	case "next":
		if draft.CurrentStep < len(setupWizardSteps) {
			draft.CurrentStep++
		}
	case "back":
		if draft.CurrentStep > 1 {
			draft.CurrentStep--
		}
	}
	draft.UpdatedAt = now
	s.drafts[key] = draft
	return true
}

func (s *setupDraftStore) delete(app *App, session string) {
	key := setupKey(app, session)
	s.mu.Lock()
	delete(s.drafts, key)
	s.mu.Unlock()
}
