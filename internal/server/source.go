package server

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
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

// createSourceHandler creates a note that does not exist yet. The path in
// the URL is the note's path under the root, so one grammar names a note
// whether it is being read, replaced, created or deleted; the body is
// optional and a request without one creates an empty note.
func (s *Server) createSourceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	path := r.PathValue("path")
	if strings.TrimSpace(path) == "" {
		writeSourceError(w, http.StatusBadRequest, "invalid_path", "a note path is required")
		return
	}
	if !tree.IsMarkdown(path) {
		writeSourceError(w, http.StatusNotFound, "not_markdown", "not a markdown file")
		return
	}
	// JSON may expand one source byte to six bytes (for example, \u0000).
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 6*source.MaxBytes+1024))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeSourceError(w, http.StatusRequestEntityTooLarge, "too_large", "request body is too large")
		return
	}
	if err != nil {
		writeSourceError(w, http.StatusBadRequest, "invalid_body", "could not read the request body")
		return
	}
	text := ""
	if len(data) > 0 || r.Header.Get("Content-Type") != "" {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeSourceError(w, http.StatusUnsupportedMediaType, "invalid_body", "Content-Type must be application/json")
			return
		}
		var body struct {
			Source *string `json:"source"`
		}
		if !utf8.Valid(data) || !validJSONUnicode(data) || json.Unmarshal(data, &body) != nil {
			writeSourceError(w, http.StatusBadRequest, "invalid_body", "body must be an object with an optional source string")
			return
		}
		if body.Source != nil {
			text = *body.Source
		}
	}
	slug := r.PathValue("slug")
	created, err := s.source.Create(slug, path, text)
	if err != nil {
		s.sourceError(w, err)
		return
	}
	// created.Path is the cleaned name, which is not always the one that
	// was sent: it is what the client reads, saves and routes to now.
	w.Header().Set("Location", "/api/r/"+url.PathEscape(slug)+"/source/"+pathEscape(created.Path))
	writeJSON(w, http.StatusCreated, map[string]string{
		"root": slug, "path": created.Path, "source": created.Source, "revision": created.Revision,
	})
}

// deleteSourceHandler removes one markdown note. Everything it can refuse
// is refused before the store is reached; the store removes exactly the
// one name it is given.
func (s *Server) deleteSourceHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	path := r.PathValue("path")
	if strings.TrimSpace(path) == "" {
		writeSourceError(w, http.StatusBadRequest, "invalid_path", "a note path is required")
		return
	}
	if !tree.IsMarkdown(path) {
		writeSourceError(w, http.StatusNotFound, "not_markdown", "not a markdown file")
		return
	}
	if err := s.source.Delete(r.PathValue("slug"), path); err != nil {
		s.sourceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pathEscape escapes a slash-separated path for a URL, segment by
// segment, so that the Location header names the note and not a different
// one with an encoded slash in its name.
func pathEscape(p string) string {
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
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
	case errors.Is(err, source.ErrExists):
		writeSourceError(w, http.StatusConflict, "exists", err.Error())
	case errors.Is(err, source.ErrName):
		writeSourceError(w, http.StatusBadRequest, "invalid_path", "a note name may not be empty, hidden, or contain a control character")
	// The length a name may be is the filesystem's to say — NAME_MAX,
	// 255 bytes per component on every filesystem this daemon is run on —
	// so the kernel's refusal is translated rather than anticipated by a
	// check of our own. It is the caller's name that is wrong, not the
	// daemon that is broken: a 400, and no log line.
	case errors.Is(err, syscall.ENAMETOOLONG):
		writeSourceError(w, http.StatusBadRequest, "invalid_path", "a note name is too long: each part of the path may be at most 255 bytes")
	case errors.Is(err, os.ErrNotExist):
		writeSourceError(w, http.StatusNotFound, "not_found", "note or root no longer exists")
	case errors.Is(err, roots.ErrNotDir):
		writeSourceError(w, http.StatusNotFound, "not_found", "a component of the path is not a directory")
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
