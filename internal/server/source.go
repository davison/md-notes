package server

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"unicode/utf8"

	"github.com/davison/md-notes/internal/roots"
	"github.com/davison/md-notes/internal/source"
	"github.com/davison/md-notes/internal/tree"
)

func (s *Server) sourceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !tree.IsMarkdown(r.PathValue("path")) {
		writeSourceError(w, http.StatusNotFound, "not_markdown", "not a markdown file")
		return
	}
	note, err := s.source.Read(r.PathValue("slug"), r.PathValue("path"))
	if err != nil {
		s.sourceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, note)
}

func (s *Server) saveSourceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !tree.IsMarkdown(r.PathValue("path")) {
		writeSourceError(w, http.StatusNotFound, "not_markdown", "not a markdown file")
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeSourceError(w, http.StatusUnsupportedMediaType, "invalid_body", "Content-Type must be application/json")
		return
	}
	// JSON may expand one source byte to six bytes (for example, \u0000).
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 6*source.MaxBytes+1024))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeSourceError(w, http.StatusRequestEntityTooLarge, "too_large", "request body is too large")
		return
	}
	var body struct {
		Source   *string `json:"source"`
		Revision *string `json:"revision"`
	}
	if err != nil || !utf8.Valid(data) || !validJSONUnicode(data) || json.Unmarshal(data, &body) != nil || body.Source == nil {
		writeSourceError(w, http.StatusBadRequest, "invalid_body", "body must contain a source string and revision string")
		return
	}
	if body.Revision == nil || *body.Revision == "" {
		writeSourceError(w, http.StatusPreconditionRequired, "revision_required", "revision is required")
		return
	}
	note, err := s.source.Save(r.PathValue("slug"), r.PathValue("path"), *body.Revision, *body.Source)
	if err != nil {
		s.sourceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, note)
}

// encoding/json replaces unpaired UTF-16 surrogate escapes with U+FFFD.
// Reject them so a malformed save cannot silently change the submitted source.
// JSON syntax validation remains the decoder's job.
func validJSONUnicode(data []byte) bool {
	for i := 0; i < len(data); i++ {
		if data[i] != '\\' {
			continue
		}
		if i+5 >= len(data) || data[i+1] != 'u' {
			i++
			continue
		}
		n, err := strconv.ParseUint(string(data[i+2:i+6]), 16, 16)
		if err != nil {
			return false
		}
		i += 5
		if n >= 0xdc00 && n <= 0xdfff {
			return false
		}
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

func (s *Server) sourceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, source.ErrConflict):
		writeSourceError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, os.ErrNotExist):
		writeSourceError(w, http.StatusNotFound, "not_found", "note or root no longer exists")
	case errors.Is(err, roots.ErrOutside):
		writeSourceError(w, http.StatusForbidden, "outside_root", "path is outside the root")
	case errors.Is(err, os.ErrPermission):
		writeSourceError(w, http.StatusForbidden, "permission_denied", "note or directory is not readable/writable")
	case errors.Is(err, source.ErrTooLarge):
		writeSourceError(w, http.StatusRequestEntityTooLarge, "too_large", err.Error())
	case errors.Is(err, source.ErrEncoding), errors.Is(err, source.ErrNotRegular):
		writeSourceError(w, http.StatusUnprocessableEntity, "unsupported_source", err.Error())
	default:
		s.log.Printf("source operation: %v", err)
		writeSourceError(w, http.StatusInternalServerError, "io_error", "could not read or save note")
	}
}

func writeSourceError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
