package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/github"
	"github.com/itswskkk/KMJG-Hub/server/internal/github/githubtest"
	"github.com/itswskkk/KMJG-Hub/server/internal/httpapi"
	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

const testWebhookSecret = "test-webhook-secret"

var testGitHubRepo = github.AvailableRepository{ExternalID: 42, OwnerLogin: "octo", Name: "hub", HTMLURL: "https://github.com/octo/hub", DefaultBranch: "main"}

type githubEnv struct {
	router    http.Handler
	handlers  *httpapi.Handlers
	projects  *fakeProjectRepo
	users     map[string]friendUser
	projectID string
}

// newGitHubEnv registers owner/admin/member/outsider, creates a Project
// owned by "owner" with admin and member added, and gives each of them a
// fake GitHub account that can see testGitHubRepo.
func newGitHubEnv(t *testing.T) *githubEnv {
	t.Helper()
	router, handlers, projects := newTestRouterWithHandlers()
	env := &githubEnv{router: router, handlers: handlers, projects: projects, users: map[string]friendUser{}}
	client := handlers.GitHub.Client.(*githubtest.Client)
	for i, name := range []string{"owner", "admin", "member", "outsider"} {
		token := registerAndToken(t, router, name)
		env.users[name] = friendUser{token: token, id: currentUserID(t, router, token)}
		client.AddAccount("code-"+name, githubtest.Account{ID: int64(i + 1), Login: name + "-gh", Repos: []github.AvailableRepository{testGitHubRepo}})
	}
	rec := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Git Project"}, env.users["owner"].token)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, rec, &p)
	env.projectID = p.ID
	projects.addMember(p.ID, env.users["admin"].id, project.RoleAdmin)
	projects.addMember(p.ID, env.users["member"].id, project.RoleMember)
	return env
}

func (e *githubEnv) authorize(t *testing.T, name string) string {
	t.Helper()
	rec := doJSON(t, e.router, http.MethodGet, "/api/v1/auth/github/authorize", nil, e.users[name].token)
	if rec.Code != http.StatusOK {
		t.Fatalf("authorize: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		AuthorizeURL string `json:"authorize_url"`
		State        string `json:"state"`
	}
	decodeJSON(t, rec, &body)
	if body.State == "" || !strings.Contains(body.AuthorizeURL, url.QueryEscape(body.State)) {
		t.Fatalf("authorize body: %+v", body)
	}
	return body.State
}

func (e *githubEnv) callback(t *testing.T, code, state string) *httptest.ResponseRecorder {
	t.Helper()
	// Deliberately no Authorization header: this is the system browser.
	return doJSON(t, e.router, http.MethodGet, "/api/v1/auth/github/callback?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(state), nil, "")
}

func (e *githubEnv) connectAccount(t *testing.T, name string) {
	t.Helper()
	rec := e.callback(t, "code-"+name, e.authorize(t, name))
	if rec.Code != http.StatusOK {
		t.Fatalf("callback for %s: %d %s", name, rec.Code, rec.Body.String())
	}
}

func githubStatus(t *testing.T, e *githubEnv, name string) map[string]any {
	t.Helper()
	rec := doJSON(t, e.router, http.MethodGet, "/api/v1/auth/github", nil, e.users[name].token)
	expectStatus(t, rec, http.StatusOK, "status")
	var body map[string]any
	decodeJSON(t, rec, &body)
	return body
}

func TestGitHubEndpointsRequireAuth(t *testing.T) {
	router := newTestRouter()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/auth/github"},
		{http.MethodGet, "/api/v1/auth/github/authorize"},
		{http.MethodDelete, "/api/v1/auth/github"},
		{http.MethodGet, "/api/v1/github/repositories"},
		{http.MethodPost, "/api/v1/projects/x/github/connect"},
		{http.MethodGet, "/api/v1/projects/x/github/info"},
		{http.MethodDelete, "/api/v1/projects/x/github/disconnect"},
		{http.MethodPut, "/api/v1/projects/x/github/notifications"},
	} {
		if rec := doJSON(t, router, tc.method, tc.path, nil, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401, got %d", tc.method, tc.path, rec.Code)
		}
	}
}

func TestGitHubOAuthConnectAndDisconnect(t *testing.T) {
	e := newGitHubEnv(t)
	if s := githubStatus(t, e, "owner"); s["configured"] != true || s["connected"] != false {
		t.Fatalf("initial status: %v", s)
	}
	expectStatus(t, doJSON(t, e.router, http.MethodGet, "/api/v1/github/repositories", nil, e.users["owner"].token), http.StatusConflict, "list before connect")

	state := e.authorize(t, "owner")
	// Tampered state is rejected and links nothing.
	rec := e.callback(t, "code-owner", state+"00")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("tampered callback: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if s := githubStatus(t, e, "owner"); s["connected"] != false {
		t.Fatalf("tampered state linked an account: %v", s)
	}

	rec = e.callback(t, "code-owner", state)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "owner-gh") {
		t.Fatalf("callback: %d %s", rec.Code, rec.Body.String())
	}
	if s := githubStatus(t, e, "owner"); s["connected"] != true || s["login"] != "owner-gh" {
		t.Fatalf("status after connect: %v", s)
	}

	// The same GitHub account cannot be linked to another KMJG Hub user.
	expectStatus(t, e.callback(t, "code-owner", e.authorize(t, "member")), http.StatusConflict, "identity in use")

	rec = doJSON(t, e.router, http.MethodGet, "/api/v1/github/repositories", nil, e.users["owner"].token)
	expectStatus(t, rec, http.StatusOK, "list repos")
	var list struct {
		Repositories []map[string]any `json:"repositories"`
	}
	decodeJSON(t, rec, &list)
	if len(list.Repositories) != 1 || list.Repositories[0]["full_name"] != "octo/hub" || list.Repositories[0]["external_repo_id"] != float64(42) {
		t.Fatalf("repositories: %v", list)
	}

	expectStatus(t, doJSON(t, e.router, http.MethodDelete, "/api/v1/auth/github", nil, e.users["owner"].token), http.StatusNoContent, "disconnect")
	if s := githubStatus(t, e, "owner"); s["connected"] != false {
		t.Fatalf("status after disconnect: %v", s)
	}
}

