package project

// ValidationError reports that user-supplied input failed a validation
// rule. Deliberately separate from auth.ValidationError: project field
// validation has nothing to do with credentials, and coupling the two
// packages for a shared error shape isn't worth it for a four-line type.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

const (
	maxNameLength        = 100
	maxDescriptionLength = 500
)

func validateName(name string) error {
	if len(name) == 0 || len(name) > maxNameLength {
		return &ValidationError{Field: "name", Message: "must be 1-100 characters"}
	}
	return nil
}

func validateDescription(description string) error {
	if len(description) > maxDescriptionLength {
		return &ValidationError{Field: "description", Message: "must be at most 500 characters"}
	}
	return nil
}
