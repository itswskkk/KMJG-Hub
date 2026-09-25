package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/chat"
)

type projectMessageDTO struct {
	ID             string            `json:"id"`
	ProjectID      string            `json:"project_id"`
	Kind           string            `json:"kind"`
	AuthorID       string            `json:"author_id"`
	AuthorUsername string            `json:"author_username"`
	Body           string            `json:"body"`
	CreatedAt      time.Time         `json:"created_at"`
	Attachments    []chat.Attachment `json:"attachments"`
}

func toProjectMessageDTO(message chat.Message) projectMessageDTO {
	return projectMessageDTO{
		ID: message.ID, ProjectID: message.ProjectID, Kind: message.Kind, AuthorID: message.AuthorID,
		AuthorUsername: message.AuthorUsername, Body: message.Body, CreatedAt: message.CreatedAt, Attachments: message.Attachments,
	}
}

func (h *Handlers) handleListProjectMessages(w http.ResponseWriter, r *http.Request) {
	page, err := h.Chat.ListPage(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), r.URL.Query().Get("cursor"))
	if err != nil {
		writeChatError(w, err)
		return
	}
	dtos := make([]projectMessageDTO, len(page.Messages))
	for i, message := range page.Messages {
		dtos[i] = toProjectMessageDTO(message)
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": dtos, "next_cursor": page.NextCursor})
}

type sendProjectMessageRequest struct {
	Body string `json:"body"`
}

func (h *Handlers) handleSendProjectMessage(w http.ResponseWriter, r *http.Request) {
	var req sendProjectMessageRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}
	message, err := h.Chat.Send(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), req.Body)
	if err != nil {
		writeChatError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toProjectMessageDTO(*message))
}

func (h *Handlers) handleDeleteProjectMessage(w http.ResponseWriter, r *http.Request) {
	err := h.Chat.Delete(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), r.PathValue("messageID"))
	if err != nil {
		writeChatError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) handleUploadProjectAttachment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.Chat.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "upload_too_large", "Attachment exceeds the configured upload limit")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Attachment file is required")
		return
	}
	defer file.Close()
	message, err := h.Chat.SendAttachment(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), r.FormValue("body"), header.Filename, header.Header.Get("Content-Type"), header.Size, file)
	if err != nil {
		writeChatError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toProjectMessageDTO(*message))
}

type projectFileDTO struct {
	ID             string    `json:"id"`
	MessageID      string    `json:"message_id"`
	ProjectID      string    `json:"project_id"`
	Filename       string    `json:"filename"`
	ContentType    string    `json:"content_type"`
	SizeBytes      int64     `json:"size_bytes"`
	AuthorID       string    `json:"author_id"`
	AuthorUsername string    `json:"author_username"`
	CreatedAt      time.Time `json:"created_at"`
}

func toProjectFileDTO(f chat.ProjectFile) projectFileDTO {
	return projectFileDTO{
		ID: f.ID, MessageID: f.MessageID, ProjectID: f.ProjectID, Filename: f.Filename,
		ContentType: f.ContentType, SizeBytes: f.SizeBytes, AuthorID: f.AuthorID,
		AuthorUsername: f.AuthorUsername, CreatedAt: f.CreatedAt,
	}
}

// handleListProjectAttachments backs the Files section: every file shared
// through the Project's chat, newest first.
func (h *Handlers) handleListProjectAttachments(w http.ResponseWriter, r *http.Request) {
	files, err := h.Chat.ListAttachments(r.Context(), currentAuth(r).User.ID, r.PathValue("id"))
	if err != nil {
		writeChatError(w, err)
		return
	}
	dtos := make([]projectFileDTO, len(files))
	for i, f := range files {
		dtos[i] = toProjectFileDTO(f)
	}
	writeJSON(w, http.StatusOK, map[string]any{"attachments": dtos})
}

func (h *Handlers) handleDownloadProjectAttachment(w http.ResponseWriter, r *http.Request) {
	attachment, reader, err := h.Chat.OpenAttachment(r.Context(), currentAuth(r).User.ID, r.PathValue("id"), r.PathValue("attachmentID"))
	if err != nil {
		writeChatError(w, err)
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", attachment.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(attachment.SizeBytes, 10))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": attachment.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, reader)
}

func writeChatError(w http.ResponseWriter, err error) {
	var validationErr *chat.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeFieldError(w, http.StatusBadRequest, "validation_error", validationErr.Message, validationErr.Field)
	case errors.Is(err, chat.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Project or message not found")
	case errors.Is(err, chat.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to delete this message")
	case errors.Is(err, chat.ErrStorageLimit):
		writeError(w, http.StatusRequestEntityTooLarge, "storage_limit", "Project attachment storage limit would be exceeded")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong, please try again")
	}
}
