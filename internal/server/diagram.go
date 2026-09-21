package server

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/davison/md-notes/internal/diagram"
	"github.com/davison/md-notes/internal/render"
	"github.com/davison/md-notes/internal/roots"
	"github.com/davison/md-notes/internal/tree"
)

// The diagram route (davison/md-notes#171): the SVG for one flowchart
// block in one note, drawn by internal/diagram, for the reading view to
// show through <img>.
//
//	GET /api/r/{slug}/diagram/{path...}?h=<hash>&theme=light|dark|eink
//
// It never draws anything but a block in a note on disk. Every request
// resolves the note as the note endpoint does, reads it again, lists its
// flowcharts with the same code the note render used (render.Diagrams),
// and looks for one whose hash is h. The hash is a lookup key among the
// blocks of a note the caller can already read, never a claim about what
// to draw, and a hash the note no longer holds is a 404: the note changed
// under the page. The route holds no state a restart loses, so a page left
// open across one gets the same answer.

// diagramCSP makes the SVG inert when it is opened as a document rather
// than as an image — "open image in new tab" — which is the one way it can
// be anything but a picture. `sandbox` gives it an opaque origin with
// scripts off, as the raw-file route does, and `default-src 'none'` stops
// it loading anything at all. The writer uses presentation attributes
// only, with no <style> and no style attribute, so the SVG needs no
// style-src and gets none.
const diagramCSP = "default-src 'none'; sandbox"

// diagramThemes are the palettes the route draws in, by the name the page
// asks for.
var diagramThemes = map[string]diagram.Theme{
	"light": diagram.Light,
	"dark":  diagram.Dark,
	"eink":  diagram.EInk,
}

// diagramHash is the form render.HashSource writes: 128 bits in hex.
var diagramHash = regexp.MustCompile(`^[0-9a-f]{32}$`)

// The cache's bounds. An SVG is a few kilobytes for the diagrams notes
// hold, and a few hundred for one at the node bound, so 8 MiB is hundreds
// of diagrams in three themes; the entry cap bounds the bookkeeping for a
// cache full of small refusals.
const (
	diagramCacheBytes   = 8 << 20
	diagramCacheEntries = 512
)

// The note-list cache's bounds. An entry holds its blocks' sources, which
// the input bound keeps to 32 KiB each; 16 MiB holds the lists of a few
// hundred ordinary notes, or a handful of pathological ones.
const (
	listCacheBytes   = 16 << 20
	listCacheEntries = 256
)

// drawFunc is diagram.Render; a test replaces it to count or hold draws.
type drawFunc func(ctx context.Context, src []byte, theme diagram.Theme) ([]byte, error)

// drawn is one cached result: an SVG and its validator, or the refusal
// that stands in for one.
type drawn struct {
	svg     []byte
	etag    string
	refusal *diagram.Refusal
}

func (s *Server) diagramHandler(w http.ResponseWriter, r *http.Request) {
	// On every answer, the refusals too: whatever a browser is handed from
	// this path, opened directly, runs nothing and is not sniffed into
	// something that could.
	h := w.Header()
	setDiagramHeaders(h)
	fail := func(status int, msg string) {
		h.Set("Cache-Control", "no-store")
		writeError(w, status, msg)
	}

	slug, rel := r.PathValue("slug"), r.PathValue("path")
	q := r.URL.Query()
	themeName := q.Get("theme")
	theme, ok := diagramThemes[themeName]
	if !ok {
		fail(http.StatusBadRequest, "theme must be light, dark or eink")
		return
	}
	hash := q.Get("h")
	if !diagramHash.MatchString(hash) {
		fail(http.StatusBadRequest, "h must be the diagram's hash")
		return
	}
	if !tree.IsMarkdown(rel) {
		fail(http.StatusNotFound, "not a markdown file")
		return
	}
	real, err := s.reg.Resolve(slug, rel)
	switch {
	case errors.Is(err, roots.ErrOutside):
		fail(http.StatusForbidden, "path is outside the root")
		return
	case err != nil:
		fail(http.StatusNotFound, "not found")
		return
	}
	source, err := s.diagramSource(r.Context(), real, hash)
	switch {
	case errors.Is(err, os.ErrNotExist):
		fail(http.StatusNotFound, "not found")
		return
	case errors.Is(err, errListing):
		fail(http.StatusInternalServerError, "the note's diagrams could not be listed")
		return
	case err != nil:
		fail(http.StatusServiceUnavailable, "the diagram was not drawn")
		return
	}
	if source == nil {
		fail(http.StatusNotFound, "the note has no such diagram; it may have changed")
		return
	}

	got, err := s.drawDiagram(r.Context(), hash+"/"+themeName, source, theme)
	if err != nil {
		// The caller went away, or the daemon is shutting down.
		fail(http.StatusServiceUnavailable, "the diagram was not drawn")
		return
	}
	if got.refusal != nil {
		fail(http.StatusUnprocessableEntity, got.refusal.Error())
		return
	}
	h.Set("Content-Type", "image/svg+xml")
	// Only this origin's pages may embed it. The guard already refuses a
	// cross-origin request that says where it is from; an <img> from
	// another site says nothing, and this is the header that answers it.
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	// Revalidate, not immutable: the URL names the source, and a daemon
	// with a better layout draws the same source differently. The ETag is
	// over the bytes, so an unchanged drawing is a 304.
	h.Set("Cache-Control", "no-cache")
	h.Set("ETag", got.etag)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(got.svg))
}

