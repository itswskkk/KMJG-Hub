package auth

import (
	"regexp"
	"strings"
)

// ValidationError reports that user-supplied input failed a validation rule.
// The HTTP layer maps this to a 400 response with a field-level message.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)

// Loose but practical email check. Full RFC 5322 validation is not required
// for v1; the goal is to reject obviously malformed input.
var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

const minPasswordLength = 8

func validateUsername(username string) error {
	if !usernamePattern.MatchString(username) {
		return &ValidationError{
			Field:   "username",
			Message: "must be 3-32 characters and contain only letters, numbers, underscores, or hyphens",
		}
	}
	return nil
}

func validateEmail(email string) error {
	if !emailPattern.MatchString(email) {
		return &ValidationError{Field: "email", Message: "must be a valid email address"}
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < minPasswordLength {
		return &ValidationError{
			Field:   "password",
			Message: "must be at least 8 characters",
		}
	}
	return nil
}

func normalizeIdentifier(identifier string) string {
	return strings.TrimSpace(identifier)
}