func TestGitHubProjectRepositoryLifecycle(t *testing.T) {
	e := newGitHubEnv(t)
	for _, name := range []string{"owner", "admin", "member", "outsider"} {
		e.connectAccount(t, name)
	}
	base := "/api/v1/projects/" + e.projectID + "/github/"
	connect := map[string]int64{"external_repo_id": 42}

	rec := doJSON(t, e.router, http.MethodGet, base+"info", nil, e.users["member"].token)
	expectStatus(t, rec, http.StatusOK, "info before connect")
	if !strings.Contains(rec.Body.String(), `"repository":null`) {
		t.Fatalf("expected null repository: %s", rec.Body.String())
	}
	expectStatus(t, doJSON(t, e.router, http.MethodGet, base+"info", nil, e.users["outsider"].token), http.StatusNotFound, "outsider info")

	expectStatus(t, doJSON(t, e.router, http.MethodPost, base+"connect", connect, e.users["member"].token), http.StatusForbidden, "member connect")
	expectStatus(t, doJSON(t, e.router, http.MethodPost, base+"connect", connect, e.users["outsider"].token), http.StatusNotFound, "outsider connect")
	expectStatus(t, doJSON(t, e.router, http.MethodPost, base+"connect", map[string]int64{"external_repo_id": 7}, e.users["admin"].token), http.StatusForbidden, "inaccessible repo")
	expectStatus(t, doJSON(t, e.router, http.MethodPost, base+"connect", map[string]int64{}, e.users["admin"].token), http.StatusBadRequest, "missing repo id")

	rec = doJSON(t, e.router, http.MethodPost, base+"connect", connect, e.users["admin"].token)
	expectStatus(t, rec, http.StatusCreated, "admin connect")
	var repo map[string]any
	decodeJSON(t, rec, &repo)
	if repo["full_name"] != "octo/hub" || repo["default_branch"] != "main" || repo["project_id"] != e.projectID || repo["connected_by_user_id"] != e.users["admin"].id {
		t.Fatalf("connected repo: %v", repo)
	}
	expectStatus(t, doJSON(t, e.router, http.MethodPost, base+"connect", connect, e.users["owner"].token), http.StatusConflict, "second connect")

	rec = doJSON(t, e.router, http.MethodGet, base+"info", nil, e.users["member"].token)
	expectStatus(t, rec, http.StatusOK, "member info")
	if !strings.Contains(rec.Body.String(), `"full_name":"octo/hub"`) {
		t.Fatalf("member info: %s", rec.Body.String())
	}

	notif := map[string]bool{"post_to_chat": true, "all_members": false, "all_branches": false}
	expectStatus(t, doJSON(t, e.router, http.MethodPut, base+"notifications", notif, e.users["admin"].token), http.StatusForbidden, "admin notification config")
	expectStatus(t, doJSON(t, e.router, http.MethodPut, base+"notifications", map[string]bool{"post_to_chat": true}, e.users["owner"].token), http.StatusBadRequest, "partial config")
	rec = doJSON(t, e.router, http.MethodPut, base+"notifications", notif, e.users["owner"].token)
	expectStatus(t, rec, http.StatusOK, "owner notification config")
	decodeJSON(t, rec, &repo)
	if repo["post_pushes_to_chat"] != true || repo["notify_all_members"] != false || repo["notify_all_branches"] != false {
		t.Fatalf("config: %v", repo)
	}

	expectStatus(t, doJSON(t, e.router, http.MethodDelete, base+"disconnect", nil, e.users["member"].token), http.StatusForbidden, "member disconnect")
	expectStatus(t, doJSON(t, e.router, http.MethodDelete, base+"disconnect", nil, e.users["owner"].token), http.StatusNoContent, "owner disconnect")
	expectStatus(t, doJSON(t, e.router, http.MethodDelete, base+"disconnect", nil, e.users["owner"].token), http.StatusNotFound, "repeat disconnect")
}

