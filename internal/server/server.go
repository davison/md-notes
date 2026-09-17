// Package server is the daemon's HTTP surface: the JSON API, confined file
// serving, and the embedded single-page UI.
package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"github.com/davison/md-notes/internal/session"
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

	// token validates the bearer token a non-loopback client presents.
	// Nil accepts nothing, so a daemon built without one refuses every
	// request that carries an Authorization header.
	token Validator
	// clipsDir is where POST /api/clip writes, relative to the notes root.
	clipsDir string
	// tailnetHost is one extra Host name the guard accepts, for requests
	// a `tailscale serve` proxy forwards here. Empty is loopback only.
	tailnetHost string
	// sessions holds the browser logins issued under tailnetHost.
	sessions *session.Store
	// logins bounds how fast one caller can fail to log in.
	logins *throttle

	// keepalive is how often an idle event stream sends a comment.
	keepalive time.Duration
	// watchBudget caps the directories watched per root. Zero is no
	// budget; the daemon's default lives with the configuration.
	watchBudget int
	// listDirs is the directory listing a watcher setup starts from. It is
	// tree.Dirs; a test replaces it to hold one setup open while the
	// registry changes under it.
	listDirs func(ctx context.Context, root string, warnf func(string, ...any)) ([]tree.Dir, error)

	wmu      sync.Mutex
	hubs     map[string]*watch.Hub
	watchers map[string]*watch.Watcher
	// starting holds a channel per root whose watcher is being set up,
	// closed when setup finishes either way.
	starting map[string]chan struct{}
	// closing is closed by Close so event streams end promptly.
	closing chan struct{}

	// etags memoises the validator of each embedded UI file, keyed by the
	// name served — a compressed sibling is its own representation and so
	// its own entry. The bundle is in the binary, so one hash lasts.
	etagMu sync.Mutex
	etags  map[string]string
}

// Option adjusts a Server before it starts watching its roots.
type Option func(*Server)

// WithWatchBudget caps the directories watched per root for live update.
// Zero, the default, is no budget at all.
func WithWatchBudget(n int) Option {
	return func(s *Server) { s.watchBudget = n }
}

// Validator answers whether a presented bearer token is the daemon's own,
// and identifies which token that is. It is *token.Store in the daemon and
// a stub in tests.
type Validator interface {
	// Authenticate reports whether presented is the current token and, in
	// the same read, the generation it was compared against — so that a
	// credential minted from a successful check cannot be stamped with a
	// generation the check never saw.
	Authenticate(presented string) (generation uint64, ok bool)
	// Generation changes when, and only when, the token does, so a
	// session cookie minted from one can be refused once the token it
	// rested on has been rotated away.
	Generation() uint64
}

// WithToken gives the daemon the bearer token clients present in an
// Authorization header.
func WithToken(v Validator) Option {
	return func(s *Server) { s.token = v }
}

