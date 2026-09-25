package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/directmessage"
)

type directMessageDTO struct {
	ID                string    `json:"id"`
	SenderID          string    `json:"sender_id"`
	SenderUsername    string    `json:"sender_username"`
	RecipientID       string    `json:"recipient_id"`
	RecipientUsername string    `json:"recipient_username"`
	Body              string    `json:"body"`
	CreatedAt         time.Time `json:"created_at"`
}

func toDirectMessageDTO(m directmessage.Message) directMessageDTO {
	return directMessageDTO{
		ID: m.ID, SenderID: m.SenderID, SenderUsername: m.SenderUsername,
		RecipientID: m.RecipientID, RecipientUsername: m.RecipientUsername,
		Body: m.Body, CreatedAt: m.CreatedAt,
	}
}

type conversationDTO struct {
	OtherUserID       string    `json:"other_user_id"`
	OtherUsername     string    `json:"other_username"`
	LastMessageBody   string    `json:"last_message_body"`
	LastMessageAt     time.Time `json:"last_message_at"`
	LastMessageFromMe bool      `json:"last_message_from_me"`
}

func toConversationDTO(c directmessage.Conversation) conversationDTO {
	return conversationDTO{
		OtherUserID: c.OtherUserID, OtherUsername: c.OtherUsername,
		LastMessageBody: c.LastMessageBody, LastMessageAt: c.LastMessageAt,
		LastMessageFromMe: c.LastMessageFromMe,
	}
}

type sendDirectMessageRequest struct {
	Body string `json:"body"`
}

func (h *Handlers) handleListConversations(w http.ResponseWriter, r *http.Request) {
	conversations, err := h.DirectMessages.ListConversations(r.Context(), currentAuth(r).User.ID)
	if err != nil {
		writeDMError(w, err)
		return
	}
	dtos := make([]conversationDTO, len(conversations))
	for i, c := range conversations {
		dtos[i] = toConversationDTO(c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": dtos})
}

func (h *Handlers) handleListDMMessages(w http.ResponseWriter, r *http.Request) {
	page, err := h.DirectMessages.ListPage(r.Context(), currentAuth(r).User.ID, r.PathValue("user_id"), r.URL.Query().Get("cursor"))
	if err != nil {
		writeDMError(w, err)
		return
	}
	dtos := make([]directMessageDTO, len(page.Messages))
	for i, m := range page.Messages {
		dtos[i] = toDirectMessageDTO(m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": dtos, "next_cursor": page.NextCursor})
}

func (h *Handlers) handleSendDM(w http.ResponseWriter, r *http.Request) {
	var req sendDirectMessageRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}
	message, err := h.DirectMessages.Send(r.Context(), currentAuth(r).User.ID, r.PathValue("user_id"), req.Body)
	if err != nil {
		writeDMError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toDirectMessageDTO(*message))
}

func (h *Handlers) handleDeleteDM(w http.ResponseWriter, r *http.Request) {
	if err := h.DirectMessages.Delete(r.Context(), r.PathValue("id"), currentAuth(r).User.ID); err != nil {
		writeDMError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeDMError(w http.ResponseWriter, err error) {
	var validationErr *directmessage.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeFieldError(w, http.StatusBadRequest, "validation_error", validationErr.Message, validationErr.Field)
	case errors.Is(err, directmessage.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "User or message not found")
	case errors.Is(err, directmessage.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to perform this Direct Message action")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong, please try again")
	}
}
