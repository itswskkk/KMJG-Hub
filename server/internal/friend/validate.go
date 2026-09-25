package friend

import "strings"

// validateUserID rejects an empty user reference before it reaches the
// repository.
func validateUserID(field, id string) error {
	if strings.TrimSpace(id) == "" {
		return &ValidationError{Field: field, Message: "A user is required"}
	}
	return nil
}

// validateSendRequest checks the rules that can be decided without reading
// relationship state: both parties are present and distinct. Blocks,
// existing friendships, and duplicate pending requests depend on stored
// state and are enforced atomically by Repository.SendRequest.
func validateSendRequest(senderID, recipientID string) error {
	if err := validateUserID("recipient", recipientID); err != nil {
		return err
	}
	if senderID == recipientID {
		return ErrSelfRequest
	}
	return nil
}

// validateBlock checks that a block names another, non-empty user.
func validateBlock(blockerID, blockedID string) error {
	if err := validateUserID("user_id", blockedID); err != nil {
		return err
	}
	if blockerID == blockedID {
		return ErrSelfBlock
	}
	return nil
}
