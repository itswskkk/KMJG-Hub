package httpapi_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itswskkk/KMJG-Hub/server/internal/chat"
	"github.com/itswskkk/KMJG-Hub/server/internal/project"
)

type fakeChatRepo struct {
	mu       sync.Mutex
	nextID   int
	messages map[string][]chat.Message
	users    usernameLookup
	projects *fakeProjectRepo
}

func newFakeChatRepo(users usernameLookup, projects *fakeProjectRepo) *fakeChatRepo {
	return &fakeChatRepo{messages: make(map[string][]chat.Message), users: users, projects: projects}
}

func (f *fakeChatRepo) role(projectID, userID string) (project.Role, bool) {
	f.projects.mu.Lock()
	defer f.projects.mu.Unlock()
	for _, member := range f.projects.members[projectID] {
		if member.UserID == userID {
			return member.Role, true
		}
	}
	return "", false
}

func (f *fakeChatRepo) Create(_ context.Context, projectID, authorID, body string) (*chat.Message, error) {
	if _, ok := f.role(projectID, authorID); !ok {
		return nil, chat.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	message := chat.Message{ID: fmt.Sprintf("message-%d", f.nextID), ProjectID: projectID, Kind: chat.KindUser, AuthorID: authorID, AuthorUsername: f.users.usernameFor(authorID), Body: body, CreatedAt: time.Now().UTC()}
	f.messages[projectID] = append(f.messages[projectID], message)
	copy := message
	return &copy, nil
}

func (f *fakeChatRepo) ListPage(_ context.Context, projectID, viewerID string, _ *chat.Cursor, limit int) ([]chat.Message, error) {
	if _, ok := f.role(projectID, viewerID); !ok {
		return nil, chat.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	messages := f.messages[projectID]
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	return append([]chat.Message(nil), messages...), nil
}

// CreateWithAttachment mirrors the SQL repository: members only, and the
// Project's total active attachment bytes may not exceed maxProjectBytes.
func (f *fakeChatRepo) CreateWithAttachment(_ context.Context, projectID, authorID, body string, attachment chat.Attachment, maxProjectBytes int64) (*chat.Message, error) {
	if _, ok := f.role(projectID, authorID); !ok {
		return nil, chat.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var used int64
	for _, m := range f.messages[projectID] {
		for _, a := range m.Attachments {
			used += a.SizeBytes
		}
	}
	if used+attachment.SizeBytes > maxProjectBytes {
		return nil, chat.ErrStorageLimit
	}
	f.nextID++
	messageID := fmt.Sprintf("message-%d", f.nextID)
	attachment.ID = fmt.Sprintf("attachment-%d", f.nextID)
	attachment.MessageID = messageID
	attachment.ProjectID = projectID
	message := chat.Message{ID: messageID, ProjectID: projectID, Kind: chat.KindUser, AuthorID: authorID, AuthorUsername: f.users.usernameFor(authorID), Body: body, CreatedAt: time.Now().UTC(), Attachments: []chat.Attachment{attachment}}
	f.messages[projectID] = append(f.messages[projectID], message)
	copy := message
	return &copy, nil
}

func (f *fakeChatRepo) CreateSystem(_ context.Context, projectID, kind, body string) (*chat.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	message := chat.Message{ID: fmt.Sprintf("message-%d", f.nextID), ProjectID: projectID, Kind: kind, Body: body, CreatedAt: time.Now().UTC()}
	f.messages[projectID] = append(f.messages[projectID], message)
	copy := message
	return &copy, nil
}

// ListFiles mirrors the SQL repository: members only, attachments of
// active messages, newest first.
func (f *fakeChatRepo) ListFiles(_ context.Context, projectID, viewerID string) ([]chat.ProjectFile, error) {
	if _, ok := f.role(projectID, viewerID); !ok {
		return nil, chat.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	files := make([]chat.ProjectFile, 0)
	messages := f.messages[projectID]
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		for j := len(m.Attachments) - 1; j >= 0; j-- {
			files = append(files, chat.ProjectFile{Attachment: m.Attachments[j], AuthorID: m.AuthorID, AuthorUsername: m.AuthorUsername, CreatedAt: m.CreatedAt})
		}
	}
	return files, nil
}

func (f *fakeChatRepo) GetAttachment(_ context.Context, projectID, attachmentID, viewerID string) (*chat.Attachment, error) {
	if _, ok := f.role(projectID, viewerID); !ok {
		return nil, chat.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.messages[projectID] {
		for _, a := range m.Attachments {
			if a.ID == attachmentID {
				copy := a
				return &copy, nil
			}
		}
	}
	return nil, chat.ErrNotFound
}
func (f *fakeChatRepo) ListExpiredAttachmentStorageIDs(context.Context, time.Time) ([]string, error) {
	return nil, nil
}

func (f *fakeChatRepo) SoftDelete(_ context.Context, projectID, messageID, actorID string) (*chat.Message, error) {
	role, member := f.role(projectID, actorID)
	if !member {
		return nil, chat.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, message := range f.messages[projectID] {
		if message.ID != messageID {
			continue
		}
		if message.Kind != chat.KindUser || (message.AuthorID != actorID && role != project.RoleOwner && role != project.RoleAdmin) {
			return nil, chat.ErrForbidden
		}
		f.messages[projectID] = append(f.messages[projectID][:i], f.messages[projectID][i+1:]...)
		copy := message
		return &copy, nil
	}
	return nil, chat.ErrNotFound
}

func (f *fakeChatRepo) PurgeDeletedBefore(context.Context, time.Time) error { return nil }

func TestProjectChatSendListAndDeletePermissions(t *testing.T) {
	router, _, projects := newTestRouterWithHandlers()
	ownerToken := registerAndToken(t, router, "owner")
	memberToken := registerAndToken(t, router, "member")
	otherToken := registerAndToken(t, router, "other")

	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Chat Project"}, ownerToken)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)
	memberID := currentUserID(t, router, memberToken)
	otherID := currentUserID(t, router, otherToken)
	projects.addMember(p.ID, memberID, project.RoleMember)
	projects.addMember(p.ID, otherID, project.RoleMember)

	sent := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/chat/messages", map[string]string{"body": "  hello team  "}, memberToken)
	if sent.Code != http.StatusCreated {
		t.Fatalf("send: %d %s", sent.Code, sent.Body.String())
	}
	var message struct {
		ID       string `json:"id"`
		Body     string `json:"body"`
		AuthorID string `json:"author_id"`
	}
	decodeJSON(t, sent, &message)
	if message.Body != "hello team" || message.AuthorID != memberID {
		t.Fatalf("unexpected message: %+v (raw: %s)", message, sent.Body.String())
	}

	listed := doJSON(t, router, http.MethodGet, "/api/v1/projects/"+p.ID+"/chat/messages", nil, ownerToken)
	if listed.Code != http.StatusOK {
		t.Fatalf("list: %d %s", listed.Code, listed.Body.String())
	}
	var body struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	decodeJSON(t, listed, &body)
	if len(body.Messages) != 1 || body.Messages[0].ID != message.ID {
		t.Fatalf("unexpected history: %+v", body.Messages)
	}

	forbidden := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+p.ID+"/chat/messages/"+message.ID, nil, otherToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("member deleting another message: %d %s", forbidden.Code, forbidden.Body.String())
	}
	deleted := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+p.ID+"/chat/messages/"+message.ID, nil, ownerToken)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("owner moderation delete: %d %s", deleted.Code, deleted.Body.String())
	}

	listed = doJSON(t, router, http.MethodGet, "/api/v1/projects/"+p.ID+"/chat/messages", nil, memberToken)
	decodeJSON(t, listed, &body)
	if len(body.Messages) != 0 {
		t.Fatalf("deleted message still visible: %+v", body.Messages)
	}
}

func TestProjectChatRejectsNonMembersAndInvalidMessages(t *testing.T) {
	router, _, _ := newTestRouterWithHandlers()
	ownerToken := registerAndToken(t, router, "owner2")
	outsiderToken := registerAndToken(t, router, "outsider")
	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Private Chat"}, ownerToken)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)

	if rec := doJSON(t, router, http.MethodGet, "/api/v1/projects/"+p.ID+"/chat/messages", nil, outsiderToken); rec.Code != http.StatusNotFound {
		t.Fatalf("non-member list: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/chat/messages", map[string]string{"body": "   "}, ownerToken); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty send: %d %s", rec.Code, rec.Body.String())
	}
}

func TestProjectChatRealtimeEventsReachCurrentMembersOnly(t *testing.T) {
	router, _, projects := newTestRouterWithHandlers()
	server := httptest.NewServer(router)
	defer server.Close()

	ownerToken := registerAndToken(t, router, "chatowner")
	memberToken := registerAndToken(t, router, "chatmember")
	outsiderToken := registerAndToken(t, router, "chatoutsider")
	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Realtime Chat"}, ownerToken)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)
	projects.addMember(p.ID, currentUserID(t, router, memberToken), project.RoleMember)

	memberConn := dial(t, server)
	if err := memberConn.WriteJSON(map[string]any{"type": "auth", "data": map[string]string{"token": memberToken}}); err != nil {
		t.Fatalf("member auth: %v", err)
	}
	_ = readEnvelope(t, memberConn) // connected
	_ = readEnvelope(t, memberConn) // presence snapshot

	outsiderConn := dial(t, server)
	if err := outsiderConn.WriteJSON(map[string]any{"type": "auth", "data": map[string]string{"token": outsiderToken}}); err != nil {
		t.Fatalf("outsider auth: %v", err)
	}
	_ = readEnvelope(t, outsiderConn) // connected; no Project snapshot

	sent := doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/chat/messages", map[string]string{"body": "realtime"}, ownerToken)
	if sent.Code != http.StatusCreated {
		t.Fatalf("send: %d %s", sent.Code, sent.Body.String())
	}

	env := readEnvelope(t, memberConn)
	if env.Type != chat.MessageCreatedEvent {
		t.Fatalf("expected %q, got %q", chat.MessageCreatedEvent, env.Type)
	}
	expectNoMoreMessages(t, outsiderConn)

	var message struct {
		ID string `json:"id"`
	}
	decodeJSON(t, sent, &message)
	deleted := doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+p.ID+"/chat/messages/"+message.ID, nil, ownerToken)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", deleted.Code, deleted.Body.String())
	}
	deleteEnv := readEnvelope(t, memberConn)
	if deleteEnv.Type != chat.MessageDeletedEvent {
		t.Fatalf("expected %q, got %q", chat.MessageDeletedEvent, deleteEnv.Type)
	}
}

