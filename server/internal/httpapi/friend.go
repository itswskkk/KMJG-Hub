package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/friend"
)

type friendRequestDTO struct {
	ID                string     `json:"id"`
	SenderID          string     `json:"sender_id"`
	SenderUsername    string     `json:"sender_username"`
	RecipientID       string     `json:"recipient_id"`
	RecipientUsername string     `json:"recipient_username"`
	Status            string     `json:"status"`
	CreatedAt         time.Time  `json:"created_at"`
	RespondedAt       *time.Time `json:"responded_at,omitempty"`
}

type friendshipDTO struct {
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	Since    time.Time `json:"since"`
}

type blockDTO struct {
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	Since    time.Time `json:"since"`
}

type listFriendsResponseDTO struct {
	Friends []friendshipDTO `json:"friends"`
}

type listFriendRequestsResponseDTO struct {
	Requests []friendRequestDTO `json:"requests"`
}

type listBlockedResponseDTO struct {
	Blocked []blockDTO `json:"blocked"`
}

func toFriendRequestDTO(req friend.FriendRequest) friendRequestDTO {
	return friendRequestDTO{
		ID: req.ID, SenderID: req.SenderID, SenderUsername: req.SenderUsername,
		RecipientID: req.RecipientID, RecipientUsername: req.RecipientUsername,
		Status: string(req.Status), CreatedAt: req.CreatedAt, RespondedAt: req.RespondedAt,
	}
}

func toBlockDTO(b friend.Block) blockDTO {
	return blockDTO{UserID: b.BlockedID, Username: b.BlockedUsername, Since: b.Since}
}

// userTargetRequest names another user either by ID or by username/email.
// Exactly one is expected; user_id wins if both are sent.
type userTargetRequest struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

// sendFriendRequestRequest names the recipient by ID or by username/email.
type sendFriendRequestRequest struct {
	RecipientID string `json:"recipient_id"`
	Recipient   string `json:"recipient"`
}

func (h *Handlers) resolveTarget(r *http.Request, field, id, identifier string) (string, error) {
	if id != "" {
		return id, nil
	}
	return h.Friends.ResolveUser(r.Context(), field, identifier)
}

// handleSendFriendRequest serves POST /api/v1/friends/requests.
func (h *Handlers) handleSendFriendRequest(w http.ResponseWriter, r *http.Request) {
	var req sendFriendRequestRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}
	recipientID, err := h.resolveTarget(r, "recipient", req.RecipientID, req.Recipient)
	if err != nil {
		writeFriendError(w, err)
		return
	}
	created, err := h.Friends.SendRequest(r.Context(), currentAuth(r).User.ID, recipientID)
	if err != nil {
		writeFriendError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toFriendRequestDTO(*created))
}

// handleListIncoming serves GET /api/v1/friends/requests/incoming.
func (h *Handlers) handleListIncoming(w http.ResponseWriter, r *http.Request) {
	requests, err := h.Friends.ListIncomingRequests(r.Context(), currentAuth(r).User.ID)
	if err != nil {
		writeFriendError(w, err)
		return
	}
	writeFriendRequests(w, requests)
}

// handleListOutgoing serves GET /api/v1/friends/requests/outgoing.
func (h *Handlers) handleListOutgoing(w http.ResponseWriter, r *http.Request) {
	requests, err := h.Friends.ListOutgoingRequests(r.Context(), currentAuth(r).User.ID)
	if err != nil {
		writeFriendError(w, err)
		return
	}
	writeFriendRequests(w, requests)
}

