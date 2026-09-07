// Package server is the daemon's HTTP surface: the JSON API, confined file
// serving, and the embedded single-page UI.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/davison/md-notes/internal/roots"
	"github.com/davison/md-notes/internal/tree"
)

// Server serves the API and UI for a registry of roots.
type Server struct {
	reg  *roots.Registry
	port int
	ui   fs.FS
	mux  *http.ServeMux
	log  *log.Logger
}

// New builds a Server. ui is the built single-page app; every path that is
// not an API route and not a file in ui serves its index.html so the app
// can route client-side.
func New(reg *roots.Registry, port int, ui fs.FS, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	s := &Server{reg: reg, port: port, ui: ui, mux: http.NewServeMux(), log: logger}
	s.mux.HandleFunc("GET /api/roots", s.listRoots)
	s.mux.HandleFunc("POST /api/roots", s.addRoot)
	s.mux.HandleFunc("GET /api/r/{slug}/raw/{path...}", s.rawFile)
	s.mux.HandleFunc("GET /api/r/{slug}/tree", s.treeHandler)
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint")
	})
	s.mux.HandleFunc("/", s.serveUI)
	return s
}

// Handler returns the full handler including the host and origin checks.
func (s *Server) Handler() http.Handler {
	return s.guard(s.mux)
}

// Addr is the loopback address the daemon binds.
func (s *Server) Addr() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(s.port))
}

// ListenAndServe binds the loopback address and serves until ctx is done.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.Addr(),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	s.log.Printf("mdn listening on http://%s", srv.Addr)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// guard rejects requests that did not come from the daemon's own origin.
// The Host check defeats DNS rebinding: a browser reaching the daemon via
// an attacker's hostname carries that hostname. The Origin check refuses
// cross-origin requests from other pages; requests with no Origin header,
// such as the CLI, pass because they are not browser cross-site requests.
func (s *Server) guard(next http.Handler) http.Handler {
	allowedHosts := map[string]bool{
		"localhost:" + strconv.Itoa(s.port): true,
		"127.0.0.1:" + strconv.Itoa(s.port): true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHosts[strings.ToLower(r.Host)] {
			writeError(w, http.StatusForbidden, "unexpected Host header")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			o := strings.ToLower(strings.TrimSuffix(origin, "/"))
			if !allowedHosts[strings.TrimPrefix(o, "http://")] || !strings.HasPrefix(o, "http://") {
				writeError(w, http.StatusForbidden, "cross-origin request refused")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) listRoots(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"roots": s.reg.List()})
}

func (s *Server) addRoot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !filepath.IsAbs(body.Path) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}
	root, err := s.reg.Add(body.Path)
	switch {
	case errors.Is(err, roots.ErrNotDir), errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		s.log.Printf("add root %q: %v", body.Path, err)
		writeError(w, http.StatusInternalServerError, "could not register root")
		return
	}
	writeJSON(w, http.StatusOK, root)
}

// treeHandler returns the markdown files of a root as a directory tree.
func (s *Server) treeHandler(w http.ResponseWriter, r *http.Request) {
	root, ok := s.reg.Get(r.PathValue("slug"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown root")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	files, err := tree.List(ctx, root.Path)
	if err != nil {
		s.log.Printf("tree %s: %v", root.Slug, err)
		if errors.Is(err, tree.ErrNoRipgrep) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "could not list files")
		return
	}
	writeJSON(w, http.StatusOK, tree.Build(files))
}

func (s *Server) rawFile(w http.ResponseWriter, r *http.Request) {
	real, err := s.reg.Resolve(r.PathValue("slug"), r.PathValue("path"))
	switch {
	case errors.Is(err, roots.ErrOutside):
		writeError(w, http.StatusForbidden, "path is outside the root")
		return
	case errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusNotFound, "not found")
		return
	case err != nil:
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	f, err := os.Open(real)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	// Raw files are user content served on the daemon's own origin. The
	// sandbox directive stops an HTML or SVG file from running script with
	// that origin's authority, which would otherwise let a crafted note
	// register roots and read files through the API.
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// serveUI serves a file from the UI bundle when one matches the request
// path, and index.html otherwise so client-side routes resolve.
func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if f, err := s.ui.Open(name); err == nil {
			defer f.Close()
			if info, err := f.Stat(); err == nil && !info.IsDir() {
				http.ServeFileFS(w, r, s.ui, name)
				return
			}
		}
	}
	index, err := fs.ReadFile(s.ui, "index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UI bundle missing: build it with make ui")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(index)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, "encode response:", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
