package profile_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/itswskkk/KMJG-Hub/server/internal/profile"
)

// fakeRepository is a minimal in-memory profile.Repository for exercising
// profile.Service without a real PostgreSQL instance.
type fakeRepository struct {
	mu       sync.Mutex
	profiles map[string]*profile.Profile
	privacy  map[string]map[profile.Field]profile.Audience
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		profiles: make(map[string]*profile.Profile),
		privacy:  make(map[string]map[profile.Field]profile.Audience),
	}
}

func (f *fakeRepository) put(p profile.Profile) {
	f.mu.Lock()
	defer f.mu.Unlock()
	stored := p
	f.profiles[p.UserID] = &stored
}

func (f *fakeRepository) GetProfile(_ any, userID string) (*profile.Profile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.profiles[userID]
	if !ok {
		return nil, profile.ErrNotFound
	}
	copy := *p
	// Repositories is a slice; copy it too so callers mutating the
	// returned Profile can't corrupt fixture state between tests.
	copy.Repositories = append([]string(nil), p.Repositories...)
	return &copy, nil
}

func (f *fakeRepository) UpdateProfile(_ any, userID string, displayName, avatar, bio *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.profiles[userID]
	if !ok {
		return profile.ErrNotFound
	}
	if displayName != nil {
		p.DisplayName = displayName
	}
	if avatar != nil {
		p.Avatar = avatar
	}
	if bio != nil {
		p.Bio = bio
	}
	return nil
}

func (f *fakeRepository) GetPrivacy(_ any, userID string) (map[profile.Field]profile.Audience, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	settings := make(map[profile.Field]profile.Audience)
	for field, audience := range f.privacy[userID] {
		settings[field] = audience
	}
	return settings, nil
}

func (f *fakeRepository) SetPrivacy(_ any, userID string, field profile.Field, audience profile.Audience) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.profiles[userID]; !ok {
		return profile.ErrNotFound
	}
	if f.privacy[userID] == nil {
		f.privacy[userID] = make(map[profile.Field]profile.Audience)
	}
	f.privacy[userID][field] = audience
	return nil
}

