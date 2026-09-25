// Package filetransfer implements KMJG Hub Direct File Transfer: a sender
// requests to send a file to a recipient, who must explicitly Accept or
// Decline before any bytes are uploaded to server storage, per
// docs/PRD.md "File Sharing and Transfer" § Direct File Transfer.
//
// This is deliberately separate from Project Chat attachments
// (internal/chat): a Direct File Transfer is a one-to-one exchange, is never
// counted against a Project's storage quota, and the file is never stored
// unless the recipient accepts.
package filetransfer

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrNotFound          = errors.New("filetransfer: not found")
	ErrForbidden         = errors.New("filetransfer: forbidden")
	ErrInvalidState      = errors.New("filetransfer: invalid state for this action")
	ErrSizeLimitExceeded = errors.New("filetransfer: file size exceeds the configured limit")
	ErrSizeMismatch      = errors.New("filetransfer: uploaded size does not match the declared size")
)

// Status is the lifecycle state of a file transfer.
type Status string

const (
	StatusPending   Status = "pending"
	StatusAccepted  Status = "accepted"
	StatusDeclined  Status = "declined"
	StatusUploaded  Status = "uploaded"
	StatusCancelled Status = "cancelled"
	StatusExpired   Status = "expired"
)

// Transfer is one Direct File Transfer request/exchange.
type Transfer struct {
	ID                string
	SenderID          string
	SenderUsername    string
	RecipientID       string
	RecipientUsername string
	FileName          string
	DeclaredFileSize  int64
	Status            Status
	StorageID         *string
	ActualFileSize    *int64
	ContentType       *string
	CreatedAt         time.Time
	RespondedAt       *time.Time
	UploadedAt        *time.Time
}

// Repository persists file transfer requests and their state transitions.
type Repository interface {
	// Create records a new pending transfer request. recipientID must exist
	// (else ErrNotFound). The pair must be "permitted users" — the same rule
	// as Direct Messages: an accepted friendship or a shared Project, and
	// neither has blocked the other — enforced in the same statement, else
	// ErrForbidden. Does not touch file storage.
	Create(ctx context.Context, senderID, recipientID, fileName string, declaredSize int64) (*Transfer, error)

	// Get returns one transfer if userID is the sender or recipient, else ErrNotFound.
	Get(ctx context.Context, transferID, userID string) (*Transfer, error)

	// ListIncoming returns userID's received transfer requests, newest first.
	ListIncoming(ctx context.Context, userID string) ([]Transfer, error)

	// ListSent returns userID's sent transfer requests, newest first.
	ListSent(ctx context.Context, userID string) ([]Transfer, error)

	// Accept transitions a pending transfer to accepted. Recipient only;
	// ErrForbidden otherwise. ErrInvalidState if not currently pending.
	Accept(ctx context.Context, transferID, recipientID string) (*Transfer, error)

	// Decline transitions a pending transfer to declined. Recipient only;
	// ErrForbidden otherwise. ErrInvalidState if not currently pending.
	Decline(ctx context.Context, transferID, recipientID string) (*Transfer, error)

	// Cancel transitions a pending or accepted (not yet uploaded) transfer
	// to cancelled, per docs/PRD.md "allow cancellation while a transfer is
	// active". Sender only.
	Cancel(ctx context.Context, transferID, senderID string) (*Transfer, error)

	// MarkUploaded records the storage ID and actual size after a
	// successful upload. Only valid on an accepted transfer belonging to
	// senderID; ErrInvalidState otherwise.
	MarkUploaded(ctx context.Context, transferID, senderID, storageID string, actualSize int64, contentType string) (*Transfer, error)
}

// FileStore is the storage backend for accepted transfer uploads. Same
// capability internal/chat uses for attachments (internal/storage).
type FileStore interface {
	Put(ctx context.Context, id string, src io.Reader, size int64) error
	Open(ctx context.Context, id string) (io.ReadCloser, error)
	Delete(ctx context.Context, id string) error
}

// Publisher delivers real-time transfer events to the relevant party.
type Publisher interface {
	PublishTransferRequested(t Transfer) // to recipient
	PublishTransferResponded(t Transfer) // to sender (accepted/declined)
	PublishTransferCancelled(t Transfer) // to recipient
	PublishTransferUploaded(t Transfer)  // to recipient (ready to download)
}

// ValidationError is safe to expose to API clients.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
