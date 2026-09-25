// Package profile implements KMJG Hub user profiles with field-level privacy.
// See docs/PRD.md § User Profiles and docs/ARCHITECTURE.md "User Profiles and Privacy".
package profile

import (
	"errors"
)

var ErrNotFound = errors.New("profile: not found")
var ErrForbidden = errors.New("profile: forbidden")

// Audience defines privacy visibility for a profile field.
type Audience string

const (
	AudienceEveryone                 Audience = "everyone"
	AudienceFriends                  Audience = "friends"
	AudienceProjectMembers           Audience = "project_members"
	AudienceFriendsAndProjectMembers Audience = "friends_and_project_members"
	AudienceNobody                   Audience = "nobody"
)

// Field identifies a profile field.
type Field string

const (
	FieldDisplayName    Field = "display_name"
	FieldAvatar         Field = "avatar"
	FieldBio            Field = "bio"
	FieldCurrentProject Field = "current_project"
	FieldCurrentTask    Field = "current_task"
	FieldCurrentBranch  Field = "current_branch"
	FieldRepositories   Field = "repositories"
)

// Profile represents a user's profile data.
// Only fields visible under the viewer's privacy settings are populated.
type Profile struct {
	UserID         string
	Username       string
	Email          string // never exposed in public profile
	DisplayName    *string
	Avatar         *string
	Bio            *string
	Presence       *string // "online" or "offline"
	WorkStatus     *string // "working" or empty
	CurrentProject *string
	CurrentTask    *string
	CurrentBranch  *string
	Repositories   []string

	// Privacy settings (only returned for own profile)
	Privacy map[Field]Audience `json:"-"`
}

// PrivacySetting holds one field's privacy configuration.
type PrivacySetting struct {
	Field    Field
	Audience Audience
}

// Repository persists and retrieves profiles.
type Repository interface {
	// GetProfile returns a user's full profile (no privacy filtering). Used internally.
	GetProfile(ctx any, userID string) (*Profile, error)

	// UpdateProfile updates profile fields for userID.
	UpdateProfile(ctx any, userID string, displayName, avatar, bio *string) error

	// GetPrivacy returns all privacy settings for userID.
	GetPrivacy(ctx any, userID string) (map[Field]Audience, error)

	// SetPrivacy updates privacy for one field, or creates if not exists.
	SetPrivacy(ctx any, userID string, field Field, audience Audience) error

	// SetDefaultPrivacy initializes default privacy rules for a new user.
	SetDefaultPrivacy(ctx any, userID string) error
}

// Membership queries friend and project relationships.
// Used to determine if a viewer can see certain profile fields.
type Membership interface {
	// AreFollowers returns true if aUserID follows bUserID.
	AreFollowers(ctx any, aUserID, bUserID string) (bool, error)

	// ShareProject returns true if both users share at least one project.
	ShareProject(ctx any, userID1, userID2 string) (bool, error)

	// IsBlocked returns true if blockerID has blocked userID.
	IsBlocked(ctx any, userID, blockerID string) (bool, error)
}
