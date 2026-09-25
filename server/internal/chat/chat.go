// Package chat implements the persistent, Project-scoped conversation
// described by docs/PRD.md "Project Communication Model".
package chat

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	// ErrNotFound deliberately covers both a missing resource and a caller
	// who is not a Project member, so private Project existence is not leaked.
	ErrNotFound     = errors.New("chat: not found")
	ErrForbidden    = errors.New("chat: forbidden")
	ErrStorageLimit = errors.New("chat: storage limit exceeded")
)

type Attachment struct {
	ID          string `json:"id"`
	MessageID   string `json:"message_id"`
	ProjectID   string `json:"project_id"`
	StorageID   string `json:"-"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

// Message kinds. KindUser messages are authored by a Project member;
// every other kind is Server-generated activity with no author (AuthorID
// and AuthorUsername are empty) that ordinary deletion does not apply to.
const (
	KindUser = "user"
	KindGit  = "git"
)

// Message is one active Project Chat message. Soft-deleted content is never
// returned through this normal Client-facing model.
type Message struct {
	ID             string
	ProjectID      string
	Kind           string
	AuthorID       string // empty for Server-generated messages
	AuthorUsername string
	Body           string
	CreatedAt      time.Time
	Attachments    []Attachment
}

// ProjectFile is one Project Chat attachment as listed by the Files
// section, with who shared it and when.
type ProjectFile struct {
	Attachment
	AuthorID       string
	AuthorUsername string
	CreatedAt      time.Time
}

type Cursor struct {
	CreatedAt time.Time
	ID        string
}

type Page struct {
	Messages   []Message
	NextCursor string
}

// Repository stores authoritative Project Chat history.
type Repository interface {
	Create(ctx context.Context, projectID, authorID, body string) (*Message, error)
	ListPage(ctx context.Context, projectID, viewerID string, before *Cursor, limit int) ([]Message, error)
	// CreateSystem stores a Server-generated message of the given
	// (non-user) kind. There is no author and no membership check; it
	// returns ErrNotFound when the Project does not exist or is deleted.
	CreateSystem(ctx context.Context, projectID, kind, body string) (*Message, error)
	CreateWithAttachment(ctx context.Context, projectID, authorID, body string, attachment Attachment, maxProjectBytes int64) (*Message, error)
	// ListFiles returns every attachment of projectID's active (not
	// deleted) messages, newest first, or ErrNotFound when viewerID is not
	// a member.
	ListFiles(ctx context.Context, projectID, viewerID string) ([]ProjectFile, error)
	GetAttachment(ctx context.Context, projectID, attachmentID, viewerID string) (*Attachment, error)
	ListExpiredAttachmentStorageIDs(ctx context.Context, cutoff time.Time) ([]string, error)
	SoftDelete(ctx context.Context, projectID, messageID, actorID string) (*Message, error)
	PurgeDeletedBefore(ctx context.Context, cutoff time.Time) error
}

type FileStore interface {
	Put(ctx context.Context, id string, src io.Reader, size int64) error
	Open(ctx context.Context, id string) (io.ReadCloser, error)
	Delete(ctx context.Context, id string) error
}

// Membership supplies the current broadcast audience from Server-owned
// Project membership rather than any recipient list supplied by a Client.
type Membership interface {
	MemberUserIDs(ctx context.Context, projectID string) ([]string, error)
}

// Publisher is the narrow real-time capability Chat needs.
type Publisher interface {
	PublishMessageCreated(userID string, message Message)
	PublishMessageDeleted(userID, projectID, messageID string)
}

// ValidationError is safe to expose to API clients.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }
