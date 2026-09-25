package httpapi_test

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/profile"
)

// fakeProfileFields holds the mutable, non-relational parts of a user's
// profile: everything internal/store/postgres.ProfileRepository would keep
// on the users table plus its own live-context derivation.
type fakeProfileFields struct {
	displayName, avatar, bio                   *string
	currentProject, currentTask, currentBranch *string
	repositories                               []string
}

// fakeProfileRepo is a minimal in-memory profile.Repository, mirroring
// fakeChatRepo/fakeProjectRepo's pattern: it reads usernames/emails from
// the shared fakeUserRepo rather than duplicating them, and lets tests
// directly control "live" state (current task/branch, online) that a real
// deployment would derive from internal/task and internal/realtime.
type fakeProfileRepo struct {
	mu      sync.Mutex
	users   *fakeUserRepo
	fields  map[string]*fakeProfileFields
	privacy map[string]map[profile.Field]profile.Audience
	online  map[string]bool
}

func newFakeProfileRepo(users *fakeUserRepo) *fakeProfileRepo {
	return &fakeProfileRepo{
		users:   users,
		fields:  make(map[string]*fakeProfileFields),
		privacy: make(map[string]map[profile.Field]profile.Audience),
		online:  make(map[string]bool),
	}
}

// setOnline lets a test control what GetProfile reports as Presence,
// standing in for a real internal/realtime.Hub connection.
func (f *fakeProfileRepo) setOnline(userID string, online bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.online[userID] = online
}

// setLiveContext lets a test set the "current task/project/branch" fields
// GetProfile reports, standing in for a real internal/task current-task +
// internal/workcontext branch lookup.
func (f *fakeProfileRepo) setLiveContext(userID string, project, task, branch *string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	flds := f.fieldsFor(userID)
	flds.currentProject, flds.currentTask, flds.currentBranch = project, task, branch
}

func (f *fakeProfileRepo) fieldsFor(userID string) *fakeProfileFields {
	flds, ok := f.fields[userID]
	if !ok {
		flds = &fakeProfileFields{repositories: []string{}}
		f.fields[userID] = flds
	}
	return flds
}