// WithClipsDir sets where the clip endpoint writes, relative to the notes
// root. The configuration has already confined it to the root.
func WithClipsDir(dir string) Option {
	return func(s *Server) { s.clipsDir = dir }
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
		source:    source.New(reg),
		clipsDir:  config.DefaultClipsDir,
		listDirs:  tree.Dirs,
		sessions:  session.New(session.DefaultTTL),
		logins:    newThrottle(),
		keepalive: 30 * time.Second,
		hubs:      map[string]*watch.Hub{},
		watchers:  map[string]*watch.Watcher{},
		starting:  map[string]chan struct{}{},
		closing:   make(chan struct{}),
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
	s.mux.HandleFunc("DELETE /api/roots/{slug}", s.removeRoot)
	s.mux.HandleFunc("POST /api/clip", s.clipHandler)
	s.mux.HandleFunc("GET /api/r/{slug}/raw/{path...}", s.rawFile)
	s.mux.HandleFunc("GET /api/r/{slug}/tree", s.treeHandler)
	s.mux.HandleFunc("GET /api/r/{slug}/note/{path...}", s.noteHandler)
	s.mux.HandleFunc("GET /api/r/{slug}/source/{path...}", s.sourceHandler)
	s.mux.HandleFunc("PUT /api/r/{slug}/source/{path...}", s.saveSourceHandler)
	s.mux.HandleFunc("POST /api/r/{slug}/source/{path...}", s.createSourceHandler)
	s.mux.HandleFunc("DELETE /api/r/{slug}/source/{path...}", s.deleteSourceHandler)
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
		dirs, err := s.listDirs(ctx, root.Path, s.log.Printf)
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
		// The root can be unregistered while its watcher is being set up,
		// and the slug it freed can come back for a different folder. So
		// the question here is not whether *something* holds the slug but
		// whether this setup's own root still does: a watcher installed
		// under a slug that now means somewhere else would stream the
		// wrong folder's changes to every page under it.
		//
		// The registry is read inside the same critical section the
		// watcher is installed in, so a removal either sees the watcher
		// and stops it or is seen here and the watcher is never installed.
		cur, held := s.reg.Get(root.Slug)
		if !held || cur.Path != root.Path {
			s.wmu.Unlock()
			w.Close()
			// The folder that took the slug found `starting` occupied by
			// this setup and started nothing of its own, so it is watched
			// here — once this setup's entry is gone, which the ready
			// channel says.
			if held {
				go func() {
					<-ready
					s.watchRoot(cur)
				}()
			}
			return
		}
		hub := watch.NewHub()
		s.hubs[root.Slug] = hub
		s.watchers[root.Slug] = w
		s.wmu.Unlock()
		go hub.Pump(w)
	}()
}

