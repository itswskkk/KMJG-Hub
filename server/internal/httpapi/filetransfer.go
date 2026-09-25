package httpapi

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/filetransfer"
)

type fileTransferDTO struct {
	ID                string     `json:"id"`
	SenderID          string     `json:"sender_id"`
	SenderUsername    string     `json:"sender_username"`
	RecipientID       string     `json:"recipient_id"`
	RecipientUsername string     `json:"recipient_username"`
	FileName          string     `json:"file_name"`
	FileSize          int64      `json:"file_size"`
	Status            string     `json:"status"`
	ContentType       *string    `json:"content_type"`
	CreatedAt         time.Time  `json:"created_at"`
	RespondedAt       *time.Time `json:"responded_at"`
	UploadedAt        *time.Time `json:"uploaded_at"`
}

// toFileTransferDTO never exposes the opaque storage identifier.
func toFileTransferDTO(t filetransfer.Transfer) fileTransferDTO {
	return fileTransferDTO{
		ID: t.ID, SenderID: t.SenderID, SenderUsername: t.SenderUsername,
		RecipientID: t.RecipientID, RecipientUsername: t.RecipientUsername,
		FileName: t.FileName, FileSize: t.DeclaredFileSize, Status: string(t.Status),
		ContentType: t.ContentType, CreatedAt: t.CreatedAt, RespondedAt: t.RespondedAt, UploadedAt: t.UploadedAt,
	}
}

func toFileTransferDTOs(items []filetransfer.Transfer) []fileTransferDTO {
	out := make([]fileTransferDTO, len(items))
	for i, t := range items {
		out[i] = toFileTransferDTO(t)
	}
	return out
}

type createFileTransferRequest struct {
	RecipientID string `json:"recipient_id"`
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
}

func (h *Handlers) handleCreateFileTransfer(w http.ResponseWriter, r *http.Request) {
	var req createFileTransferRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body must be valid JSON matching the expected fields")
		return
	}
	t, err := h.FileTransfers.CreateRequest(r.Context(), currentAuth(r).User.ID, req.RecipientID, req.FileName, req.FileSize)
	if err != nil {
		writeFileTransferError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toFileTransferDTO(*t))
}

func (h *Handlers) handleListIncomingFileTransfers(w http.ResponseWriter, r *http.Request) {
	items, err := h.FileTransfers.ListIncoming(r.Context(), currentAuth(r).User.ID)
	if err != nil {
		writeFileTransferError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transfers": toFileTransferDTOs(items), "max_upload_bytes": h.FileTransfers.MaxUploadBytes})
}

func (h *Handlers) handleListSentFileTransfers(w http.ResponseWriter, r *http.Request) {
	items, err := h.FileTransfers.ListSent(r.Context(), currentAuth(r).User.ID)
	if err != nil {
		writeFileTransferError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transfers": toFileTransferDTOs(items), "max_upload_bytes": h.FileTransfers.MaxUploadBytes})
}

func (h *Handlers) handleAcceptFileTransfer(w http.ResponseWriter, r *http.Request) {
	h.respondFileTransfer(w, r, h.FileTransfers.Accept)
}

func (h *Handlers) handleDeclineFileTransfer(w http.ResponseWriter, r *http.Request) {
	h.respondFileTransfer(w, r, h.FileTransfers.Decline)
}

func (h *Handlers) handleCancelFileTransfer(w http.ResponseWriter, r *http.Request) {
	h.respondFileTransfer(w, r, h.FileTransfers.Cancel)
}

func (h *Handlers) respondFileTransfer(w http.ResponseWriter, r *http.Request, action func(ctx context.Context, transferID, userID string) (*filetransfer.Transfer, error)) {
	t, err := action(r.Context(), r.PathValue("id"), currentAuth(r).User.ID)
	if err != nil {
		writeFileTransferError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toFileTransferDTO(*t))
}

// handleUploadFileTransfer accepts the file as multipart/form-data field
// "file", the same shape as Project Chat attachment uploads. The body is
// capped at the per-file limit (plus multipart overhead) before parsing.
func (h *Handlers) handleUploadFileTransfer(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.FileTransfers.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "upload_too_large", "File exceeds the configured upload limit")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "validation_error", "File is required", "file")
		return
	}
	defer file.Close()
	t, err := h.FileTransfers.Upload(r.Context(), r.PathValue("id"), currentAuth(r).User.ID, file, header.Size, header.Header.Get("Content-Type"))
	if err != nil {
		writeFileTransferError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toFileTransferDTO(*t))
}

func (h *Handlers) handleDownloadFileTransfer(w http.ResponseWriter, r *http.Request) {
	reader, t, err := h.FileTransfers.Download(r.Context(), r.PathValue("id"), currentAuth(r).User.ID)
	if err != nil {
		writeFileTransferError(w, err)
		return
	}
	defer reader.Close()
	contentType := "application/octet-stream"
	if t.ContentType != nil && *t.ContentType != "" {
		contentType = *t.ContentType
	}
	size := t.DeclaredFileSize
	if t.ActualFileSize != nil {
		size = *t.ActualFileSize
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": t.FileName}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, reader)
}

func writeFileTransferError(w http.ResponseWriter, err error) {
	var validationErr *filetransfer.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeFieldError(w, http.StatusBadRequest, "validation_error", validationErr.Message, validationErr.Field)
	case errors.Is(err, filetransfer.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "File transfer or user not found")
	case errors.Is(err, filetransfer.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to perform this file transfer action")
	case errors.Is(err, filetransfer.ErrInvalidState):
		writeError(w, http.StatusConflict, "invalid_state", "This file transfer is not in a state that allows this action")
	case errors.Is(err, filetransfer.ErrSizeLimitExceeded):
		writeFieldError(w, http.StatusRequestEntityTooLarge, "upload_too_large", "File exceeds the configured upload limit", "file_size")
	case errors.Is(err, filetransfer.ErrSizeMismatch):
		writeFieldError(w, http.StatusBadRequest, "size_mismatch", "Uploaded file size does not match the size in the transfer request", "file")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong, please try again")
	}
}
