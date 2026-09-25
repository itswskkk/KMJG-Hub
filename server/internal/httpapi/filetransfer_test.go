package httpapi_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
)

// Small limits so over-limit paths are cheap to exercise.
const (
	testMaxUploadBytes         = 1 << 10
	testMaxProjectStorageBytes = 2 << 10
)

type fileTransferBody struct {
	ID                string `json:"id"`
	SenderID          string `json:"sender_id"`
	SenderUsername    string `json:"sender_username"`
	RecipientID       string `json:"recipient_id"`
	RecipientUsername string `json:"recipient_username"`
	FileName          string `json:"file_name"`
	FileSize          int64  `json:"file_size"`
	Status            string `json:"status"`
}

// doMultipart sends one file as multipart/form-data field "file", plus any
// extra text fields.
func doMultipart(t *testing.T, router http.Handler, method, path, token, filename, contentType string, content []byte, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = writer.WriteField(k, v)
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(content)
	_ = writer.Close()
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func createTransfer(t *testing.T, router http.Handler, from, to friendUser, name string, size int64) (int, fileTransferBody) {
	t.Helper()
	rec := doJSON(t, router, http.MethodPost, "/api/v1/file-transfers", map[string]any{"recipient_id": to.id, "file_name": name, "file_size": size}, from.token)
	var out fileTransferBody
	if rec.Code == http.StatusCreated {
		decodeJSON(t, rec, &out)
	}
	return rec.Code, out
}

func listTransfers(t *testing.T, router http.Handler, user friendUser, which string) []fileTransferBody {
	t.Helper()
	rec := doJSON(t, router, http.MethodGet, "/api/v1/file-transfers/"+which, nil, user.token)
	expectStatus(t, rec, http.StatusOK, "list "+which)
	var out struct {
		Transfers      []fileTransferBody `json:"transfers"`
		MaxUploadBytes int64              `json:"max_upload_bytes"`
	}
	decodeJSON(t, rec, &out)
	if out.MaxUploadBytes != testMaxUploadBytes {
		t.Fatalf("max_upload_bytes = %d", out.MaxUploadBytes)
	}
	return out.Transfers
}

func TestFileTransferEndpointsRequireAuth(t *testing.T) {
	router := newTestRouter()
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/file-transfers"},
		{http.MethodGet, "/api/v1/file-transfers/incoming"},
		{http.MethodGet, "/api/v1/file-transfers/sent"},
		{http.MethodPost, "/api/v1/file-transfers/x/accept"},
		{http.MethodPost, "/api/v1/file-transfers/x/decline"},
		{http.MethodPost, "/api/v1/file-transfers/x/cancel"},
		{http.MethodPut, "/api/v1/file-transfers/x/upload"},
		{http.MethodGet, "/api/v1/file-transfers/x/download"},
	} {
		if rec := doJSON(t, router, tc.method, tc.path, nil, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401, got %d", tc.method, tc.path, rec.Code)
		}
	}
}