func (f *fakeRepository) SetDefaultPrivacy(_ any, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.profiles[userID]; !ok {
		return profile.ErrNotFound
	}
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

// fakeMembership is a minimal in-memory profile.Membership fixture, set up
// directly by each test.
type fakeMembership struct {
	mu       sync.Mutex
	friends  map[string]map[string]bool // undirected: friends[a][b] == friends[b][a]
	sharedPr map[string]map[string]bool // undirected
	blocks   map[string]map[string]bool // blocks[blockerID][blockedID]
}

func newFakeMembership() *fakeMembership {
	return &fakeMembership{
		friends:  make(map[string]map[string]bool),
		sharedPr: make(map[string]map[string]bool),
		blocks:   make(map[string]map[string]bool),
	}
}

func (f *fakeMembership) setFriends(a, b string) {
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

func (f *fakeMembership) setSharesProject(a, b string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sharedPr[a] == nil {
		f.sharedPr[a] = make(map[string]bool)
	}
	if f.sharedPr[b] == nil {
		f.sharedPr[b] = make(map[string]bool)
	}
	f.sharedPr[a][b] = true
	f.sharedPr[b][a] = true
}

func (f *fakeMembership) setBlocked(blockerID, blockedID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.blocks[blockerID] == nil {
		f.blocks[blockerID] = make(map[string]bool)
	}
	f.blocks[blockerID][blockedID] = true
}

func (f *fakeMembership) AreFollowers(_ any, aUserID, bUserID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.friends[aUserID][bUserID], nil
}

func (f *fakeMembership) ShareProject(_ any, userID1, userID2 string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sharedPr[userID1][userID2], nil
}

func (f *fakeMembership) IsBlocked(_ any, userID, blockerID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.blocks[blockerID][userID], nil
}

func strPtr(s string) *string { return &s }

func newService() (*profile.Service, *fakeRepository, *fakeMembership) {
	repo := newFakeRepository()
	membership := newFakeMembership()
	return &profile.Service{Repo: repo, Membership: membership}, repo, membership
}

func TestGetOwnProfileIsFullyVisible(t *testing.T) {
	svc, repo, _ := newService()
	online := "online"
	repo.put(profile.Profile{
		UserID: "u1", Username: "alice", Email: "alice@example.com",
		DisplayName: strPtr("Alice"), Bio: strPtr("hi"),
		CurrentProject: strPtr("KMJG"), CurrentTask: strPtr("Ship it"), CurrentBranch: strPtr("main"),
		Presence: &online,
	})
	repo.privacy["u1"] = map[profile.Field]profile.Audience{profile.FieldBio: profile.AudienceNobody}

	got, err := svc.GetOwnProfile(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetOwnProfile: %v", err)
	}
	if got.Bio == nil || *got.Bio != "hi" {
		t.Fatalf("own profile bio should be fully visible regardless of privacy, got %+v", got.Bio)
	}
	if got.Email != "alice@example.com" {
		t.Fatalf("own profile should include email, got %q", got.Email)
	}
	if got.Privacy[profile.FieldBio] != profile.AudienceNobody {
		t.Fatalf("own profile should include privacy settings")
	}
}

func TestGetPublicProfileHidesEmailAndPrivacy(t *testing.T) {
	svc, repo, _ := newService()
	online := "online"
	repo.put(profile.Profile{UserID: "u1", Username: "alice", Email: "alice@example.com", Presence: &online})

	got, err := svc.GetPublicProfile(context.Background(), "u1", "u2")
	if err != nil {
		t.Fatalf("GetPublicProfile: %v", err)
	}
	if got.Email != "" {
		t.Fatalf("public profile must never expose email, got %q", got.Email)
	}
	if got.Privacy != nil {
		t.Fatalf("public profile must never expose privacy settings")
	}
}

func TestDefaultPrivacyHidesBioFromStrangerButShowsFriend(t *testing.T) {
	svc, repo, membership := newService()
	online := "online"
	repo.put(profile.Profile{UserID: "owner", Username: "alice", Bio: strPtr("secret bio"), Presence: &online})
	// No explicit privacy rows: service falls back to its own defaults,
	// where bio defaults to friends-only.

	stranger, err := svc.GetPublicProfile(context.Background(), "owner", "stranger")
	if err != nil {
		t.Fatalf("GetPublicProfile (stranger): %v", err)
	}
	if stranger.Bio != nil {
		t.Fatalf("bio should default to friends-only and be hidden from a stranger, got %+v", stranger.Bio)
	}

	membership.setFriends("owner", "friend")
	friend, err := svc.GetPublicProfile(context.Background(), "owner", "friend")
	if err != nil {
		t.Fatalf("GetPublicProfile (friend): %v", err)
	}
	if friend.Bio == nil || *friend.Bio != "secret bio" {
		t.Fatalf("bio should be visible to a friend, got %+v", friend.Bio)
	}
}

func TestExplicitProjectMembersPrivacyAppliesToSharedProjectViewerOnly(t *testing.T) {
	svc, repo, membership := newService()
	online := "online"
	repo.put(profile.Profile{UserID: "owner", Username: "alice", CurrentTask: strPtr("Refactor auth"), Presence: &online})
	if err := svc.SetPrivacy(context.Background(), "owner", profile.FieldCurrentTask, profile.AudienceProjectMembers); err != nil {
		t.Fatalf("SetPrivacy: %v", err)
	}

	// Neither friend nor project member: hidden.
	stranger, err := svc.GetPublicProfile(context.Background(), "owner", "stranger")
	if err != nil {
		t.Fatalf("GetPublicProfile (stranger): %v", err)
	}
	if stranger.CurrentTask != nil {
		t.Fatalf("current_task set to project_members should be hidden from a non-member, got %+v", stranger.CurrentTask)
	}

	// Shares a project, not a friend: visible.
	membership.setSharesProject("owner", "teammate")
	teammate, err := svc.GetPublicProfile(context.Background(), "owner", "teammate")
	if err != nil {
		t.Fatalf("GetPublicProfile (teammate): %v", err)
	}
	if teammate.CurrentTask == nil || *teammate.CurrentTask != "Refactor auth" {
		t.Fatalf("current_task set to project_members should be visible to a shared-project viewer, got %+v", teammate.CurrentTask)
	}
}

func TestAudienceNobodyHidesFieldFromEveryone(t *testing.T) {
	svc, repo, membership := newService()
	online := "online"
	repo.put(profile.Profile{UserID: "owner", Username: "alice", DisplayName: strPtr("Alice"), Presence: &online})
	if err := svc.SetPrivacy(context.Background(), "owner", profile.FieldDisplayName, profile.AudienceNobody); err != nil {
		t.Fatalf("SetPrivacy: %v", err)
	}
	membership.setFriends("owner", "friend")

	got, err := svc.GetPublicProfile(context.Background(), "owner", "friend")
	if err != nil {
		t.Fatalf("GetPublicProfile: %v", err)
	}
	if got.DisplayName != nil {
		t.Fatalf("audience nobody should hide the field even from a friend, got %+v", got.DisplayName)
	}
}

func TestBlockedViewerCannotSeeProfile(t *testing.T) {
	svc, repo, membership := newService()
	online := "online"
	repo.put(profile.Profile{UserID: "owner", Username: "alice", Presence: &online})
	membership.setBlocked("owner", "blockedviewer")

	_, err := svc.GetPublicProfile(context.Background(), "owner", "blockedviewer")
	if !errors.Is(err, profile.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a blocked viewer, got %v", err)
	}
}

func TestOfflineHidesLiveFieldsEvenWhenAudienceAllows(t *testing.T) {
	svc, repo, _ := newService()
	offline := "offline"
	repo.put(profile.Profile{
		UserID: "owner", Username: "alice",
		CurrentProject: strPtr("KMJG"), CurrentTask: strPtr("Ship it"), CurrentBranch: strPtr("main"),
		Presence: &offline,
	})
	// Defaults: current_project is everyone-visible, so this isolates the
	// offline-hiding rule rather than the audience rule.

	got, err := svc.GetPublicProfile(context.Background(), "owner", "anyone")
	if err != nil {
		t.Fatalf("GetPublicProfile: %v", err)
	}
	if got.CurrentProject != nil || got.CurrentTask != nil || got.CurrentBranch != nil {
		t.Fatalf("live fields should be hidden while offline, got project=%+v task=%+v branch=%+v", got.CurrentProject, got.CurrentTask, got.CurrentBranch)
	}
}

func TestOnlineShowsLiveFieldsWhenAudienceAllows(t *testing.T) {
	svc, repo, _ := newService()
	online := "online"
	repo.put(profile.Profile{
		UserID: "owner", Username: "alice",
		CurrentProject: strPtr("KMJG"),
		Presence:       &online,
	})

	got, err := svc.GetPublicProfile(context.Background(), "owner", "anyone")
	if err != nil {
		t.Fatalf("GetPublicProfile: %v", err)
	}
	if got.CurrentProject == nil || *got.CurrentProject != "KMJG" {
		t.Fatalf("current_project should be visible while online under the default everyone audience, got %+v", got.CurrentProject)
	}
}

func TestUpdateProfileValidatesFields(t *testing.T) {
	svc, repo, _ := newService()
	repo.put(profile.Profile{UserID: "u1", Username: "alice"})

	tooLong := make([]byte, 256)
	for i := range tooLong {
		tooLong[i] = 'a'
	}
	err := svc.UpdateProfile(context.Background(), "u1", strPtr(string(tooLong)), nil, nil)
	var validationErr *profile.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "display_name" {
		t.Fatalf("expected display_name validation error, got %v", err)
	}

	err = svc.UpdateProfile(context.Background(), "u1", nil, strPtr("not-a-url"), nil)
	if !errors.As(err, &validationErr) || validationErr.Field != "avatar" {
		t.Fatalf("expected avatar validation error, got %v", err)
	}

	err = svc.UpdateProfile(context.Background(), "u1", nil, nil, strPtr("  "))
	if !errors.As(err, &validationErr) || validationErr.Field != "bio" {
		t.Fatalf("expected bio validation error for whitespace-only bio, got %v", err)
	}
}

func TestUpdateProfileTrimsAndAppliesValidFields(t *testing.T) {
	svc, repo, _ := newService()
	repo.put(profile.Profile{UserID: "u1", Username: "alice"})

	if err := svc.UpdateProfile(context.Background(), "u1", strPtr("  Alice B  "), strPtr("https://example.com/a.png"), strPtr("  hello  ")); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	got, err := svc.GetOwnProfile(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetOwnProfile: %v", err)
	}
	if got.DisplayName == nil || *got.DisplayName != "Alice B" {
		t.Fatalf("expected trimmed display name, got %+v", got.DisplayName)
	}
	if got.Avatar == nil || *got.Avatar != "https://example.com/a.png" {
		t.Fatalf("expected avatar to be set, got %+v", got.Avatar)
	}
	if got.Bio == nil || *got.Bio != "hello" {
		t.Fatalf("expected trimmed bio, got %+v", got.Bio)
	}
}

func TestSetPrivacyValidatesFieldAndAudience(t *testing.T) {
	svc, repo, _ := newService()
	repo.put(profile.Profile{UserID: "u1", Username: "alice"})

	err := svc.SetPrivacy(context.Background(), "u1", profile.Field("nonsense"), profile.AudienceEveryone)
	var validationErr *profile.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "field" {
		t.Fatalf("expected field validation error, got %v", err)
	}

	err = svc.SetPrivacy(context.Background(), "u1", profile.FieldBio, profile.Audience("nonsense"))
	if !errors.As(err, &validationErr) || validationErr.Field != "audience" {
		t.Fatalf("expected audience validation error, got %v", err)
	}
}

func TestInitializeDefaultPrivacy(t *testing.T) {
	svc, repo, _ := newService()
	repo.put(profile.Profile{UserID: "u1", Username: "alice"})

	if err := svc.InitializeDefaultPrivacy(context.Background(), "u1"); err != nil {
		t.Fatalf("InitializeDefaultPrivacy: %v", err)
	}
	settings, err := repo.GetPrivacy(context.Background(), "u1")
	if err != nil {
		t.Fatalf("GetPrivacy: %v", err)
	}
	if settings[profile.FieldBio] != profile.AudienceFriends {
		t.Fatalf("expected default bio audience friends, got %v", settings[profile.FieldBio])
	}
	if settings[profile.FieldDisplayName] != profile.AudienceEveryone {
		t.Fatalf("expected default display_name audience everyone, got %v", settings[profile.FieldDisplayName])
	}
}

func TestGetPublicProfileNotFound(t *testing.T) {
	svc, _, _ := newService()
	_, err := svc.GetPublicProfile(context.Background(), "missing", "viewer")
	if !errors.Is(err, profile.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
