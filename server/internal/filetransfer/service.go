package filetransfer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"strings"

	storagepkg "github.com/itswskkk/KMJG-Hub/server/internal/storage"
)

// NotificationEventType is the persistent notification kind created for a
// recipient when a transfer is requested (docs/PRD.md § Notifications).
const NotificationEventType = "file_transfer_request"

// Notifier creates persistent notifications. Satisfied by
// *notification.Service; declared here so this package does not import it.
type Notifier interface {
	Notify(ctx context.Context, userID, eventType string, payload any) error
}

// Service implements Direct File Transfer use cases. Who may act on a
// transfer, and in which state, is enforced by Repository inside
// PostgreSQL; Service validates input, guards the upload path so no bytes
// reach storage for a transfer that has not been accepted, and publishes
// real-time events and notifications.
type Service struct {
	Repo      Repository
	Storage   FileStore
	Publisher Publisher // optional
	Notifier  Notifier  // optional; nil disables notifications
	// MaxUploadBytes is the administrator's per-file limit
	// (KMJG_MAX_UPLOAD_BYTES). Transfers never count against Project
	// storage quotas.
	MaxUploadBytes int64
}

func (s *Service) CreateRequest(ctx context.Context, senderID, recipientID, fileName string, declaredSize int64) (*Transfer, error) {
	recipientID = strings.TrimSpace(recipientID)
	if recipientID == "" {
		return nil, &ValidationError{Field: "recipient_id", Message: "Recipient is required"}
	}
	if recipientID == senderID {
		return nil, &ValidationError{Field: "recipient_id", Message: "You cannot send a file to yourself"}
	}
	fileName, err := validateFileName(fileName)
	if err != nil {
		return nil, err
	}
	if err := validateFileSize(declaredSize, s.MaxUploadBytes); err != nil {
		return nil, err
	}
	t, err := s.Repo.Create(ctx, senderID, recipientID, fileName, declaredSize)
	if err != nil {
		return nil, err
	}
	if s.Publisher != nil {
		s.Publisher.PublishTransferRequested(*t)
	}
	if s.Notifier != nil {
		if err := s.Notifier.Notify(ctx, t.RecipientID, NotificationEventType, map[string]any{
			"transfer_id":     t.ID,
			"sender_id":       t.SenderID,
			"sender_username": t.SenderUsername,
			"file_name":       t.FileName,
			"file_size":       t.DeclaredFileSize,
		}); err != nil {
			slog.Warn("filetransfer: create notification", "error", err)
		}
	}
	return t, nil
}

func (s *Service) Accept(ctx context.Context, transferID, recipientID string) (*Transfer, error) {
	t, err := s.Repo.Accept(ctx, transferID, recipientID)
	if err != nil {
		return nil, err
	}
	if s.Publisher != nil {
		s.Publisher.PublishTransferResponded(*t)
	}
	return t, nil
}

func (s *Service) Decline(ctx context.Context, transferID, recipientID string) (*Transfer, error) {
	t, err := s.Repo.Decline(ctx, transferID, recipientID)
	if err != nil {
		return nil, err
	}
	if s.Publisher != nil {
		s.Publisher.PublishTransferResponded(*t)
	}
	return t, nil
}

func (s *Service) Cancel(ctx context.Context, transferID, senderID string) (*Transfer, error) {
	t, err := s.Repo.Cancel(ctx, transferID, senderID)
	if err != nil {
		return nil, err
	}
	if s.Publisher != nil {
		s.Publisher.PublishTransferCancelled(*t)
	}
	return t, nil
}

// Upload stores the file for an accepted transfer. The state and actor are
// checked before any byte is written, so declined, cancelled or still
// pending transfers never cause server-side storage (docs/PRD.md "The file
// must not be uploaded to server storage before the recipient accepts").
// MarkUploaded re-checks both atomically; if it loses a race (e.g. a
// concurrent cancel) the stored object is removed again.
func (s *Service) Upload(ctx context.Context, transferID, senderID string, content io.Reader, actualSize int64, contentType string) (*Transfer, error) {
	t, err := s.Repo.Get(ctx, transferID, senderID)
	if err != nil {
		return nil, err
	}
	if t.SenderID != senderID {
		return nil, ErrForbidden
	}
	if t.Status != StatusAccepted {
		return nil, ErrInvalidState
	}
	if err := validateFileSize(actualSize, s.MaxUploadBytes); err != nil {
		return nil, err
	}
	if actualSize != t.DeclaredFileSize {
		return nil, ErrSizeMismatch
	}
	contentType = normalizeContentType(contentType, t.FileName)

	storageID, err := newStorageID()
	if err != nil {
		return nil, err
	}
	if err := s.Storage.Put(ctx, storageID, content, actualSize); err != nil {
		return nil, err
	}
	uploaded, err := s.Repo.MarkUploaded(ctx, transferID, senderID, storageID, actualSize, contentType)
	if err != nil {
		_ = s.Storage.Delete(context.Background(), storageID)
		return nil, err
	}
	if s.Publisher != nil {
		s.Publisher.PublishTransferUploaded(*uploaded)
	}
	return uploaded, nil
}

// Download opens an uploaded transfer's content. Only the recipient may
// download; the caller must close the returned reader.
func (s *Service) Download(ctx context.Context, transferID, userID string) (io.ReadCloser, *Transfer, error) {
	t, err := s.Repo.Get(ctx, transferID, userID)
	if err != nil {
		return nil, nil, err
	}
	if t.RecipientID != userID {
		return nil, nil, ErrForbidden
	}
	if t.Status != StatusUploaded || t.StorageID == nil {
		return nil, nil, ErrInvalidState
	}
	reader, err := s.Storage.Open(ctx, *t.StorageID)
	if err != nil {
		if errors.Is(err, storagepkg.ErrNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	return reader, t, nil
}

func (s *Service) Get(ctx context.Context, transferID, userID string) (*Transfer, error) {
	return s.Repo.Get(ctx, transferID, userID)
}

func (s *Service) ListIncoming(ctx context.Context, userID string) ([]Transfer, error) {
	return s.Repo.ListIncoming(ctx, userID)
}

func (s *Service) ListSent(ctx context.Context, userID string) ([]Transfer, error) {
	return s.Repo.ListSent(ctx, userID)
}

// newStorageID returns an opaque random identifier, the same form Project
// Chat attachments use; the "ft-" prefix keeps the two logically separate
// in the shared storage root.
func newStorageID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "ft-" + hex.EncodeToString(buf), nil
}