func TestFileTransferRequestRules(t *testing.T) {
	router, _, u := newDMRouter(t, "alice", "bob", "carol")

	if code, _ := createTransfer(t, router, u["alice"], u["bob"], "a.txt", 5); code != http.StatusForbidden {
		t.Fatalf("unrelated users: expected 403, got %d", code)
	}
	makeFriends(t, router, u["alice"], u["bob"])
	if code, _ := createTransfer(t, router, u["alice"], u["bob"], "../a.txt", 5); code != http.StatusBadRequest {
		t.Fatalf("path traversal name: expected 400, got %d", code)
	}
	if code, _ := createTransfer(t, router, u["alice"], u["bob"], "a.txt", 0); code != http.StatusBadRequest {
		t.Fatalf("empty file: expected 400, got %d", code)
	}
	if code, _ := createTransfer(t, router, u["alice"], u["bob"], "a.txt", testMaxUploadBytes+1); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("over limit: expected 413, got %d", code)
	}
	if code, _ := createTransfer(t, router, u["alice"], friendUser{id: "ghost"}, "a.txt", 5); code != http.StatusNotFound {
		t.Fatalf("unknown recipient: expected 404, got %d", code)
	}
	if code, _ := createTransfer(t, router, u["alice"], u["alice"], "a.txt", 5); code != http.StatusBadRequest {
		t.Fatalf("self: expected 400, got %d", code)
	}

	code, tr := createTransfer(t, router, u["alice"], u["bob"], "report.pdf", 5)
	if code != http.StatusCreated || tr.Status != "pending" || tr.SenderUsername != "alice" || tr.RecipientUsername != "bob" || tr.FileSize != 5 {
		t.Fatalf("create: %d %+v", code, tr)
	}
	if in := listTransfers(t, router, u["bob"], "incoming"); len(in) != 1 || in[0].ID != tr.ID {
		t.Fatalf("bob incoming: %+v", in)
	}
	if sent := listTransfers(t, router, u["alice"], "sent"); len(sent) != 1 || sent[0].ID != tr.ID {
		t.Fatalf("alice sent: %+v", sent)
	}
	if in := listTransfers(t, router, u["carol"], "incoming"); len(in) != 0 {
		t.Fatalf("carol incoming: %+v", in)
	}

	// The recipient is notified of the incoming request.
	rec := doJSON(t, router, http.MethodGet, "/api/v1/notifications", nil, u["bob"].token)
	if !strings.Contains(rec.Body.String(), `"file_transfer_request"`) || !strings.Contains(rec.Body.String(), "report.pdf") {
		t.Fatalf("expected file_transfer_request notification: %s", rec.Body.String())
	}

	// Blocking revokes the ability to send.
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/blocked", map[string]string{"user_id": u["alice"].id}, u["bob"].token), http.StatusCreated, "block")
	if code, _ := createTransfer(t, router, u["alice"], u["bob"], "a.txt", 5); code != http.StatusForbidden {
		t.Fatalf("blocked: expected 403, got %d", code)
	}
}

