// Package search runs a literal, case-insensitive phrase search over the
// markdown files of a root with ripgrep and returns hits with context.
package search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/davison/md-notes/internal/tree"
)

// Hit is one matching line.
type Hit struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
	// Matches are [start, end) offsets into Text in UTF-16 code units, so
	// JavaScript string indexing lines up with them.
	Matches [][2]int `json:"matches"`
	Before  string   `json:"before,omitempty"`
	After   string   `json:"after,omitempty"`
}

// Limits on what one search returns.
const (
	MaxHits        = 200
	MaxHitsPerFile = 20
	// MaxText bounds a hit's line, as a window around its first match, and
	// MaxContext bounds each context line, both in runes. A very long line
	// would otherwise make one keystroke a huge response.
	MaxText    = 300
	MaxContext = 200
)

// ErrNoRipgrep is returned when the rg binary cannot be found.
var ErrNoRipgrep = tree.ErrNoRipgrep

var lookPath = exec.LookPath

// rg's JSON stream: one object per line.
type rgLine struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text  string `json:"text"`
			Bytes []byte `json:"bytes"` // base64 in the stream; set for non-UTF-8 lines
		} `json:"lines"`
		LineNumber int `json:"line_number"`
		Submatches []struct {
			Start int `json:"start"`
			End   int `json:"end"`
		} `json:"submatches"`
	} `json:"data"`
}

// ErrBadQuery is returned for a query that cannot be searched for: empty,
// or containing control characters.
var ErrBadQuery = errors.New("query must be non-empty text without control characters")

// Result is what a search returns.
type Result struct {
	Hits []Hit `json:"hits"`
	// Truncated reports that more hits existed than MaxHits.
	Truncated bool `json:"truncated"`
}

