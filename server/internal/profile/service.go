package profile

import (
	"context"
	"strings"
)

// Service implements profile operations.
type Service struct {
	Repo       Repository
	Membership Membership
}

// GetOwnProfile returns the viewer's full profile including privacy settings.
func (s *Service) GetOwnProfile(ctx context.Context, userID string) (*Profile, error) {
	profile, err := s.Repo.GetProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	privacy, err := s.Repo.GetPrivacy(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile.Privacy = privacy

	return profile, nil
}

// GetPublicProfile returns a user's profile as seen by viewerID,
// filtered by privacy settings and relationships.
func (s *Service) GetPublicProfile(ctx context.Context, userID, viewerID string) (*Profile, error) {
	// Check if viewerID is blocked
	if blocked, err := s.Membership.IsBlocked(ctx, viewerID, userID); err == nil && blocked {
		return nil, ErrForbidden // Blocked users see nothing
	}

	profile, err := s.Repo.GetProfile(ctx, userID)
	if err != nil {
		return nil, err
	}

	privacy, err := s.Repo.GetPrivacy(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Filter fields based on privacy and relationships
	s.applyPrivacyFiltering(ctx, profile, privacy, viewerID)

	// Don't expose privacy settings or email in public profile
	profile.Privacy = nil
	profile.Email = ""

	return profile, nil
}

// UpdateProfile updates the viewer's own profile fields.
func (s *Service) UpdateProfile(ctx context.Context, userID string, displayName, avatar, bio *string) error {
	if err := validateDisplayName(displayName); err != nil {
		return err
	}
	if err := validateAvatar(avatar); err != nil {
		return err
	}
	if err := validateBio(bio); err != nil {
		return err
	}

	// Trim whitespace
	if displayName != nil {
		trimmed := strings.TrimSpace(*displayName)
		displayName = &trimmed
	}
	if bio != nil {
		trimmed := strings.TrimSpace(*bio)
		bio = &trimmed
	}

	return s.Repo.UpdateProfile(ctx, userID, displayName, avatar, bio)
}

// SetPrivacy updates the privacy setting for a field.
func (s *Service) SetPrivacy(ctx context.Context, userID string, field Field, audience Audience) error {
	if err := validatePrivacyField(field); err != nil {
		return err
	}
	if err := validatePrivacyAudience(audience); err != nil {
		return err
	}

	return s.Repo.SetPrivacy(ctx, userID, field, audience)
}

// applyPrivacyFiltering removes fields from profile based on privacy settings and viewer relationships.
func (s *Service) applyPrivacyFiltering(ctx context.Context, profile *Profile, privacy map[Field]Audience, viewerID string) {
	// Default privacy if not set
	defaultPrivacy := map[Field]Audience{
		FieldDisplayName:    AudienceEveryone,
		FieldAvatar:         AudienceEveryone,
		FieldBio:            AudienceFriends,
		FieldCurrentProject: AudienceEveryone,
		FieldCurrentTask:    AudienceFriends,
		FieldCurrentBranch:  AudienceFriends,
		FieldRepositories:   AudienceEveryone,
	}

	// Determine viewer relationships
	isFriend := false
	sharesProject := false
	if viewerID != profile.UserID {
		if f, err := s.Membership.AreFollowers(ctx, viewerID, profile.UserID); err == nil {
			isFriend = f
		}
		if sp, err := s.Membership.ShareProject(ctx, viewerID, profile.UserID); err == nil {
			sharesProject = sp
		}
	} else {
		// Own profile: always fully visible
		return
	}

	// Check each field's audience
	if !s.canViewField(privacy, FieldDisplayName, defaultPrivacy, isFriend, sharesProject) {
		profile.DisplayName = nil
	}
	if !s.canViewField(privacy, FieldAvatar, defaultPrivacy, isFriend, sharesProject) {
		profile.Avatar = nil
	}
	if !s.canViewField(privacy, FieldBio, defaultPrivacy, isFriend, sharesProject) {
		profile.Bio = nil
	}
	if !s.canViewField(privacy, FieldCurrentProject, defaultPrivacy, isFriend, sharesProject) {
		profile.CurrentProject = nil
	}
	if !s.canViewField(privacy, FieldCurrentTask, defaultPrivacy, isFriend, sharesProject) {
		profile.CurrentTask = nil
	}
	if !s.canViewField(privacy, FieldCurrentBranch, defaultPrivacy, isFriend, sharesProject) {
		profile.CurrentBranch = nil
	}
	if !s.canViewField(privacy, FieldRepositories, defaultPrivacy, isFriend, sharesProject) {
		profile.Repositories = []string{}
	}

	// Hide live fields if user is offline
	if profile.Presence != nil && *profile.Presence == "offline" {
		profile.CurrentProject = nil
		profile.CurrentTask = nil
		profile.CurrentBranch = nil
	}
}

func (s *Service) canViewField(privacy map[Field]Audience, field Field, defaultPrivacy map[Field]Audience, isFriend, sharesProject bool) bool {
	var audience Audience
	if aud, exists := privacy[field]; exists {
		audience = aud
	} else {
		audience = defaultPrivacy[field]
	}

	switch audience {
	case AudienceEveryone:
		return true
	case AudienceFriends:
		return isFriend
	case AudienceProjectMembers:
		return sharesProject
	case AudienceFriendsAndProjectMembers:
		return isFriend || sharesProject
	case AudienceNobody:
		return false
	default:
		return false
	}
}

// InitializeDefaultPrivacy sets up default privacy rules for a new user.
func (s *Service) InitializeDefaultPrivacy(ctx context.Context, userID string) error {
	return s.Repo.SetDefaultPrivacy(ctx, userID)
}
