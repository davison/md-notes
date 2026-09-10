package server

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"time"
	"unicode/utf8"

	"github.com/davison/md-notes/internal/clip"
	"github.com/davison/md-notes/internal/roots"
	"github.com/davison/md-notes/internal/source"
)

// maxClipEnvelope is what the JSON around the markdown may add: the URL,
// the title, the field names, and the six bytes a single source byte can
// become once escaped.
const maxClipEnvelope = 64 << 10

// clipHandler creates a note from a web clipping. It requires the bearer
// token, since the whole point of the endpoint is that something which is
// not the loopback web UI is writing, and it reuses the source API's error
// envelope so a client has one set of codes to handle.
func (s *Server) clipHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !s.authenticated(r) {
		writeUnauthorized(w)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeSourceError(w, http.StatusUnsupportedMediaType, "invalid_body", "Content-Type must be application/json")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 6*source.MaxBytes+maxClipEnvelope))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeSourceError(w, http.StatusRequestEntityTooLarge, "too_large", "request body is too large")
		return
	}
	var body struct {
		URL      *string `json:"url"`
		Title    *string `json:"title"`
		Markdown *string `json:"markdown"`
		Kind     *string `json:"kind"`
	}
	if err != nil || !utf8.Valid(data) || !validJSONUnicode(data) || json.Unmarshal(data, &body) != nil ||
		body.URL == nil || body.Markdown == nil || body.Kind == nil {
		writeSourceError(w, http.StatusBadRequest, "invalid_body", "body must contain url, markdown and kind strings, and may contain a title")
		return
	}
	if body.Title == nil {
		empty := ""
		body.Title = &empty
	}
	if !absoluteURL(*body.URL) {
		writeSourceError(w, http.StatusBadRequest, "invalid_body", "url must be an absolute URL")
		return
	}
	if len(*body.Markdown) > source.MaxBytes {
		writeSourceError(w, http.StatusRequestEntityTooLarge, "too_large", source.ErrTooLarge.Error())
		return
	}
	if *body.Markdown == "" {
		writeSourceError(w, http.StatusBadRequest, "invalid_body", "markdown is empty; there is nothing to clip")
		return
	}

	root, ok := s.reg.Notes()
	if !ok {
		s.log.Print("clip: the registry has no notes root")
		writeSourceError(w, http.StatusInternalServerError, "io_error", "could not write the clip")
		return
	}
	rel, err := clip.Write(root, s.clipsDir, clip.Clip{
		URL:      *body.URL,
		Title:    *body.Title,
		Markdown: *body.Markdown,
		Kind:     *body.Kind,
	}, time.Now())
	if err != nil {
		s.clipError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"root": root.Slug, "path": rel})
}

// absoluteURL reports whether raw is a URL a note can record as its source.
// Any scheme is allowed — a clip of a local file is still a clip — but it
// must be absolute, so that the frontmatter names something reachable.
func absoluteURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() {
		return false
	}
	return u.Host != "" || u.Opaque != "" || u.Path != ""
}

func (s *Server) clipError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, clip.ErrKind):
		writeSourceError(w, http.StatusBadRequest, "invalid_body", err.Error())
	case errors.Is(err, roots.ErrOutside):
		writeSourceError(w, http.StatusForbidden, "outside_root", "the clips directory is outside the notes root")
	case errors.Is(err, os.ErrPermission):
		writeSourceError(w, http.StatusForbidden, "permission_denied", "the clips directory is not writable")
	case errors.Is(err, clip.ErrCrowded):
		writeSourceError(w, http.StatusConflict, "conflict", err.Error())
	default:
		s.log.Printf("clip: %v", err)
		writeSourceError(w, http.StatusInternalServerError, "io_error", "could not write the clip")
	}
}