// diagramSource finds the block whose hash is hash in the note at real.
// It returns nil when the note holds no such block, and an error wrapping
// os.ErrNotExist when the note cannot be read.
//
// A page asks for each of its diagrams separately, so listing the note
// afresh for every request would parse the whole note once per diagram —
// quadratic in its size, and outside any bound (the round-one review of
// PR #175, B1). So the list is cached by the file's identity as a stat sees
// it, and a request for a diagram of an unchanged note costs a stat and a
// map look-up. A listing that has to be built — the read and the parse —
// takes a draw slot like a draw does, so a burst of them queues.
//
// A stale entry could only come from a write that kept both the size and
// the modification time to the nanosecond; the worst it can do is answer
// with a block the note held a moment ago, or a 404 the page already
// treats as a fallback.
func (s *Server) diagramSource(ctx context.Context, real, hash string) ([]byte, error) {
	info, err := os.Stat(real)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("stat note: %w", os.ErrNotExist)
	}
	key := noteKey{path: real, size: info.Size(), mtime: info.ModTime().UnixNano()}
	list, ok := s.lists.get(key)
	if !ok {
		if list, err = s.listInSlot(ctx, key, real); err != nil {
			return nil, err
		}
	}
	for _, d := range list {
		if d.Hash == hash {
			return d.Source, nil
		}
	}
	return nil, nil
}

// errListing is a listing that panicked: a bug, answered as a 500 for that
// request rather than taking the slot, or the daemon, with it.
var errListing = errors.New("listing the note failed")

// listInSlot builds a note's list inside a draw slot and caches it. The
// slot is released however the build ends, and a panic in it becomes
// errListing: the same second line diagram.Render keeps, for the same
// reason — the listing runs diagram.Parse over hostile input too, and one
// bad note must not stop diagrams for every other (round-two review of
// PR #175, B2).
//
// A read that blocks — a note on a hung network or FUSE mount — holds its
// slot for as long as the read does; os.ReadFile cannot be interrupted.
// That is accepted, and the note render has the same exposure.
func (s *Server) listInSlot(ctx context.Context, key noteKey, real string) (list []render.Diagram, err error) {
	select {
	case s.drawSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.drawSlots }()
	defer func() {
		if r := recover(); r != nil {
			s.log.Printf("diagram: listing %s panicked: %v", real, r)
			list, err = nil, errListing
		}
	}()
	// Another request may have listed it while this one waited.
	if list, ok := s.lists.get(key); ok {
		return list, nil
	}
	list, err = s.buildList(real)
	if err != nil {
		return nil, err
	}
	// Before the slot is handed on, so the next waiter finds it.
	s.lists.put(key, list)
	return list, nil
}

// listNote reads a note and lists its flowcharts. It is the listing the
// note render does; a test replaces it to count or hold listings.
func (s *Server) listNote(real string) ([]render.Diagram, error) {
	src, err := os.ReadFile(real)
	if err != nil {
		return nil, fmt.Errorf("read note: %w", os.ErrNotExist)
	}
	return s.md.Diagrams(src), nil
}

// drawDiagram answers from the cache, or draws — at most drawSlots at once
// — and caches the answer. A refusal is cached as well as an SVG, so a
// hostile block costs its render deadline once rather than at every view.
// An error means the context ended; nothing is cached for it.
func (s *Server) drawDiagram(ctx context.Context, key string, src []byte, theme diagram.Theme) (drawn, error) {
	if d, ok := s.svgs.get(key); ok {
		return d, nil
	}
	select {
	case s.drawSlots <- struct{}{}:
	case <-ctx.Done():
		return drawn{}, ctx.Err()
	}
	defer func() { <-s.drawSlots }()
	// Another request may have drawn it while this one waited.
	if d, ok := s.svgs.get(key); ok {
		return d, nil
	}
	svg, err := s.draw(ctx, src, theme)
	var refusal *diagram.Refusal
	switch {
	case err == nil:
		sum := sha256.Sum256(svg)
		d := drawn{svg: svg, etag: `"` + base64.RawURLEncoding.EncodeToString(sum[:16]) + `"`}
		s.svgs.put(key, d)
		return d, nil
	case errors.As(err, &refusal):
		d := drawn{refusal: refusal}
		s.svgs.put(key, d)
		return d, nil
	default:
		return drawn{}, err
	}
}

