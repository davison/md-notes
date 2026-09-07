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
	"os/exec"
	"path"
	"strings"

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
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
		Submatches []struct {
			Start int `json:"start"`
			End   int `json:"end"`
		} `json:"submatches"`
	} `json:"data"`
}

// Search returns hits for query under root in path order, capped at
// MaxHits overall and MaxHitsPerFile per file. ripgrep's ignore rules
// apply; the markdown type filter is a type, not an include glob, so it
// does not override them.
func Search(ctx context.Context, root, query string, warnf func(string, ...any)) ([]Hit, error) {
	if warnf == nil {
		warnf = func(string, ...any) {}
	}
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("empty query")
	}
	rg, err := lookPath("rg")
	if err != nil {
		return nil, ErrNoRipgrep
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, rg,
		"--json", "--fixed-strings", "--ignore-case", "--type", "md",
		"--context", "1", "--max-count", fmt.Sprint(MaxHitsPerFile),
		"--sort", "path", "-e", query, ".")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("rg: %w", err)
	}

	var hits []Hit
	// Per file: the lines seen so far (matches and context) so a hit can
	// pick up the line before and after it.
	type fileLines struct {
		text    map[int]string
		matches int
	}
	files := map[string]*fileLines{}
	var pending []Hit // hits whose After may still arrive
	flushFile := func(p string) {
		fl := files[p]
		if fl == nil {
			return
		}
		for i := range pending {
			if pending[i].Path != p {
				continue
			}
			pending[i].Before = fl.text[pending[i].Line-1]
			pending[i].After = fl.text[pending[i].Line+1]
			hits = append(hits, pending[i])
		}
		pending = pending[:0]
		delete(files, p)
	}

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	full := false
	for sc.Scan() {
		var l rgLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			continue
		}
		p := path.Clean(strings.TrimPrefix(l.Data.Path.Text, "./"))
		switch l.Type {
		case "begin":
			if !tree.IsMarkdown(p) {
				continue
			}
			files[p] = &fileLines{text: map[int]string{}}
		case "match", "context":
			fl := files[p]
			if fl == nil {
				continue
			}
			text := strings.TrimRight(l.Data.Lines.Text, "\r\n")
			fl.text[l.Data.LineNumber] = text
			// rg emits a trailing context line that also matches as a match,
			// so the per-file cap is enforced here as well.
			if l.Type == "match" && fl.matches < MaxHitsPerFile {
				fl.matches++
				h := Hit{Path: p, Line: l.Data.LineNumber, Text: text}
				for _, sm := range l.Data.Submatches {
					h.Matches = append(h.Matches, [2]int{utf16Offset(text, sm.Start), utf16Offset(text, sm.End)})
				}
				pending = append(pending, h)
				if len(hits)+len(pending) >= MaxHits {
					full = true
				}
			}
		case "end":
			flushFile(p)
			if full {
				cancel()
			}
		}
		if full && len(hits) >= MaxHits {
			break
		}
	}
	for p := range files {
		flushFile(p)
	}
	runErr := cmd.Wait()
	if !full && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if runErr != nil && !full {
		var exit *exec.ExitError
		if errors.As(runErr, &exit) {
			switch {
			case exit.ExitCode() == 1 && stderr.Len() == 0:
				// No matches.
			case exit.ExitCode() == 2 && (len(hits) > 0 || stderr.Len() > 0):
				warnf("rg: search of %s reported errors: %s", root, strings.TrimSpace(stderr.String()))
			default:
				return nil, fmt.Errorf("rg: %w: %s", runErr, strings.TrimSpace(stderr.String()))
			}
		} else {
			return nil, fmt.Errorf("rg: %w", runErr)
		}
	}
	if len(hits) > MaxHits {
		hits = hits[:MaxHits]
	}
	return hits, nil
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