// Search returns hits for query under root in path order, capped at
// MaxHits overall and MaxHitsPerFile per file. ripgrep's ignore rules
// apply: the markdown type filter honours them, unlike an include glob.
// The type filter does re-include hidden files, so those are excluded
// again with a glob, matching the navigator.
func Search(ctx context.Context, root, query string, warnf func(string, ...any)) (Result, error) {
	if warnf == nil {
		warnf = func(string, ...any) {}
	}
	if strings.TrimSpace(query) == "" || strings.ContainsFunc(query, func(r rune) bool { return (r < ' ' && r != '\t') || r == 0x7f }) {
		return Result{}, ErrBadQuery
	}
	rg, err := lookPath("rg")
	if err != nil {
		return Result{}, ErrNoRipgrep
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, rg,
		"--json", "--fixed-strings", "--ignore-case", "--type", "md", "--glob", "!.*",
		"--context", "1", "--max-count", fmt.Sprint(MaxHitsPerFile),
		"--sort", "path", "-e", query, ".")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("rg: %w", err)
	}

	var hits []Hit
	// Per file: the lines seen so far (matches and context) so a hit can
	// pick up the line before and after it.
	type fileLines struct {
		text    map[int]string
		matches int
		pending []Hit // hits whose After may still arrive
	}
	files := map[string]*fileLines{}
	flushFile := func(p string) {
		fl := files[p]
		if fl == nil {
			return
		}
		for _, h := range fl.pending {
			h.Before = clip(fl.text[h.Line-1], MaxContext)
			h.After = clip(fl.text[h.Line+1], MaxContext)
			hits = append(hits, window(h))
		}
		delete(files, p)
	}

	// Lines are read with an unbounded reader: a match line can be as long
	// as any line in a note, and the stream is JSON per line.
	rd := bufio.NewReaderSize(stdout, 64*1024)
	full := false
	var readErr error
	for {
		raw, err := rd.ReadBytes('\n')
		if len(raw) > 0 {
			var l rgLine
			if json.Unmarshal(raw, &l) == nil {
				p := path.Clean(strings.TrimPrefix(l.Data.Path.Text, "./"))
				switch l.Type {
				case "begin":
					if tree.IsMarkdown(p) {
						files[p] = &fileLines{text: map[int]string{}}
					}
				case "match", "context":
					fl := files[p]
					if fl == nil {
						break
					}
					text := l.Data.Lines.Text
					if text == "" && len(l.Data.Lines.Bytes) > 0 {
						text = string(l.Data.Lines.Bytes)
					}
					text = strings.TrimRight(text, "\r\n")
					fl.text[l.Data.LineNumber] = text
					// rg emits a trailing context line that also matches as a
					// match, so the per-file cap is enforced here as well.
					if l.Type == "match" && fl.matches < MaxHitsPerFile {
						fl.matches++
						h := Hit{Path: p, Line: l.Data.LineNumber, Text: text}
						for _, sm := range l.Data.Submatches {
							h.Matches = append(h.Matches, [2]int{utf16Offset(text, sm.Start), utf16Offset(text, sm.End)})
						}
						fl.pending = append(fl.pending, h)
						if len(hits)+fl.matches > MaxHits {
							full = true
						}
					}
				case "end":
					flushFile(p)
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				readErr = err
			}
			break
		}
		if full {
			// One more than the cap has been seen, which is all that is
			// needed to report truncation. Stop rg and drain it.
			cancel()
			io.Copy(io.Discard, rd)
			break
		}
	}
	for p := range files {
		flushFile(p)
	}
	runErr := cmd.Wait()
	if !full && ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	if readErr != nil {
		return Result{}, fmt.Errorf("rg: reading output: %w", readErr)
	}
	if runErr != nil && !full {
		var exit *exec.ExitError
		if errors.As(runErr, &exit) {
			switch {
			case exit.ExitCode() == 1 && stderr.Len() == 0:
				// No matches.
			case exit.ExitCode() == 2 && len(hits) > 0:
				warnf("rg: search of %s reported errors: %s", root, strings.TrimSpace(stderr.String()))
			default:
				return Result{}, fmt.Errorf("rg: %w: %s", runErr, strings.TrimSpace(stderr.String()))
			}
		} else {
			return Result{}, fmt.Errorf("rg: %w", runErr)
		}
	}
	res := Result{Hits: hits, Truncated: len(hits) > MaxHits}
	if res.Truncated {
		res.Hits = hits[:MaxHits]
	}
	if res.Hits == nil {
		res.Hits = []Hit{}
	}
	return res, nil
}

// utf16Offset converts a byte offset into s to a UTF-16 code unit offset.
func utf16Offset(s string, byteOff int) int {
	if byteOff > len(s) {
		byteOff = len(s)
	}
	n := 0
	for _, r := range s[:byteOff] {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// clip shortens s to at most n runes, marking the cut.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n]) + "…"
}

// window shortens a long hit line to MaxText runes around its first match,
// adjusting match offsets and dropping matches that fall outside.
func window(h Hit) Hit {
	if utf8.RuneCountInString(h.Text) <= MaxText {
		return h
	}
	units := utf16.Encode([]rune(h.Text))
	start := 0
	if len(h.Matches) > 0 {
		start = h.Matches[0][0] - MaxText/3
	}
	if start < 0 {
		start = 0
	}
	end := start + MaxText
	if end > len(units) {
		end = len(units)
		start = max(0, end-MaxText)
	}
	var out Hit
	out.Path, out.Line = h.Path, h.Line
	prefix, suffix := "", ""
	if start > 0 {
		prefix = "…"
	}
	if end < len(units) {
		suffix = "…"
	}
	out.Text = prefix + string(utf16.Decode(units[start:end])) + suffix
	shift := len(utf16.Encode([]rune(prefix))) - start
	for _, m := range h.Matches {
		if m[0] >= start && m[1] <= end {
			out.Matches = append(out.Matches, [2]int{m[0] + shift, m[1] + shift})
		}
	}
	out.Before, out.After = h.Before, h.After
	return out
}
