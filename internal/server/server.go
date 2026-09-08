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
	"sync"
	"time"

	"github.com/davison/md-notes/internal/config"
	"github.com/davison/md-notes/internal/render"
	"github.com/davison/md-notes/internal/roots"
	"github.com/davison/md-notes/internal/search"
	"github.com/davison/md-notes/internal/source"
	"github.com/davison/md-notes/internal/tags"
	"github.com/davison/md-notes/internal/tree"
	"github.com/davison/md-notes/internal/watch"
)

// Server serves the API and UI for a registry of roots.
type Server struct {
	reg    *roots.Registry
	port   int
	ui     fs.FS
	mux    *http.ServeMux
	log    *log.Logger
	md     *render.Renderer
	source *source.Store

	// keepalive is how often an idle event stream sends a comment.
	keepalive time.Duration
	// watchBudget caps the directories watched per root; see
	// config.DefaultMaxWatches. Zero or less places no cap.
	watchBudget int

	wmu      sync.Mutex
	hubs     map[string]*watch.Hub
	watchers map[string]*watch.Watcher
	// starting holds a channel per root whose watcher is being set up,
	// closed when setup finishes either way.
	starting map[string]chan struct{}
	// closing is closed by Close so event streams end promptly.
	closing chan struct{}
}

// Option adjusts a Server before it starts watching its roots.
type Option func(*Server)

// WithWatchBudget caps the directories watched per root for live update.
// Zero or less places no cap.
func WithWatchBudget(n int) Option {
	return func(s *Server) { s.watchBudget = n }
}

// New builds a Server. ui is the built single-page app; every path that is
// not an API route and not a file in ui serves its index.html so the app
// can route client-side.
func New(reg *roots.Registry, port int, ui fs.FS, logger *log.Logger, opts ...Option) *Server {
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	s := &Server{
		reg: reg, port: port, ui: ui, mux: http.NewServeMux(), log: logger, md: render.New(),
		source:      source.New(reg),
		keepalive:   30 * time.Second,
		hubs:        map[string]*watch.Hub{},
		watchers:    map[string]*watch.Watcher{},
		starting:    map[string]chan struct{}{},
		closing:     make(chan struct{}),
		watchBudget: config.DefaultMaxWatches,
	}
	for _, o := range opts {
		o(s)
	}
	// Watchers are set up in the background so a large root does not
	// delay binding the port; the events endpoint waits for its root.
	for _, root := range reg.List() {
		s.watchRoot(root)
	}
	s.mux.HandleFunc("GET /api/roots", s.listRoots)
	s.mux.HandleFunc("POST /api/roots", s.addRoot)
	s.mux.HandleFunc("GET /api/r/{slug}/raw/{path...}", s.rawFile)
	s.mux.HandleFunc("GET /api/r/{slug}/tree", s.treeHandler)
	s.mux.HandleFunc("GET /api/r/{slug}/note/{path...}", s.noteHandler)
	s.mux.HandleFunc("GET /api/r/{slug}/source/{path...}", s.sourceHandler)
	s.mux.HandleFunc("PUT /api/r/{slug}/source/{path...}", s.saveSourceHandler)
	s.mux.HandleFunc("GET /api/r/{slug}/events", s.eventsHandler)
	s.mux.HandleFunc("GET /api/r/{slug}/search", s.searchHandler)
	s.mux.HandleFunc("GET /api/r/{slug}/tags", s.tagsHandler)
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
		s.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// setupWait bounds how long Close waits for a watcher setup in progress.
var setupWait = 5 * time.Second

// Close ends every event stream and stops every root's watcher. Safe to
// call more than once.
func (s *Server) Close() {
	s.wmu.Lock()
	select {
	case <-s.closing:
	default:
		close(s.closing)
	}
	watchers := s.watchers
	s.watchers = map[string]*watch.Watcher{}
	var starting []chan struct{}
	for _, ch := range s.starting {
		starting = append(starting, ch)
	}
	s.wmu.Unlock()
	// A setup stuck on a slow filesystem must not hold shutdown for long.
	deadline := time.After(setupWait)
	for _, ch := range starting {
		select {
		case <-ch:
		case <-deadline:
		}
	}
	s.wmu.Lock()
	for slug, w := range s.watchers {
		watchers[slug] = w
	}
	s.watchers = map[string]*watch.Watcher{}
	s.wmu.Unlock()
	for _, w := range watchers {
		w.Close()
	}
}

// watchRoot starts watching root in the background, if it is not already
// watched or being set up, and pumps its batches into the root's hub. The
// hub exists only once the watcher does, so the events endpoint can say
// when live update is unavailable. The directory set comes from the same
// listing the navigator uses; without ripgrep it falls back to a walk.
func (s *Server) watchRoot(root roots.Root) {
	s.wmu.Lock()
	if _, ok := s.watchers[root.Slug]; ok {
		s.wmu.Unlock()
		return
	}
	if _, ok := s.starting[root.Slug]; ok {
		s.wmu.Unlock()
		return
	}
	select {
	case <-s.closing:
		s.wmu.Unlock()
		return
	default:
	}
	ready := make(chan struct{})
	s.starting[root.Slug] = ready
	s.wmu.Unlock()

	go func() {
		defer func() {
			s.wmu.Lock()
			delete(s.starting, root.Slug)
			s.wmu.Unlock()
			close(ready)
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		dirs, err := tree.Dirs(ctx, root.Path, s.log.Printf)
		if err != nil {
			s.log.Printf("watch %s: %v; watching every non-hidden directory", root.Slug, err)
			dirs = watch.Walk(root.Path)
		}
		w, err := watch.New(root.Path, dirs, s.log.Printf, watch.WithBudget(s.watchBudget))
		if err != nil {
			s.log.Printf("watch %s: %v; live update disabled for this root", root.Slug, err)
			return
		}
		s.wmu.Lock()
		select {
		case <-s.closing:
			s.wmu.Unlock()
			w.Close()
			return
		default:
		}
		hub := watch.NewHub()
		s.hubs[root.Slug] = hub
		s.watchers[root.Slug] = w
		s.wmu.Unlock()
		go hub.Pump(w)
	}()
}

// hub returns the root's hub, waiting for a setup in progress. The second
// result is false when live update is unavailable for the root.
func (s *Server) hub(ctx context.Context, slug string) (*watch.Hub, bool) {
	for {
		s.wmu.Lock()
		h, ok := s.hubs[slug]
		starting := s.starting[slug]
		s.wmu.Unlock()
		if ok {
			return h, true
		}
		if starting == nil {
			return nil, false
		}
		select {
		case <-starting:
		case <-ctx.Done():
			return nil, false
		}
	}
}

// coverage reports how much of a root's directory set its watcher covers.
// The second result is false when the root has no watcher.
func (s *Server) coverage(slug string) (watch.Coverage, bool) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	w, ok := s.watchers[slug]
	if !ok {
		return watch.Coverage{}, false
	}
	return w.Coverage(), true
}

// eventsHandler streams a root's change batches as Server-Sent Events. A
// status event carries the root's watch coverage on connect, and again
// whenever it changes, so a page can say that live update covers part of
// the root rather than leaving the limit in the daemon's log.
func (s *Server) eventsHandler(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if _, ok := s.reg.Get(slug); !ok {
		writeError(w, http.StatusNotFound, "unknown root")
		return
	}
	hub, ok := s.hub(r.Context(), slug)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "live update is not available for this root")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	ch, cancel := hub.Subscribe()
	defer cancel()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	sendStatus := func(cov watch.Coverage) {
		data, err := json.Marshal(cov)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "event: status\ndata: %s\n\n", data)
		flusher.Flush()
	}
	last, haveCoverage := s.coverage(slug)
	if haveCoverage {
		sendStatus(last)
	}

	ticker := time.NewTicker(s.keepalive)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.closing:
			return
		case b, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(b)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: change\ndata: %s\n\n", data)
			flusher.Flush()
		case <-ticker.C:
			// A directory appearing at runtime can spend the budget, so
			// the coverage is re-sent when it moves.
			if cov, ok := s.coverage(slug); ok && cov != last {
				last = cov
				sendStatus(cov)
			}
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
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
	s.watchRoot(root)
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
	files, err := tree.List(ctx, root.Path, s.log.Printf)
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