func TestFileTransferFullFlow(t *testing.T) {
	router, _, u := newDMRouter(t, "alice", "bob", "eve")
	makeFriends(t, router, u["alice"], u["bob"])
	content := []byte("hello, bob")
	_, tr := createTransfer(t, router, u["alice"], u["bob"], "greeting.txt", int64(len(content)))
	base := "/api/v1/file-transfers/" + tr.ID

	// No upload before the recipient accepts.
	expectStatus(t, doMultipart(t, router, http.MethodPut, base+"/upload", u["alice"].token, "greeting.txt", "text/plain", content, nil), http.StatusConflict, "upload before accept")

	expectStatus(t, doJSON(t, router, http.MethodPost, base+"/accept", nil, u["alice"].token), http.StatusForbidden, "sender accept")
	expectStatus(t, doJSON(t, router, http.MethodPost, base+"/accept", nil, u["eve"].token), http.StatusNotFound, "outsider accept")
	expectStatus(t, doJSON(t, router, http.MethodPost, base+"/cancel", nil, u["bob"].token), http.StatusForbidden, "recipient cancel")
	expectStatus(t, doJSON(t, router, http.MethodPost, base+"/accept", nil, u["bob"].token), http.StatusOK, "accept")
	expectStatus(t, doJSON(t, router, http.MethodPost, base+"/accept", nil, u["bob"].token), http.StatusConflict, "double accept")
	expectStatus(t, doJSON(t, router, http.MethodPost, base+"/decline", nil, u["bob"].token), http.StatusConflict, "decline after accept")

	expectStatus(t, doMultipart(t, router, http.MethodPut, base+"/upload", u["bob"].token, "greeting.txt", "text/plain", content, nil), http.StatusForbidden, "recipient upload")
	rec := doMultipart(t, router, http.MethodPut, base+"/upload", u["alice"].token, "greeting.txt", "text/plain", []byte("wrong size"+"!"), nil)
	expectStatus(t, rec, http.StatusBadRequest, "size mismatch")
	if !strings.Contains(rec.Body.String(), "size_mismatch") {
		t.Fatalf("size mismatch body: %s", rec.Body.String())
	}
	expectStatus(t, doJSON(t, router, http.MethodGet, base+"/download", nil, u["bob"].token), http.StatusConflict, "download before upload")

	rec = doMultipart(t, router, http.MethodPut, base+"/upload", u["alice"].token, "ignored-name.txt", "text/plain", content, nil)
	expectStatus(t, rec, http.StatusOK, "upload")
	var uploaded fileTransferBody
	decodeJSON(t, rec, &uploaded)
	if uploaded.Status != "uploaded" {
		t.Fatalf("uploaded: %+v", uploaded)
	}
	expectStatus(t, doMultipart(t, router, http.MethodPut, base+"/upload", u["alice"].token, "greeting.txt", "text/plain", content, nil), http.StatusConflict, "second upload")
	expectStatus(t, doJSON(t, router, http.MethodPost, base+"/cancel", nil, u["alice"].token), http.StatusConflict, "cancel after upload")

	expectStatus(t, doJSON(t, router, http.MethodGet, base+"/download", nil, u["alice"].token), http.StatusForbidden, "sender download")
	expectStatus(t, doJSON(t, router, http.MethodGet, base+"/download", nil, u["eve"].token), http.StatusNotFound, "outsider download")
	rec = doJSON(t, router, http.MethodGet, base+"/download", nil, u["bob"].token)
	expectStatus(t, rec, http.StatusOK, "download")
	if rec.Body.String() != string(content) {
		t.Fatalf("downloaded %q", rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename=greeting.txt` {
		t.Fatalf("content-disposition %q", cd)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("content-type %q", ct)
	}
}

func TestFileTransferDeclineAndCancel(t *testing.T) {
	router, _, u := newDMRouter(t, "alice", "bob")
	makeFriends(t, router, u["alice"], u["bob"])

	_, declined := createTransfer(t, router, u["alice"], u["bob"], "a.txt", 3)
	rec := doJSON(t, router, http.MethodPost, "/api/v1/file-transfers/"+declined.ID+"/decline", nil, u["bob"].token)
	expectStatus(t, rec, http.StatusOK, "decline")
	var out fileTransferBody
	decodeJSON(t, rec, &out)
	if out.Status != "declined" {
		t.Fatalf("declined: %+v", out)
	}
	expectStatus(t, doMultipart(t, router, http.MethodPut, "/api/v1/file-transfers/"+declined.ID+"/upload", u["alice"].token, "a.txt", "", []byte("abc"), nil), http.StatusConflict, "upload after decline")

	_, cancelled := createTransfer(t, router, u["alice"], u["bob"], "b.txt", 3)
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/file-transfers/"+cancelled.ID+"/accept", nil, u["bob"].token), http.StatusOK, "accept")
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/file-transfers/"+cancelled.ID+"/cancel", nil, u["alice"].token), http.StatusOK, "cancel accepted")
	expectStatus(t, doMultipart(t, router, http.MethodPut, "/api/v1/file-transfers/"+cancelled.ID+"/upload", u["alice"].token, "b.txt", "", []byte("abc"), nil), http.StatusConflict, "upload after cancel")
}

func TestFileTransferUploadBodyLimit(t *testing.T) {
	router, _, u := newDMRouter(t, "alice", "bob")
	makeFriends(t, router, u["alice"], u["bob"])
	_, tr := createTransfer(t, router, u["alice"], u["bob"], "a.bin", testMaxUploadBytes)
	expectStatus(t, doJSON(t, router, http.MethodPost, "/api/v1/file-transfers/"+tr.ID+"/accept", nil, u["bob"].token), http.StatusOK, "accept")
	huge := bytes.Repeat([]byte("x"), testMaxUploadBytes+(2<<20))
	expectStatus(t, doMultipart(t, router, http.MethodPut, "/api/v1/file-transfers/"+tr.ID+"/upload", u["alice"].token, "a.bin", "", huge, nil), http.StatusRequestEntityTooLarge, "oversized body")
}
