package server

import (
	"errors"
	"io"
	"net/http"

	"github.com/davison/md-notes/internal/source"
)

// uploadHandler puts one image into the root's _resources directory: the
// editor's paste and drop. The body is the file's bytes and nothing else;
// its Content-Type is not read, because what the file is gets decided by
// its content (source.Upload). The optional name query parameter is the
// dropped file's own name, which the store reduces to a safe one; a paste
// sends none. 201 for a new file, 200 for one already there with the same
// bytes, and the same body either way, so the client links to whichever it
// was given.
func (s *Server) uploadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	// One byte past the cap is enough to know the body is over it, and
	// nothing past that is read.
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, source.MaxUploadBytes+1))
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeSourceError(w, http.StatusRequestEntityTooLarge, "too_large", source.ErrUploadTooLarge.Error())
		return
	case err != nil:
		writeSourceError(w, http.StatusBadRequest, "invalid_body", "could not read the request body")
		return
	case len(data) == 0:
		writeSourceError(w, http.StatusBadRequest, "invalid_body", "the body must be the image's bytes")
		return
	}
	slug := r.PathValue("slug")
	up, err := s.source.Upload(slug, r.URL.Query().Get("name"), data)
	if err != nil {
		s.sourceError(w, err)
		return
	}
	status := http.StatusOK
	if up.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"root": slug, "path": up.Path, "name": up.Name, "created": up.Created})
}
