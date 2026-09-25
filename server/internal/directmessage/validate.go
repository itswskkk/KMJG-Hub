package directmessage

import (
	"strings"
	"unicode/utf8"
)

// validateBody trims surrounding whitespace and enforces the same body rules
// as Project Chat: non-empty, valid UTF-8, at most MaxMessageCharacters.
func validateBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", &ValidationError{Field: "body", Message: "Message cannot be empty"}
	}
	if !utf8.ValidString(body) || utf8.RuneCountInString(body) > MaxMessageCharacters {
		return "", &ValidationError{Field: "body", Message: "Message must be 4,000 characters or fewer"}
	}
	return body, nil
}
