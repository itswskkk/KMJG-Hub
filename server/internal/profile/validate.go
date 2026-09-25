package profile

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ValidationError describes a validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("profile: validation error on field %s: %s", e.Field, e.Message)
}

func validateDisplayName(name *string) error {
	if name == nil {
		return nil // optional
	}

	trimmed := strings.TrimSpace(*name)
	if trimmed == "" {
		// Empty string allowed, but not whitespace-only
		if *name != "" {
			return &ValidationError{Field: "display_name", Message: "Cannot be only whitespace"}
		}
		return nil
	}

	if utf8.RuneCountInString(trimmed) > 255 {
		return &ValidationError{Field: "display_name", Message: "Display name must be 255 characters or fewer"}
	}
	return nil
}

func validateBio(bio *string) error {
	if bio == nil {
		return nil // optional
	}

	trimmed := strings.TrimSpace(*bio)
	if trimmed == "" {
		if *bio != "" {
			return &ValidationError{Field: "bio", Message: "Cannot be only whitespace"}
		}
		return nil
	}

	if utf8.RuneCountInString(trimmed) > 500 {
		return &ValidationError{Field: "bio", Message: "Bio must be 500 characters or fewer"}
	}
	return nil
}

func validateAvatar(avatar *string) error {
	if avatar == nil {
		return nil // optional
	}

	if *avatar == "" {
		return nil // allow clearing
	}

	// Very basic URL validation: at least has :// and . somewhere
	if !strings.Contains(*avatar, "://") || !strings.Contains(*avatar, ".") {
		return &ValidationError{Field: "avatar", Message: "Avatar must be a valid URL"}
	}

	if utf8.RuneCountInString(*avatar) > 2048 {
		return &ValidationError{Field: "avatar", Message: "Avatar URL must be 2048 characters or fewer"}
	}
	return nil
}

func validatePrivacyField(field Field) error {
	switch field {
	case FieldDisplayName, FieldAvatar, FieldBio, FieldCurrentProject, FieldCurrentTask, FieldCurrentBranch, FieldRepositories:
		return nil
	default:
		return &ValidationError{Field: "field", Message: fmt.Sprintf("Unknown field: %s", field)}
	}
}

func validatePrivacyAudience(audience Audience) error {
	switch audience {
	case AudienceEveryone, AudienceFriends, AudienceProjectMembers, AudienceFriendsAndProjectMembers, AudienceNobody:
		return nil
	default:
		return &ValidationError{Field: "audience", Message: fmt.Sprintf("Unknown audience: %s", audience)}
	}
}
