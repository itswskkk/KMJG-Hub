package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/notification"
)

type notificationDTO struct {
	ID        string          `json:"id"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
	ReadAt    *time.Time      `json:"read_at"`
}

type notificationPageDTO struct {
	Notifications []notificationDTO `json:"notifications"`
	NextCursor    string            `json:"next_cursor"`
	UnreadCount   int               `json:"unread_count"`
}

func toNotificationDTO(n notification.Notification) notificationDTO {
	payload := n.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	return notificationDTO{
		ID: n.ID, EventType: string(n.EventType), Payload: payload,
		CreatedAt: n.CreatedAt, ReadAt: n.ReadAt,
	}
}

func (h *Handlers) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	page, err := h.Notifications.ListPage(r.Context(), currentAuth(r).User.ID, r.URL.Query().Get("cursor"))
	if err != nil {
		writeNotificationError(w, err)
		return
	}
	dtos := make([]notificationDTO, len(page.Notifications))
	for i, n := range page.Notifications {
		dtos[i] = toNotificationDTO(n)
	}
	writeJSON(w, http.StatusOK, notificationPageDTO{
		Notifications: dtos, NextCursor: page.NextCursor, UnreadCount: page.UnreadCount,
	})
}

func (h *Handlers) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	if err := h.Notifications.MarkRead(r.Context(), r.PathValue("id"), currentAuth(r).User.ID); err != nil {
		writeNotificationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleDeleteNotification(w http.ResponseWriter, r *http.Request) {
	if err := h.Notifications.Delete(r.Context(), r.PathValue("id"), currentAuth(r).User.ID); err != nil {
		writeNotificationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeNotificationError(w http.ResponseWriter, err error) {
	var validationErr *notification.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeFieldError(w, http.StatusBadRequest, "validation_error", validationErr.Message, validationErr.Field)
	case errors.Is(err, notification.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Notification not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong, please try again")
	}
}