func writeFriendRequests(w http.ResponseWriter, requests []friend.FriendRequest) {
	out := listFriendRequestsResponseDTO{Requests: make([]friendRequestDTO, 0, len(requests))}
	for _, req := range requests {
		out.Requests = append(out.Requests, toFriendRequestDTO(req))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAcceptRequest serves POST /api/v1/friends/requests/{id}/accept.
func (h *Handlers) handleAcceptRequest(w http.ResponseWriter, r *http.Request) {
	req, err := h.Friends.AcceptRequest(r.Context(), currentAuth(r).User.ID, r.PathValue("id"))
	if err != nil {
		writeFriendError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toFriendRequestDTO(*req))
}

// handleDeclineRequest serves POST /api/v1/friends/requests/{id}/decline.
func (h *Handlers) handleDeclineRequest(w http.ResponseWriter, r *http.Request) {
	req, err := h.Friends.DeclineRequest(r.Context(), currentAuth(r).User.ID, r.PathValue("id"))
	if err != nil {
		writeFriendError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toFriendRequestDTO(*req))
}

// handleCancelRequest serves DELETE /api/v1/friends/requests/{id}.
func (h *Handlers) handleCancelRequest(w http.ResponseWriter, r *http.Request) {
	if err := h.Friends.CancelRequest(r.Context(), currentAuth(r).User.ID, r.PathValue("id")); err != nil {
		writeFriendError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetFriends serves GET /api/v1/friends.
func (h *Handlers) handleGetFriends(w http.ResponseWriter, r *http.Request) {
	friends, err := h.Friends.ListFriends(r.Context(), currentAuth(r).User.ID)
	if err != nil {
		writeFriendError(w, err)
		return
	}
	out := listFriendsResponseDTO{Friends: make([]friendshipDTO, 0, len(friends))}
	for _, f := range friends {
		out.Friends = append(out.Friends, friendshipDTO{UserID: f.UserID, Username: f.Username, Since: f.Since})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRemoveFriend serves DELETE /api/v1/friends/{user_id}.
func (h *Handlers) handleRemoveFriend(w http.ResponseWriter, r *http.Request) {
	if err := h.Friends.RemoveFriendship(r.Context(), currentAuth(r).User.ID, r.PathValue("user_id")); err != nil {
		writeFriendError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleBlock serves POST /api/v1/blocked.
func (h *Handlers) handleBlock(w http.ResponseWriter, r *http.Request) {
	var req userTargetRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}
	blockedID, err := h.resolveTarget(r, "user_id", req.UserID, req.Username)
	if err != nil {
		writeFriendError(w, err)
		return
	}
	b, err := h.Friends.Block(r.Context(), currentAuth(r).User.ID, blockedID)
	if err != nil {
		writeFriendError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toBlockDTO(*b))
}

// handleUnblock serves DELETE /api/v1/blocked/{user_id}.
func (h *Handlers) handleUnblock(w http.ResponseWriter, r *http.Request) {
	if err := h.Friends.Unblock(r.Context(), currentAuth(r).User.ID, r.PathValue("user_id")); err != nil {
		writeFriendError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListBlocked serves GET /api/v1/blocked.
func (h *Handlers) handleListBlocked(w http.ResponseWriter, r *http.Request) {
	blocks, err := h.Friends.ListBlocked(r.Context(), currentAuth(r).User.ID)
	if err != nil {
		writeFriendError(w, err)
		return
	}
	out := listBlockedResponseDTO{Blocked: make([]blockDTO, 0, len(blocks))}
	for _, b := range blocks {
		out.Blocked = append(out.Blocked, toBlockDTO(b))
	}
	writeJSON(w, http.StatusOK, out)
}

func writeFriendError(w http.ResponseWriter, err error) {
	var validationErr *friend.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeFieldError(w, http.StatusBadRequest, "validation_error", validationErr.Message, validationErr.Field)
	case errors.Is(err, friend.ErrSelfRequest):
		writeError(w, http.StatusBadRequest, "self_request", "You cannot send a friend request to yourself")
	case errors.Is(err, friend.ErrSelfBlock):
		writeError(w, http.StatusBadRequest, "self_block", "You cannot block yourself")
	case errors.Is(err, friend.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Not found")
	case errors.Is(err, friend.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to do that")
	case errors.Is(err, friend.ErrBlocked):
		writeError(w, http.StatusForbidden, "blocked", "Friend requests are not available between you and this user")
	case errors.Is(err, friend.ErrAlreadyFriends):
		writeError(w, http.StatusConflict, "already_friends", "You are already friends with this user")
	case errors.Is(err, friend.ErrRequestPending):
		writeError(w, http.StatusConflict, "request_pending", "A friend request between you and this user is already pending")
	case errors.Is(err, friend.ErrNotPending):
		writeError(w, http.StatusConflict, "not_pending", "This friend request has already been answered")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong, please try again")
	}
}
