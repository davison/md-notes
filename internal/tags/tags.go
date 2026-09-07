// Package tags collects tags from the markdown files of a root: a
// frontmatter "tags" value and inline hashtags in body text.
package tags

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Tag is one tag with the notes that carry it.
type Tag struct {
	Name  string   `json:"name"`
	Count int      `json:"count"`
	Notes []string `json:"notes"`
}

// inline matches a hashtag at the start of a line or after whitespace:
// letters, digits, _, - and /, and at least one letter somewhere.
var inline = regexp.MustCompile(`(?:^|[\s(])#([\pL\pN_/\-]*\pL[\pL\pN_/\-]*)`)

// Collect reads each of files (relative to root) and returns the tags
// found, sorted by count descending then name. Files that cannot be read
// are skipped.
func Collect(ctx context.Context, root string, files []string) []Tag {
	byTag := map[string]map[string]struct{}{}
	for _, f := range files {
		if ctx.Err() != nil {
			break
		}
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		for _, t := range Extract(src) {
			if byTag[t] == nil {
				byTag[t] = map[string]struct{}{}
			}
			byTag[t][f] = struct{}{}
		}
	}
	out := make([]Tag, 0, len(byTag))
	for name, notes := range byTag {
		t := Tag{Name: name, Count: len(notes), Notes: make([]string, 0, len(notes))}
		for n := range notes {
			t.Notes = append(t.Notes, n)
		}
		sort.Strings(t.Notes)
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Extract returns the distinct tags of one note, lower-cased and sorted.
func Extract(src []byte) []string {
	set := map[string]struct{}{}
	fm, body := splitFrontmatter(src)
	for _, t := range frontmatterTags(fm) {
		set[t] = struct{}{}
	}
	for _, t := range inlineTags(body) {
		set[t] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for t := range set {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func normalise(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	t = strings.TrimPrefix(t, "#")
	t = strings.Trim(t, "/-")
	return t
}

// frontmatterTags reads "tags" as a list of scalars, or one string split
// on commas and whitespace.
func frontmatterTags(fm map[string]any) []string {
	var out []string
	add := func(s string) {
		if n := normalise(s); n != "" {
			out = append(out, n)
		}
	}
	switch v := fm["tags"].(type) {
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				add(s)
			}
		}
	case string:
		for _, s := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			add(s)
		}
	}
	return out
}

// inlineTags scans body text outside fenced and inline code.
func inlineTags(body []byte) []string {
	var out []string
	inFence := false
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		trimmed := bytes.TrimLeft(line, " \t")
		if bytes.HasPrefix(trimmed, []byte("```")) || bytes.HasPrefix(trimmed, []byte("~~~")) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		text := stripInlineCode(string(line))
		for _, m := range inline.FindAllStringSubmatch(text, -1) {
			if n := normalise(m[1]); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

// stripInlineCode blanks the contents of backtick spans so a tag inside
// them is not counted.
func stripInlineCode(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if r == '`' {
			in = !in
			b.WriteRune(' ')
			continue
		}
		if in {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// splitFrontmatter is the same rule the renderer uses: a leading "---"
// line, a YAML mapping, and a closing "---" or "..." line.
func splitFrontmatter(src []byte) (map[string]any, []byte) {
	if !bytes.HasPrefix(src, []byte("---\n")) && !bytes.HasPrefix(src, []byte("---\r\n")) {
		return nil, src
	}
	rest := src[bytes.IndexByte(src, '\n')+1:]
	pos := 0
	for pos <= len(rest) {
		end := bytes.IndexByte(rest[pos:], '\n')
		var line []byte
		if end < 0 {
			line = rest[pos:]
			end = len(rest) - pos
		} else {
			line = rest[pos : pos+end]
		}
		t := bytes.TrimRight(line, "\r")
		if bytes.Equal(t, []byte("---")) || bytes.Equal(t, []byte("...")) {
			var fm map[string]any
			if err := yaml.Unmarshal(rest[:pos], &fm); err != nil {
				return nil, src
			}
			after := pos + end + 1
			if after > len(rest) {
				after = len(rest)
			}
			return fm, rest[after:]
		}
		pos += end + 1
	}
	return nil, src
}