func (f *fakeProfileRepo) GetProfile(_ any, userID string) (*profile.Profile, error) {
	u, err := f.users.GetByID(context.Background(), userID)
	if err != nil {
		return nil, profile.ErrNotFound
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	flds := f.fieldsFor(userID)

	presence := "offline"
	if f.online[userID] {
		presence = "online"
	}

	return &profile.Profile{
		UserID: u.ID, Username: u.Username, Email: u.Email,
		DisplayName: flds.displayName, Avatar: flds.avatar, Bio: flds.bio,
		Presence:       &presence,
		CurrentProject: flds.currentProject, CurrentTask: flds.currentTask, CurrentBranch: flds.currentBranch,
		Repositories: append([]string(nil), flds.repositories...),
	}, nil
}

func (f *fakeProfileRepo) UpdateProfile(_ any, userID string, displayName, avatar, bio *string) error {
	if _, err := f.users.GetByID(context.Background(), userID); err != nil {
		return profile.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	flds := f.fieldsFor(userID)
	if displayName != nil {
		flds.displayName = displayName
	}
	if avatar != nil {
		flds.avatar = avatar
	}
	if bio != nil {
		flds.bio = bio
	}
	return nil
}

func (f *fakeProfileRepo) GetPrivacy(_ any, userID string) (map[profile.Field]profile.Audience, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	settings := make(map[profile.Field]profile.Audience)
	for field, audience := range f.privacy[userID] {
		settings[field] = audience
	}
	return settings, nil
}

func (f *fakeProfileRepo) SetPrivacy(_ any, userID string, field profile.Field, audience profile.Audience) error {
	if _, err := f.users.GetByID(context.Background(), userID); err != nil {
		return profile.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.privacy[userID] == nil {
		f.privacy[userID] = make(map[profile.Field]profile.Audience)
	}
	f.privacy[userID][field] = audience
	return nil
}

func (f *fakeProfileRepo) SetDefaultPrivacy(_ any, userID string) error {
	if _, err := f.users.GetByID(context.Background(), userID); err != nil {
		return profile.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.privacy[userID] = map[profile.Field]profile.Audience{
		profile.FieldDisplayName:    profile.AudienceEveryone,
		profile.FieldAvatar:         profile.AudienceEveryone,
		profile.FieldBio:            profile.AudienceFriends,
		profile.FieldCurrentProject: profile.AudienceEveryone,
		profile.FieldCurrentTask:    profile.AudienceFriends,
		profile.FieldCurrentBranch:  profile.AudienceFriends,
		profile.FieldRepositories:   profile.AudienceEveryone,
	}
	return nil
}

// fakeProfileMembership is a minimal in-memory profile.Membership.
// ShareProject is backed by the real fakeProjectRepo (Project membership is
// already a real product feature); friends and blocks have no
// product-facing way to be created yet (see
// internal/store/postgres.ProfileRepository's AreFollowers/IsBlocked doc
// comments — that's Phase 2), so tests set them directly here.
type fakeProfileMembership struct {
	projects *fakeProjectRepo

	mu      sync.Mutex
	friends map[string]map[string]bool
	blocks  map[string]map[string]bool // blocks[blockerID][blockedID]
}

func newFakeProfileMembership(projects *fakeProjectRepo) *fakeProfileMembership {
	return &fakeProfileMembership{
		projects: projects,
		friends:  make(map[string]map[string]bool),
		blocks:   make(map[string]map[string]bool),
	}
}

func (f *fakeProfileMembership) setFriends(a, b string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.friends[a] == nil {
		f.friends[a] = make(map[string]bool)
	}
	if f.friends[b] == nil {
		f.friends[b] = make(map[string]bool)
	}
	f.friends[a][b] = true
	f.friends[b][a] = true
}

func (f *fakeProfileMembership) setBlocked(blockerID, blockedID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocks[blockerID] == nil {
		f.blocks[blockerID] = make(map[string]bool)
	}
	f.blocks[blockerID][blockedID] = true
}

func (f *fakeProfileMembership) AreFollowers(_ any, aUserID, bUserID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.friends[aUserID][bUserID], nil
}

func (f *fakeProfileMembership) ShareProject(_ any, userID1, userID2 string) (bool, error) {
	f.projects.mu.Lock()
	defer f.projects.mu.Unlock()
	for _, members := range f.projects.members {
		has1, has2 := false, false
		for _, m := range members {
			if m.UserID == userID1 {
				has1 = true
			}
			if m.UserID == userID2 {
				has2 = true
			}
		}
		if has1 && has2 {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeProfileMembership) IsBlocked(_ any, userID, blockerID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.blocks[blockerID][userID], nil
}

func TestGetOwnProfileRequiresAuth(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	rec := doJSON(t, router, http.MethodGet, "/api/v1/users/me", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetOwnProfileIncludesEmailAndPrivacy(t *testing.T) {
	router, handlers, _ := newTestRouterWithHandlers()
	token := registerAndToken(t, router, "owner")
	userID := currentUserID(t, router, token)
	if err := handlers.Profiles.InitializeDefaultPrivacy(context.Background(), userID); err != nil {
		t.Fatalf("InitializeDefaultPrivacy: %v", err)
	}

	rec := doJSON(t, router, http.MethodGet, "/api/v1/users/me", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		UserID   string `json:"user_id"`
		Email    string `json:"email"`
		Username string `json:"username"`
		Privacy  []struct {
			Field    string `json:"field"`
			Audience string `json:"audience"`
		} `json:"privacy"`
	}
	decodeJSON(t, rec, &body)
	if body.UserID != userID || body.Username != "owner" {
		t.Fatalf("unexpected profile: %+v", body)
	}
	if body.Email == "" {
		t.Fatalf("own profile should include email, got: %s", rec.Body.String())
	}
	if len(body.Privacy) != 7 {
		t.Fatalf("expected 7 privacy settings after default initialization, got %d: %+v", len(body.Privacy), body.Privacy)
	}
}

func TestUpdateOwnProfile(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	token := registerAndToken(t, router, "editor")

	rec := doJSON(t, router, http.MethodPut, "/api/v1/users/profile", map[string]any{
		"display_name": "Ed Editor",
		"avatar":       "https://example.com/avatar.png",
		"bio":          "Hello world",
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		DisplayName *string `json:"display_name"`
		Avatar      *string `json:"avatar"`
		Bio         *string `json:"bio"`
	}
	decodeJSON(t, rec, &body)
	if body.DisplayName == nil || *body.DisplayName != "Ed Editor" {
		t.Fatalf("unexpected display_name: %+v", body.DisplayName)
	}
	if body.Bio == nil || *body.Bio != "Hello world" {
		t.Fatalf("unexpected bio: %+v", body.Bio)
	}
}

func TestUpdateOwnProfileValidationError(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	token := registerAndToken(t, router, "badinput")

	rec := doJSON(t, router, http.MethodPut, "/api/v1/users/profile", map[string]any{
		"avatar": "not-a-url",
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetPublicProfileHidesFriendsOnlyBioFromStranger(t *testing.T) {
	router, handlers, _ := newTestRouterWithHandlers()
	ownerToken := registerAndToken(t, router, "bioowner")
	strangerToken := registerAndToken(t, router, "stranger")
	ownerID := currentUserID(t, router, ownerToken)

	if err := handlers.Profiles.InitializeDefaultPrivacy(context.Background(), ownerID); err != nil {
		t.Fatalf("InitializeDefaultPrivacy: %v", err)
	}
	updateRec := doJSON(t, router, http.MethodPut, "/api/v1/users/profile", map[string]any{"bio": "friends only bio"}, ownerToken)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update bio: %d %s", updateRec.Code, updateRec.Body.String())
	}

	rec := doJSON(t, router, http.MethodGet, "/api/v1/users/"+ownerID+"/profile", nil, strangerToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("get public profile: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Bio   *string `json:"bio"`
		Email string  `json:"email"`
	}
	decodeJSON(t, rec, &body)
	if body.Bio != nil {
		t.Fatalf("expected bio hidden from stranger (default friends-only), got %+v", body.Bio)
	}
	if body.Email != "" {
		t.Fatalf("public profile must never expose email, got %q", body.Email)
	}

	profileMembership := handlers.Profiles.Membership.(*fakeProfileMembership)
	strangerID := currentUserID(t, router, strangerToken)
	profileMembership.setFriends(ownerID, strangerID)

	rec = doJSON(t, router, http.MethodGet, "/api/v1/users/"+ownerID+"/profile", nil, strangerToken)
	decodeJSON(t, rec, &body)
	if body.Bio == nil || *body.Bio != "friends only bio" {
		t.Fatalf("expected bio visible to a friend, got %+v", body.Bio)
	}
}

func TestGetPublicProfileProjectMembersOnlyField(t *testing.T) {
	router, handlers, projects := newTestRouterWithHandlers()
	ownerToken := registerAndToken(t, router, "pmowner")
	teammateToken := registerAndToken(t, router, "pmteammate")
	outsiderToken := registerAndToken(t, router, "pmoutsider")
	ownerID := currentUserID(t, router, ownerToken)
	teammateID := currentUserID(t, router, teammateToken)

	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Shared"}, ownerToken)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)
	projects.addMember(p.ID, teammateID, "member")

	task := "Refactor auth"
	profileRepo := handlers.Profiles.Repo.(*fakeProfileRepo)
	profileRepo.setLiveContext(ownerID, nil, &task, nil)
	profileRepo.setOnline(ownerID, true)

	privRec := doJSON(t, router, http.MethodPut, "/api/v1/users/profile/privacy", map[string]any{
		"settings": []map[string]string{{"field": "current_task", "audience": "project_members"}},
	}, ownerToken)
	if privRec.Code != http.StatusOK {
		t.Fatalf("set privacy: %d %s", privRec.Code, privRec.Body.String())
	}

	// Outsider: neither friend nor shared-project member.
	rec := doJSON(t, router, http.MethodGet, "/api/v1/users/"+ownerID+"/profile", nil, outsiderToken)
	var body struct {
		CurrentTask *string `json:"current_task"`
	}
	decodeJSON(t, rec, &body)
	if body.CurrentTask != nil {
		t.Fatalf("expected current_task hidden from outsider, got %+v", body.CurrentTask)
	}

	// Teammate: shares a project.
	rec = doJSON(t, router, http.MethodGet, "/api/v1/users/"+ownerID+"/profile", nil, teammateToken)
	decodeJSON(t, rec, &body)
	if body.CurrentTask == nil || *body.CurrentTask != "Refactor auth" {
		t.Fatalf("expected current_task visible to project teammate, got %+v", body.CurrentTask)
	}
}

func TestGetPublicProfileHidesLiveFieldsWhenOffline(t *testing.T) {
	router, handlers, _ := newTestRouterWithHandlers()
	ownerToken := registerAndToken(t, router, "offlineowner")
	viewerToken := registerAndToken(t, router, "offlineviewer")
	ownerID := currentUserID(t, router, ownerToken)

	proj := "KMJG Hub"
	profileRepo := handlers.Profiles.Repo.(*fakeProfileRepo)
	profileRepo.setLiveContext(ownerID, &proj, nil, nil)
	profileRepo.setOnline(ownerID, false)

	rec := doJSON(t, router, http.MethodGet, "/api/v1/users/"+ownerID+"/profile", nil, viewerToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("get public profile: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		CurrentProject *string `json:"current_project"`
		Presence       *string `json:"presence"`
	}
	decodeJSON(t, rec, &body)
	if body.CurrentProject != nil {
		t.Fatalf("expected current_project hidden while offline (default everyone audience), got %+v", body.CurrentProject)
	}
	if body.Presence == nil || *body.Presence != "offline" {
		t.Fatalf("expected presence offline, got %+v", body.Presence)
	}

	profileRepo.setOnline(ownerID, true)
	rec = doJSON(t, router, http.MethodGet, "/api/v1/users/"+ownerID+"/profile", nil, viewerToken)
	decodeJSON(t, rec, &body)
	if body.CurrentProject == nil || *body.CurrentProject != "KMJG Hub" {
		t.Fatalf("expected current_project visible while online, got %+v", body.CurrentProject)
	}
}

func TestGetPublicProfileBlockedViewerForbidden(t *testing.T) {
	router, handlers, _ := newTestRouterWithHandlers()
	ownerToken := registerAndToken(t, router, "blockowner")
	viewerToken := registerAndToken(t, router, "blockedviewer")
	ownerID := currentUserID(t, router, ownerToken)
	viewerID := currentUserID(t, router, viewerToken)

	profileMembership := handlers.Profiles.Membership.(*fakeProfileMembership)
	profileMembership.setBlocked(ownerID, viewerID)

	rec := doJSON(t, router, http.MethodGet, "/api/v1/users/"+ownerID+"/profile", nil, viewerToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a blocked viewer, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetPublicProfileNotFoundForUnknownUser(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	token := registerAndToken(t, router, "lookupuser")

	rec := doJSON(t, router, http.MethodGet, "/api/v1/users/does-not-exist/profile", nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSetPrivacyValidationError(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	token := registerAndToken(t, router, "privacyuser")

	rec := doJSON(t, router, http.MethodPut, "/api/v1/users/profile/privacy", map[string]any{
		"settings": []map[string]string{{"field": "bio", "audience": "not-a-real-audience"}},
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterInitializesDefaultPrivacy(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	token := registerAndToken(t, router, "freshuser")

	rec := doJSON(t, router, http.MethodGet, "/api/v1/users/me", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get own profile: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Privacy []struct {
			Field    string `json:"field"`
			Audience string `json:"audience"`
		} `json:"privacy"`
	}
	decodeJSON(t, rec, &body)
	if len(body.Privacy) != 7 {
		t.Fatalf("expected registration to initialize 7 default privacy settings, got %d: %+v", len(body.Privacy), body.Privacy)
	}
	found := false
	for _, setting := range body.Privacy {
		if setting.Field == "bio" {
			found = true
			if setting.Audience != "friends" {
				t.Fatalf("expected bio to default to friends, got %q", setting.Audience)
			}
		}
	}
	if !found {
		t.Fatalf("expected a bio privacy setting, got %+v", body.Privacy)
	}
}