// searchHandler runs a literal, case-insensitive search over a root.
func (s *Server) searchHandler(w http.ResponseWriter, r *http.Request) {
	root, ok := s.reg.Get(r.PathValue("slug"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown root")
		return
	}
	q := r.URL.Query().Get("q")
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := search.Search(ctx, root.Path, q, s.log.Printf)
	switch {
	case errors.Is(err, search.ErrBadQuery):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, search.ErrNoRipgrep):
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	case err != nil:
		s.log.Printf("search %s %q: %v", root.Slug, q, err)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// tagsHandler lists a root's tags with counts and the notes carrying them.
func (s *Server) tagsHandler(w http.ResponseWriter, r *http.Request) {
	root, ok := s.reg.Get(r.PathValue("slug"))
	if !ok {
		writeError(w, http.StatusNotFound, "unknown root")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	files, err := tree.List(ctx, root.Path, s.log.Printf)
	if err != nil {
		s.log.Printf("tags %s: %v", root.Slug, err)
		if errors.Is(err, tree.ErrNoRipgrep) {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "could not list files")
		return
	}
	list := tags.Collect(ctx, root.Path, files)
	if list == nil {
		list = []tags.Tag{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": list})
}

// noteHandler renders one markdown file. Paths that are not markdown are
// 404 here; the raw endpoint serves them.
func (s *Server) noteHandler(w http.ResponseWriter, r *http.Request) {
	slug, rel := r.PathValue("slug"), r.PathValue("path")
	if !tree.IsMarkdown(rel) {
		writeError(w, http.StatusNotFound, "not a markdown file")
		return
	}
	real, err := s.reg.Resolve(slug, rel)
	switch {
	case errors.Is(err, roots.ErrOutside):
		writeError(w, http.StatusForbidden, "path is outside the root")
		return
	case err != nil:
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	src, err := os.ReadFile(real)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	note, err := s.md.Render(slug, path.Clean(rel), src)
	if err != nil {
		s.log.Printf("render %s/%s: %v", slug, rel, err)
		writeError(w, http.StatusInternalServerError, "could not render note")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, note)
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