func currentUserID(t *testing.T, router http.Handler, token string) string {
	t.Helper()
	rec := doJSON(t, router, http.MethodGet, "/api/v1/auth/session", nil, token)
	var user struct {
		ID string `json:"id"`
	}
	decodeJSON(t, rec, &user)
	return user.ID
}

// TestProjectChatAttachmentRoundTrip verifies the pre-existing Project Chat
// attachment endpoints end to end: multipart upload, member-only download
// with the original bytes and headers, the per-file limit, and the
// per-Project storage quota.
func TestProjectChatAttachmentRoundTrip(t *testing.T) {
	router, _, projects := newTestRouterWithHandlers()
	ownerToken := registerAndToken(t, router, "attach-owner")
	memberToken := registerAndToken(t, router, "attach-member")
	outsiderToken := registerAndToken(t, router, "attach-outsider")
	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Attachments"}, ownerToken)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)
	projects.addMember(p.ID, currentUserID(t, router, memberToken), project.RoleMember)
	base := "/api/v1/projects/" + p.ID + "/chat/attachments"

	content := []byte("attachment bytes")
	rec := doMultipart(t, router, http.MethodPost, base, memberToken, "notes.txt", "text/plain", content, map[string]string{"body": "see attached"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	var message struct {
		Body        string            `json:"body"`
		Attachments []chat.Attachment `json:"attachments"`
	}
	decodeJSON(t, rec, &message)
	if message.Body != "see attached" || len(message.Attachments) != 1 || message.Attachments[0].Filename != "notes.txt" || message.Attachments[0].SizeBytes != int64(len(content)) {
		t.Fatalf("uploaded message: %s", rec.Body.String())
	}
	download := base + "/" + message.Attachments[0].ID

	expectStatus(t, doJSON(t, router, http.MethodGet, download, nil, outsiderToken), http.StatusNotFound, "outsider download")
	expectStatus(t, doMultipart(t, router, http.MethodPost, base, outsiderToken, "x.txt", "", []byte("x"), nil), http.StatusNotFound, "outsider upload")
	rec = doJSON(t, router, http.MethodGet, download, nil, ownerToken)
	expectStatus(t, rec, http.StatusOK, "owner download")
	if rec.Body.String() != string(content) || rec.Header().Get("Content-Disposition") != "attachment; filename=notes.txt" || rec.Header().Get("Content-Type") != "text/plain" {
		t.Fatalf("download: %q headers=%v", rec.Body.String(), rec.Header())
	}

	// Per-file limit, then the per-Project quota (2 KiB with 1 KiB files).
	expectStatus(t, doMultipart(t, router, http.MethodPost, base, memberToken, "big.bin", "", bytes.Repeat([]byte("x"), testMaxUploadBytes+1), nil), http.StatusBadRequest, "over per-file limit")
	expectStatus(t, doMultipart(t, router, http.MethodPost, base, memberToken, "a.bin", "", bytes.Repeat([]byte("x"), testMaxUploadBytes), nil), http.StatusCreated, "first 1 KiB")
	rec = doMultipart(t, router, http.MethodPost, base, memberToken, "b.bin", "", bytes.Repeat([]byte("x"), testMaxUploadBytes), nil)
	expectStatus(t, rec, http.StatusRequestEntityTooLarge, "project quota")
	if !strings.Contains(rec.Body.String(), "storage_limit") {
		t.Fatalf("quota body: %s", rec.Body.String())
	}
}

func TestProjectChatListAttachments(t *testing.T) {
	router, _, projects := newTestRouterWithHandlers()
	ownerToken := registerAndToken(t, router, "files-owner")
	memberToken := registerAndToken(t, router, "files-member")
	outsiderToken := registerAndToken(t, router, "files-outsider")
	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "Files"}, ownerToken)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)
	projects.addMember(p.ID, currentUserID(t, router, memberToken), project.RoleMember)
	base := "/api/v1/projects/" + p.ID + "/chat/attachments"

	type listed struct {
		Attachments []struct {
			ID             string    `json:"id"`
			MessageID      string    `json:"message_id"`
			Filename       string    `json:"filename"`
			SizeBytes      int64     `json:"size_bytes"`
			AuthorID       string    `json:"author_id"`
			AuthorUsername string    `json:"author_username"`
			CreatedAt      time.Time `json:"created_at"`
			StorageID      *string   `json:"storage_id"`
		} `json:"attachments"`
	}

	// Empty list is an array, not null.
	rec := doJSON(t, router, http.MethodGet, base, nil, memberToken)
	expectStatus(t, rec, http.StatusOK, "empty list")
	if !strings.Contains(rec.Body.String(), `"attachments":[]`) {
		t.Fatalf("empty list body: %s", rec.Body.String())
	}

	expectStatus(t, doMultipart(t, router, http.MethodPost, base, ownerToken, "first.txt", "text/plain", []byte("one"), nil), http.StatusCreated, "upload first")
	rec = doMultipart(t, router, http.MethodPost, base, memberToken, "second.txt", "text/plain", []byte("second"), nil)
	expectStatus(t, rec, http.StatusCreated, "upload second")
	var second struct {
		ID string `json:"id"`
	}
	decodeJSON(t, rec, &second)
	// Plain text messages contribute no files.
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/projects/"+p.ID+"/chat/messages", map[string]string{"body": "no file"}, ownerToken), http.StatusCreated, "text message")

	rec = doJSON(t, router, http.MethodGet, base, nil, ownerToken)
	expectStatus(t, rec, http.StatusOK, "list")
	var got listed
	decodeJSON(t, rec, &got)
	if len(got.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %s", rec.Body.String())
	}
	if got.Attachments[0].Filename != "second.txt" || got.Attachments[0].AuthorUsername != "files-member" || got.Attachments[0].SizeBytes != 6 {
		t.Fatalf("newest first with author: %s", rec.Body.String())
	}
	if got.Attachments[1].Filename != "first.txt" || got.Attachments[1].AuthorUsername != "files-owner" || got.Attachments[1].CreatedAt.IsZero() {
		t.Fatalf("second entry: %s", rec.Body.String())
	}
	if got.Attachments[0].StorageID != nil || strings.Contains(rec.Body.String(), "storage_id") {
		t.Fatalf("storage id must not be exposed: %s", rec.Body.String())
	}

	// Listed IDs work with the existing download endpoint.
	expectStatus(t, doJSON(t, router, http.MethodGet, base+"/"+got.Attachments[1].ID, nil, memberToken), http.StatusOK, "download listed file")

	expectStatus(t, doJSON(t, router, http.MethodGet, base, nil, outsiderToken), http.StatusNotFound, "outsider list")

	// Deleting the carrying message removes the file from the list.
	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+p.ID+"/chat/messages/"+second.ID, nil, memberToken), http.StatusNoContent, "delete message")
	rec = doJSON(t, router, http.MethodGet, base, nil, memberToken)
	got = listed{}
	decodeJSON(t, rec, &got)
	if len(got.Attachments) != 1 || got.Attachments[0].Filename != "first.txt" {
		t.Fatalf("after delete: %s", rec.Body.String())
	}
}

func TestProjectChatSystemMessagesAreNotDeletable(t *testing.T) {
	router, handlers, _ := newTestRouterWithHandlers()
	ownerToken := registerAndToken(t, router, "sys-owner")
	created := doJSON(t, router, http.MethodPost, "/api/v1/projects", map[string]string{"name": "System"}, ownerToken)
	var p struct {
		ID string `json:"id"`
	}
	decodeJSON(t, created, &p)
	message, err := handlers.Chat.CreateSystemMessage(context.Background(), p.ID, chat.KindGit, "alice-gh pushed 1 commit → main")
	if err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, router, http.MethodGet, "/api/v1/projects/"+p.ID+"/chat/messages", nil, ownerToken)
	if !strings.Contains(rec.Body.String(), `"kind":"git"`) || !strings.Contains(rec.Body.String(), `"author_id":""`) {
		t.Fatalf("system message listing: %s", rec.Body.String())
	}
	expectStatus(t, doJSON(t, router, http.MethodDelete, "/api/v1/projects/"+p.ID+"/chat/messages/"+message.ID, nil, ownerToken), http.StatusForbidden, "owner deleting git activity")
}