// noteKey is a note's identity as a stat sees it.
type noteKey struct {
	path  string
	size  int64
	mtime int64
}

// listCache holds the flowchart list of recently viewed notes, bounded by
// the bytes of source it holds and by entries, least recently used first
// out. A note's old entries age out on their own: an edit changes its key.
type listCache struct {
	mu         sync.Mutex
	maxBytes   int
	maxEntries int
	size       int
	order      *list.List
	items      map[noteKey]*list.Element
}

type listItem struct {
	key  noteKey
	list []render.Diagram
}

func newListCache(maxBytes, maxEntries int) *listCache {
	return &listCache{maxBytes: maxBytes, maxEntries: maxEntries, order: list.New(), items: map[noteKey]*list.Element{}}
}

func listSize(key noteKey, l []render.Diagram) int {
	n := len(key.path) + 16
	for _, d := range l {
		n += len(d.Source) + len(d.Hash) + 8
	}
	return n
}

func (c *listCache) get(key noteKey) ([]render.Diagram, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(e)
	return e.Value.(*listItem).list, true
}

func (c *listCache) put(key noteKey, l []render.Diagram) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := listSize(key, l)
	if n > c.maxBytes {
		return
	}
	if e, ok := c.items[key]; ok {
		c.size -= listSize(key, e.Value.(*listItem).list)
		e.Value.(*listItem).list = l
		c.size += n
		c.order.MoveToFront(e)
	} else {
		c.items[key] = c.order.PushFront(&listItem{key: key, list: l})
		c.size += n
	}
	for c.size > c.maxBytes || c.order.Len() > c.maxEntries {
		last := c.order.Back()
		it := last.Value.(*listItem)
		c.order.Remove(last)
		delete(c.items, it.key)
		c.size -= listSize(it.key, it.list)
	}
}

// setDiagramHeaders puts on an answer at a diagram URL the two headers
// that keep whatever it is from being sniffed into, or run as, anything.
func setDiagramHeaders(h http.Header) {
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", diagramCSP)
}

// diagramPath reports whether p, as the request wrote it, is under a
// root's diagram route: /api/r/{slug}/diagram/…. The guard asks, so the
// answers written before the handler runs carry the headers too.
func diagramPath(p string) bool {
	rest, ok := strings.CutPrefix(p, "/api/r/")
	if !ok {
		return false
	}
	_, sub, ok := strings.Cut(rest, "/")
	return ok && (sub == "diagram" || strings.HasPrefix(sub, "diagram/"))
}

// drawSlotCount is how many diagrams may be drawn at once: half the
// processors, so a note full of expensive blocks queues rather than taking
// every core from the rest of the daemon.
func drawSlotCount() int {
	return max(1, runtime.GOMAXPROCS(0)/2)
}

// svgCache is a least-recently-used cache bounded by bytes and by entries.
type svgCache struct {
	mu         sync.Mutex
	maxBytes   int
	maxEntries int
	size       int
	order      *list.List // front is most recently used
	items      map[string]*list.Element
}

type svgItem struct {
	key string
	val drawn
}

func newSVGCache(maxBytes, maxEntries int) *svgCache {
	return &svgCache{maxBytes: maxBytes, maxEntries: maxEntries, order: list.New(), items: map[string]*list.Element{}}
}

func itemSize(key string, d drawn) int {
	n := len(key) + len(d.svg) + len(d.etag)
	if d.refusal != nil {
		n += len(d.refusal.Reason)
	}
	return n
}

func (c *svgCache) get(key string) (drawn, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		return drawn{}, false
	}
	c.order.MoveToFront(e)
	return e.Value.(*svgItem).val, true
}

func (c *svgCache) put(key string, d drawn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := itemSize(key, d)
	if n > c.maxBytes {
		return
	}
	if e, ok := c.items[key]; ok {
		c.size -= itemSize(key, e.Value.(*svgItem).val)
		e.Value.(*svgItem).val = d
		c.size += n
		c.order.MoveToFront(e)
	} else {
		c.items[key] = c.order.PushFront(&svgItem{key: key, val: d})
		c.size += n
	}
	for c.size > c.maxBytes || c.order.Len() > c.maxEntries {
		last := c.order.Back()
		it := last.Value.(*svgItem)
		c.order.Remove(last)
		delete(c.items, it.key)
		c.size -= itemSize(it.key, it.val)
	}
}

func (c *svgCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
