package chat

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	storagepkg "github.com/itswskkk/KMJG-Hub/server/internal/storage"
)

// MaxMessageCharacters bounds persistent and real-time message payloads.
// ASSUMPTION A-260925-4: product maximum is not specified; see ASSUMPTIONS.md.
const MaxMessageCharacters = 4000

// RecentMessageLimit bounds initial history retrieval.
// ASSUMPTION A-260925-5: product page size is not specified; see ASSUMPTIONS.md.
const RecentMessageLimit = 50

// DeletedMessageRetention is required by docs/PRD.md "Deleted Messages".
const DeletedMessageRetention = 30 * 24 * time.Hour

type Service struct {
	Repo                   Repository
	Membership             Membership
	Publisher              Publisher
	Storage                FileStore
	MaxUploadBytes         int64
	MaxProjectStorageBytes int64
}

func (s *Service) ListPage(ctx context.Context, userID, projectID, encodedCursor string) (*Page, error) {
	var before *Cursor
	if encodedCursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(encodedCursor)
		if err != nil {
			return nil, &ValidationError{Field: "cursor", Message: "Cursor is invalid"}
		}
		var value Cursor
		if json.Unmarshal(decoded, &value) != nil || value.ID == "" || value.CreatedAt.IsZero() {
			return nil, &ValidationError{Field: "cursor", Message: "Cursor is invalid"}
		}
		before = &value
	}
	messages, err := s.Repo.ListPage(ctx, projectID, userID, before, RecentMessageLimit+1)
	if err != nil {
		return nil, err
	}
	page := &Page{Messages: messages}
	if len(messages) > RecentMessageLimit {
		page.Messages = messages[1:]
		cursor := Cursor{CreatedAt: page.Messages[0].CreatedAt, ID: page.Messages[0].ID}
		raw, _ := json.Marshal(cursor)
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return page, nil
}

func (s *Service) Send(ctx context.Context, userID, projectID, body string) (*Message, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, &ValidationError{Field: "body", Message: "Message cannot be empty"}
	}
	if !utf8.ValidString(body) || utf8.RuneCountInString(body) > MaxMessageCharacters {
		return nil, &ValidationError{Field: "body", Message: "Message must be 4,000 characters or fewer"}
	}

	message, err := s.Repo.Create(ctx, projectID, userID, body)
	if err != nil {
		return nil, err
	}
	s.publishCreated(ctx, *message)
	return message, nil
}

func (s *Service) SendAttachment(ctx context.Context, userID, projectID, body, filename, contentType string, size int64, src io.Reader) (*Message, error) {
	if s.Membership != nil {
		memberIDs, err := s.Membership.MemberUserIDs(ctx, projectID)
		if err != nil {
			return nil, ErrNotFound
		}
		isMember := false
		for _, memberID := range memberIDs {
			if memberID == userID {
				isMember = true
				break
			}
		}
		if !isMember {
			return nil, ErrNotFound
		}
	}
	body = strings.TrimSpace(body)
	filename = filepath.Base(strings.TrimSpace(filename))
	contentType = strings.TrimSpace(contentType)
	if filename == "" || filename == "." || utf8.RuneCountInString(filename) > 255 {
		return nil, &ValidationError{Field: "file", Message: "Attachment filename is invalid"}
	}
	if size < 0 || size > s.MaxUploadBytes {
		return nil, &ValidationError{Field: "file", Message: "Attachment exceeds the configured upload limit"}
	}
	if body != "" && (!utf8.ValidString(body) || utf8.RuneCountInString(body) > MaxMessageCharacters) {
		return nil, &ValidationError{Field: "body", Message: "Message must be 4,000 characters or fewer"}
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(filename))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if len(contentType) > 255 {
		return nil, &ValidationError{Field: "file", Message: "Attachment content type is invalid"}
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	storageID := hex.EncodeToString(buf)
	if err := s.Storage.Put(ctx, storageID, src, size); err != nil {
		return nil, err
	}
	attachment := Attachment{StorageID: storageID, Filename: filename, ContentType: contentType, SizeBytes: size}
	message, err := s.Repo.CreateWithAttachment(ctx, projectID, userID, body, attachment, s.MaxProjectStorageBytes)
	if err != nil {
		_ = s.Storage.Delete(context.Background(), storageID)
		return nil, err
	}
	s.publishCreated(ctx, *message)
	return message, nil
}

func (s *Service) OpenAttachment(ctx context.Context, userID, projectID, attachmentID string) (*Attachment, io.ReadCloser, error) {
	attachment, err := s.Repo.GetAttachment(ctx, projectID, attachmentID, userID)
	if err != nil {
		return nil, nil, err
	}
	reader, err := s.Storage.Open(ctx, attachment.StorageID)
	if err != nil {
		if errors.Is(err, storagepkg.ErrNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	return attachment, reader, nil
}

func (s *Service) Delete(ctx context.Context, userID, projectID, messageID string) error {
	message, err := s.Repo.SoftDelete(ctx, projectID, messageID, userID)
	if err != nil {
		return err
	}
	s.publishDeleted(ctx, message.ProjectID, message.ID)
	return nil
}

// PurgeExpiredDeleted permanently removes messages whose soft-deletion
// retention window has ended. It is an operator lifecycle action and never
// exposes deleted content to normal Project members.
func (s *Service) PurgeExpiredDeleted(ctx context.Context, now time.Time) error {
	cutoff := now.Add(-DeletedMessageRetention)
	if s.Storage == nil {
		return s.Repo.PurgeDeletedBefore(ctx, cutoff)
	}
	ids, err := s.Repo.ListExpiredAttachmentStorageIDs(ctx, cutoff)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.Storage.Delete(ctx, id); err != nil && !errors.Is(err, storagepkg.ErrNotFound) {
			return err
		}
	}
	return s.Repo.PurgeDeletedBefore(ctx, cutoff)
}

func (s *Service) publishCreated(ctx context.Context, message Message) {
	if s.Membership == nil || s.Publisher == nil {
		return
	}
	userIDs, err := s.Membership.MemberUserIDs(ctx, message.ProjectID)
	if err != nil {
		return
	}
	for _, userID := range userIDs {
		s.Publisher.PublishMessageCreated(userID, message)
	}
}

func (s *Service) publishDeleted(ctx context.Context, projectID, messageID string) {
	if s.Membership == nil || s.Publisher == nil {
		return
	}
	userIDs, err := s.Membership.MemberUserIDs(ctx, projectID)
	if err != nil {
		return
	}
	for _, userID := range userIDs {
		s.Publisher.PublishMessageDeleted(userID, projectID, messageID)
	}
}
