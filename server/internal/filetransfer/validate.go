package filetransfer

import (
	"mime"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxFileNameCharacters bounds a transfer's display file name. It matches
// the Project Chat attachment filename limit (internal/chat).
const MaxFileNameCharacters = 255

// validateFileName normalizes and checks a Client-supplied file name. The
// name is only ever displayed and used as a download hint; it is never a
// storage path (storage uses opaque random IDs), but path-like names are
// rejected anyway so nothing downstream can mistake one for a path.
func validateFileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	invalid := &ValidationError{Field: "file_name", Message: "File name is invalid"}
	if name == "" || name == "." || name == ".." {
		return "", invalid
	}
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxFileNameCharacters {
		return "", invalid
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", invalid
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", invalid
		}
	}
	return name, nil
}

// validateFileSize checks a declared or actual size against the configured
// per-file upload limit. Direct File Transfers are never counted against a
// Project's storage quota (docs/PRD.md "File and Storage Limits").
func validateFileSize(size, maxBytes int64) error {
	if size <= 0 {
		return &ValidationError{Field: "file_size", Message: "File must not be empty"}
	}
	if maxBytes > 0 && size > maxBytes {
		return ErrSizeLimitExceeded
	}
	return nil
}

// normalizeContentType falls back to the file name's extension, then to a
// generic binary type.
func normalizeContentType(contentType, fileName string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" || len(contentType) > 255 {
		contentType = mime.TypeByExtension(filepath.Ext(fileName))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return contentType
}