func postWebhook(router http.Handler, event string, payload []byte, signature string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/github/webhooks", bytes.NewReader(payload))
	req.Header.Set("X-GitHub-Event", event)
	if signature != "" {
		req.Header.Set("X-Hub-Signature-256", signature)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestGitHubWebhook(t *testing.T) {
	e := newGitHubEnv(t)
	e.connectAccount(t, "owner")
	expectStatus(t, doJSON(t, e.router, http.MethodPost, "/api/v1/projects/"+e.projectID+"/github/connect", map[string]int64{"external_repo_id": 42}, e.users["owner"].token), http.StatusCreated, "connect")

	payload, _ := json.Marshal(map[string]any{
		"ref": "refs/heads/main", "repository": map[string]any{"id": 42},
		"sender":  map[string]any{"login": "owner-gh"},
		"commits": []map[string]any{{"message": "Initial commit\n\nbody"}},
	})
	expectStatus(t, postWebhook(e.router, "push", payload, ""), http.StatusUnauthorized, "unsigned")
	expectStatus(t, postWebhook(e.router, "push", payload, githubtest.Sign("wrong", payload)), http.StatusUnauthorized, "wrong secret")
	tampered := bytes.Replace(payload, []byte("Initial"), []byte("Evil"), 1)
	expectStatus(t, postWebhook(e.router, "push", tampered, githubtest.Sign(testWebhookSecret, payload)), http.StatusUnauthorized, "tampered")
	if page := listNotifications(t, e.router, e.users["member"], ""); len(page.Notifications) != 0 {
		t.Fatalf("rejected webhook produced notifications: %+v", page)
	}

	expectStatus(t, postWebhook(e.router, "ping", []byte(`{}`), githubtest.Sign(testWebhookSecret, []byte(`{}`))), http.StatusNoContent, "ping")
	expectStatus(t, postWebhook(e.router, "push", payload, githubtest.Sign(testWebhookSecret, payload)), http.StatusNoContent, "valid push")

	for _, name := range []string{"owner", "admin", "member"} {
		page := listNotifications(t, e.router, e.users[name], "")
		if len(page.Notifications) != 1 || page.Notifications[0].EventType != "git_push" {
			t.Fatalf("%s notifications: %+v", name, page)
		}
		var p map[string]any
		_ = json.Unmarshal(page.Notifications[0].Payload, &p)
		if p["branch"] != "main" || p["repository"] != "octo/hub" || p["commit_count"] != float64(1) {
			t.Fatalf("%s payload: %v", name, p)
		}
	}
	if page := listNotifications(t, e.router, e.users["outsider"], ""); len(page.Notifications) != 0 {
		t.Fatalf("outsider notified: %+v", page)
	}
}

func TestGitHubNotConfigured(t *testing.T) {
	e := newGitHubEnv(t)
	e.handlers.GitHub.Client = nil
	if s := githubStatus(t, e, "owner"); s["configured"] != false {
		t.Fatalf("status: %v", s)
	}
	rec := doJSON(t, e.router, http.MethodGet, "/api/v1/auth/github/authorize", nil, e.users["owner"].token)
	expectStatus(t, rec, http.StatusServiceUnavailable, "authorize")
	if !strings.Contains(rec.Body.String(), "github_not_configured") {
		t.Fatalf("body: %s", rec.Body.String())
	}
	expectStatus(t, postWebhook(e.router, "push", []byte(`{}`), "sha256=00"), http.StatusServiceUnavailable, "webhook")
	expectStatus(t, doJSON(t, e.router, http.MethodGet, "/api/v1/projects/"+e.projectID+"/github/info", nil, e.users["member"].token), http.StatusOK, "info still works")
}