// unwatchRoot stops watching a root that is no longer served and ends the
// event streams open on it. Closing the hub is what those streams notice:
// each reader's channel closes, the handler returns, and the browser's
// reconnect meets the 404 an unknown slug gives — which is how a tab open
// on a removed root finds out it has to go home.
func (s *Server) unwatchRoot(slug string) {
	s.wmu.Lock()
	w := s.watchers[slug]
	h := s.hubs[slug]
	delete(s.watchers, slug)
	delete(s.hubs, slug)
	s.wmu.Unlock()
	if w != nil {
		w.Close()
	}
	if h != nil {
		h.Close()
	}
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
//
// A request carrying the daemon's bearer token is exempt from the Origin
// check, because the token is the claim the same-origin rule stands in for
// and no other page can read it: that is how the extension writes from its
// chrome-extension origin. A wrong token is refused outright rather than
// falling back to the origin rule, so a stale one is reported as such. The
// Host check applies either way.
//
// A configured tailnet host is one further name, and one different rule:
// see guardTailnet. Loopback keeps the rule above whether or not that name
// is configured — the extra name adds reach, and changes nothing about the
// machine the daemon runs on.
func (s *Server) guard(next http.Handler) http.Handler {
	loopbackHosts := map[string]bool{
		"localhost:" + strconv.Itoa(s.port): true,
		"127.0.0.1:" + strconv.Itoa(s.port): true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Which rule applies is decided by the Host, so nothing may be
		// able to decide it other than the Host header itself. A request
		// target in absolute form carries an authority of its own, which
		// Go copies into r.Host, so the target would choose the rule; no
		// browser and no reverse proxy sends one to an origin server.
		if r.URL.Host != "" {
			writeGuardError(w, http.StatusForbidden, "bad_host",
				"the request target must be in origin form")
			return
		}
		host := strings.ToLower(r.Host)
		switch {
		case loopbackHosts[host]:
			// A proxy that rewrote the Host would present exactly this:
			// loopback, with the forwarding headers it added on its way
			// past. Nothing on loopback sets those, and serving it under
			// the loopback rule would hand the whole API to whatever the
			// proxy admits, unauthenticated. Fail closed and say so.
			if s.tailnetHost != "" && forwarded(r) {
				writeGuardError(w, http.StatusForbidden, "bad_host",
					"a forwarded request carries a loopback Host; the proxy in front must pass the original Host through unchanged")
				return
			}
			if !s.guardLoopback(w, r, loopbackHosts) {
				return
			}
		case s.tailnetHost != "" && trimRootLabel(host) == s.tailnetHost:
			if !s.guardTailnet(w, r) {
				return
			}
		default:
			writeGuardError(w, http.StatusForbidden, "bad_host", "unexpected Host header")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// proxyMarkers are the headers a proxy adds to say it handled a request:
// the standard one from RFC 7239, the hop record every RFC 9110 proxy is
// supposed to append, and the three X-Forwarded-* conventions that
// predate both. Nothing on loopback sets any of them.
var proxyMarkers = [...]string{
	"Forwarded",
	"Via",
	"X-Forwarded-For",
	"X-Forwarded-Proto",
	"X-Forwarded-Host",
	"X-Real-IP",
}

// forwarded reports whether the request passed through a proxy that said
// so. The headers are meaningful only under the tailnet name; on loopback
// their presence is the evidence that something in front rewrote the Host
// the guard depends on.
//
// This is a backstop and cannot be anything else: a proxy that rewrites
// the Host and announces itself in none of these ways is indistinguishable
// from a local client, and nginx's bare `proxy_pass` is exactly that. What
// stops that configuration is the documentation, which says the original
// Host must be passed through; this catches the proxies that do say so —
// Caddy, Traefik, Apache's defaults, and the usual nginx boilerplate —
// before a misconfiguration becomes unauthenticated access.
func forwarded(r *http.Request) bool {
	for _, h := range proxyMarkers {
		if r.Header.Get(h) != "" {
			return true
		}
	}
	return false
}

// trimRootLabel drops one trailing dot from a Host's name, so that the
// fully qualified form of the tailnet name matches the configured one —
// which is stored without it, because that is what a browser usually
// sends. The loopback names are compared without this: they are the
// daemon's own behaviour on the machine, and M3-R6 leaves that alone.
func trimRootLabel(host string) string {
	name, port, err := net.SplitHostPort(host)
	if err != nil {
		return strings.TrimSuffix(host, ".")
	}
	return net.JoinHostPort(strings.TrimSuffix(name, "."), port)
}

// guardLoopback is the rule for a request to localhost or 127.0.0.1,
// unchanged since the token landed. It reports whether the request may go
// on to the mux.
func (s *Server) guardLoopback(w http.ResponseWriter, r *http.Request, allowedHosts map[string]bool) bool {
	presented, carried := bearer(r)
	switch {
	case carried && s.validToken(presented):
	case carried:
		writeUnauthorized(w)
		return false
	default:
		if origin := r.Header.Get("Origin"); origin != "" {
			o := strings.ToLower(strings.TrimSuffix(origin, "/"))
			if !allowedHosts[strings.TrimPrefix(o, "http://")] || !strings.HasPrefix(o, "http://") {
				// A client that meant to authenticate and has no token
				// yet lands here, so the code has to be distinguishable
				// from a token that was presented and refused.
				writeGuardError(w, http.StatusForbidden, "cross_origin",
					"cross-origin request refused; present the bearer token to write from another origin")
				return false
			}
		}
	}
	return true
}

// bearer returns the token presented in the Authorization header. The
// second result says whether the request carried the header at all; a
// header in another scheme carries no token and is refused, since the
// daemon offers no other way to authenticate.
func bearer(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	scheme, value, found := strings.Cut(h, " ")
	if !found || !strings.EqualFold(scheme, "bearer") {
		return "", true
	}
	return strings.TrimSpace(value), true
}

// validToken reports whether presented is the daemon's bearer token. A
// daemon with no token configured accepts none.
func (s *Server) validToken(presented string) bool {
	if s.token == nil {
		return false
	}
	_, ok := s.token.Authenticate(presented)
	return ok
}

// authenticated reports whether the request proved it holds the token.
// The guard lets an unauthenticated same-origin request through, so an
// endpoint that requires the token asks for itself.
func (s *Server) authenticated(r *http.Request) bool {
	presented, carried := bearer(r)
	return carried && s.validToken(presented)
}

// writeGuardError refuses a request before any handler runs, in the same
// {code, error} envelope the handlers use, so one client can tell the
// guard's refusals apart from each other and from a handler's: a popup
// with no token pasted in yet gets cross_origin, and one holding a stale
// token gets unauthorized.
func writeGuardError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Cache-Control", "no-store")
	writeSourceError(w, status, code, message)
}

func writeUnauthorized(w http.ResponseWriter) {
	// A generic HTTP client expects the challenge; a browser will not act
	// on it, since the daemon has no interactive login on this path.
	w.Header().Set("WWW-Authenticate", `Bearer realm="mdn"`)
	writeGuardError(w, http.StatusUnauthorized, "unauthorized", "a valid bearer token is required")
}

func (s *Server) listRoots(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"roots": s.reg.List()})
}

// addRoot registers a folder. The optional `file` names a note inside it —
// relative to the folder, or absolute and under it — and the folder is
// registered only if that note is there: otherwise `404 not_found`, with
// nothing added to the registry and nothing written to the state file.
//
// That field is what stops a `file:` URL for a note that does not exist
// from registering its directory for ever
// ([#50](https://github.com/davison/md-notes/issues/50)). The daemon does
// the checking rather than the extension registering and undoing, because
// a rollback has a window in which the root is registered and persisted,
// and a client that dies in that window leaves the very root the check
// exists to prevent (M7-R1).
//
// A request with no `file` is what `mdn open` sends and is unchanged.
func (s *Server) addRoot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		File string `json:"file"`
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
	// What counts as a note is decided here, beside every other handler
	// that asks it of a request path; the registry answers for whether the
	// file is there and inside the folder.
	if body.File != "" && !tree.IsMarkdown(body.File) {
		noSuchNote(w, body.File)
		return
	}
	root, err := s.reg.AddFor(body.Path, body.File)
	switch {
	case errors.Is(err, roots.ErrNoNote):
		noSuchNote(w, body.File)
		return
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

// noSuchNote is the one refusal every way of failing the check earns, in
// the `{code, error}` envelope the source and clip endpoints answer in so
// a client branches on the code rather than on the sentence. It names the
// file the caller sent and nothing else: the caller is on loopback and
// holds that path already, and which of "missing", "not markdown" and
// "not under the folder" it was says more about what is outside the folder
// than about the folder.
func noSuchNote(w http.ResponseWriter, file string) {
	writeSourceError(w, http.StatusNotFound, "not_found",
		"no such note under that folder: "+file)
}

// removeRoot unregisters a recent root: out of the registry, out of the
// state file, its watcher stopped and its open event streams ended. It
// removes nothing from disk (M7-R2).
//
// The configured notes root is refused `notes_root`: it is the daemon's
// configuration rather than a registration, and a daemon serving nothing
// until its next restart is not a state the home page should be able to
// ask for. An unknown slug is `not_found`, which is also what removing the
// same root twice gets. Under the tailnet name the request never reaches
// here at all — the allow-list admits `GET /api/roots` and nothing else
// under that path, and this endpoint is refused by that default.
func (s *Server) removeRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	slug := r.PathValue("slug")
	root, err := s.reg.Remove(slug)
	switch {
	case errors.Is(err, roots.ErrNotesRoot):
		writeSourceError(w, http.StatusForbidden, "notes_root",
			"the notes root is the daemon's configuration, not a registration; it cannot be removed")
		return
	case errors.Is(err, os.ErrNotExist):
		writeSourceError(w, http.StatusNotFound, "not_found", "no such root: "+slug)
		return
	case err != nil:
		s.log.Printf("remove root %q: %v", slug, err)
		writeSourceError(w, http.StatusInternalServerError, "io_error", "could not remove the root")
		return
	}
	s.unwatchRoot(root.Slug)
	s.log.Printf("unregistered root %s (%s); nothing was removed from disk", root.Slug, root.Path)
	w.WriteHeader(http.StatusNoContent)
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

// uiAssetsDir is where the UI build writes its hashed output. A file under
// it is immutable by construction: its content hash is part of its name, so
// a changed file is a changed URL and the old one is never asked for again.
const uiAssetsDir = "assets/"

// uiImmutable is a year — the longest age a cache is asked to treat as
// sensible — plus the token that stops even a reload revalidating.
const uiImmutable = "public, max-age=31536000, immutable"

// uiEncodings are the content codings the daemon can answer from a file the
// build precompressed, most preferred first. Nothing is compressed here at
// request time: the bundle ships its own `.br` and `.gz` copies.
var uiEncodings = []struct{ coding, suffix string }{
	{"br", ".br"},
	{"gzip", ".gz"},
}

// uiTypes is the Content-Type for each kind of file the UI build emits.
// mime.TypeByExtension would answer from the build host's /etc/mime.types,
// which makes the header a property of whichever machine compiled the
// binary rather than of the binary — and it has no entry for .woff2 or .map
// at all, so the first font or source map the bundle gains would be typed
// one way here and another way there. The bundle is ours; so is this table.
//
// It is the same list the build's `compressible` pattern draws its text
// kinds from (ui/vite.config.ts); TestUITypesCoverTheBundle holds them
// together by failing on any extension in dist that is missing here.
var uiTypes = map[string]string{
	".css":   "text/css; charset=utf-8",
	".html":  "text/html; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".json":  "application/json",
	".map":   "application/json",
	".png":   "image/png",
	".svg":   "image/svg+xml",
	".woff2": "font/woff2",
}

// serveUI serves a file from the UI bundle when one matches the request
// path, and index.html otherwise so client-side routes resolve.
func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name != "" && name != "index.html" && !uiSibling(name) && s.uiFile(name) {
		s.serveUIFile(w, r, name)
		return
	}
	// A name under the hashed assets directory is build output or it is
	// nothing: no client-side route lives there. Answering one the bundle
	// does not have with the app shell is how a stale chunk URL — an
	// upgraded daemon under an open tab, the one case where a name cannot
	// be right — turns into a MIME-type error in the console instead of the
	// status that says what happened.
	if uiAsset(name) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if !s.uiFile("index.html") {
		writeError(w, http.StatusInternalServerError, "UI bundle missing: build it with make ui")
		return
	}
	s.serveUIFile(w, r, "index.html")
}

// uiAsset reports whether name addresses the build's output directory or
// anything inside it. Nothing there is a client-side route, and everything
// there is hashed.
func uiAsset(name string) bool {
	return name == strings.TrimSuffix(uiAssetsDir, "/") || strings.HasPrefix(name, uiAssetsDir)
}

// uiSibling reports whether name is one of the precompressed copies the
// build writes. They are representations of another URL, not resources of
// their own: served directly they would be undecodable bytes, so they are
// not addressable and fall through to the app like any other unknown path.
func uiSibling(name string) bool {
	for _, e := range uiEncodings {
		if strings.HasSuffix(name, e.suffix) {
			return true
		}
	}
	return false
}

// uiFile reports whether name is a regular file in the embedded bundle.
func (s *Server) uiFile(name string) bool {
	f, err := s.ui.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	return err == nil && !info.IsDir()
}

// serveUIFile serves one embedded file with the caching and compression a
// built asset deserves. The hashed assets get an immutable year and
// everything else, index.html included, gets no-cache, which means
// revalidate rather than do not store. embed.FS carries a zero modification
// time, so http.ServeFileFS can offer no validator at all and a conditional
// request is answered with the whole file; an ETag over the bytes actually
// sent restores the 304.
func (s *Server) serveUIFile(w http.ResponseWriter, r *http.Request, name string) {
	ctype := uiTypes[strings.ToLower(path.Ext(name))]
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	cache := "no-cache"
	if uiAsset(name) {
		cache = uiImmutable
	}
	// Which bytes come back depends on Accept-Encoding, so every response
	// says so — the uncompressed one too, or a cache holding the brotli copy
	// would hand it to a client that cannot read it.
	w.Header().Set("Vary", "Accept-Encoding")
	w.Header().Set("Cache-Control", cache)
	// Set before ServeContent, which would otherwise guess from the name it
	// is given; the compressed siblings end in .br and .gz and would sniff
	// as binary.
	w.Header().Set("Content-Type", ctype)

	served, encoded := name, false
	if coding, file, ok := s.uiEncoded(name, r.Header.Get("Accept-Encoding")); ok {
		w.Header().Set("Content-Encoding", coding)
		served, encoded = file, true
	}
	// The ETag names the representation rather than the file: the brotli,
	// gzip and identity forms of one asset are three different bodies, and
	// sharing a tag across them is how a cache serves the wrong one.
	if tag, err := s.uiETag(served); err == nil {
		w.Header().Set("ETag", tag)
	}
	f, err := s.ui.Open(served)
	if err != nil {
		uiError(w, http.StatusNotFound, "not found")
		return
	}
	defer f.Close()
	// ServeContent leaves out the Content-Length once Content-Encoding is
	// set, guarding against a handler that meant the writer to do the
	// compressing. This body is already compressed and its length is known,
	// so say it rather than fall back to chunked framing; a range request
	// still overwrites this with the length of the range.
	if info, err := f.Stat(); err == nil && encoded {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	}
	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, name, time.Time{}, rs)
		return
	}
	// An fs.FS whose files do not seek: read the bytes and serve those.
	b, err := fs.ReadFile(s.ui, served)
	if err != nil {
		uiError(w, http.StatusNotFound, "not found")
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}

// uiError answers a UI request that failed after the headers describing the
// representation were already set. They describe a body that is not coming:
// a JSON error labelled `Content-Encoding: br`, carrying another file's
// validator and a year-long immutable cache directive, is one a cache would
// be right to keep and wrong to serve. net/http's own serveError scrubs the
// same set before writing.
func uiError(w http.ResponseWriter, status int, msg string) {
	h := w.Header()
	for _, k := range []string{"Cache-Control", "Content-Encoding", "Content-Length", "ETag", "Vary"} {
		h.Del(k)
	}
	writeError(w, status, msg)
}

// uiEncoded picks the precompressed sibling to serve for name, given what
// the client said it accepts. Missing siblings are not an error: the build
// skips a file it could not shrink, and the identity bytes always answer.
func (s *Server) uiEncoded(name, accept string) (coding, file string, ok bool) {
	if accept == "" {
		return "", "", false
	}
	best := 0.0
	for _, e := range uiEncodings {
		q := encodingQuality(accept, e.coding)
		if q <= best || !s.uiFile(name+e.suffix) {
			continue
		}
		coding, file, ok, best = e.coding, name+e.suffix, true, q
	}
	return coding, file, ok
}

// encodingQuality is the weight an Accept-Encoding header gives one coding.
// A named coding beats the wildcard, an unmentioned coding is unacceptable,
// and q=0 is the only way a client can refuse one by name.
func encodingQuality(header, coding string) float64 {
	named, wildcard := -1.0, -1.0
	for _, part := range strings.Split(header, ",") {
		spec, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		q := 1.0
		for params != "" {
			var param string
			param, params, _ = strings.Cut(params, ";")
			k, v, ok := strings.Cut(param, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(k), "q") {
				continue
			}
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
				q = f
			}
		}
		switch strings.ToLower(strings.TrimSpace(spec)) {
		case coding:
			named = q
		case "*":
			wildcard = q
		}
	}
	switch {
	case named >= 0:
		return named
	case wildcard >= 0:
		return wildcard
	}
	return 0
}

// uiETag is the validator for one embedded file, hashed once and kept: the
// bundle is baked into the binary, so its bytes cannot change under us.
func (s *Server) uiETag(name string) (string, error) {
	s.etagMu.Lock()
	tag, ok := s.etags[name]
	s.etagMu.Unlock()
	if ok {
		return tag, nil
	}
	b, err := fs.ReadFile(s.ui, name)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	tag = `"` + base64.RawURLEncoding.EncodeToString(sum[:16]) + `"`
	s.etagMu.Lock()
	if s.etags == nil {
		s.etags = map[string]string{}
	}
	s.etags[name] = tag
	s.etagMu.Unlock()
	return tag, nil
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
