// Package clip turns a web clipping into a new note inside the notes root.
// It is the only part of the daemon that creates a file rather than
// replacing one, so the confinement is its own concern: every path it
// composes is created through a directory handle opened on the root.
package clip

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/davison/md-notes/internal/roots"
	"gopkg.in/yaml.v3"
)

// maxSlug bounds the slug taken from a title. A page title can be a
// sentence, and the name still has to be readable in a file listing and
// short enough to survive being copied about.
const maxSlug = 64

// maxTitle bounds the title carried into the frontmatter, in runes, since
// what makes a header unreadable is its length on screen and not its
// length in bytes. It is not a filename and can afford to be longer than
// the slug.
const maxTitle = 300

// maxCollisions bounds the numeric suffix search, so that a directory that
// cannot accept the file reports an error rather than spinning.
const maxCollisions = 500

// FallbackSlug names a clip whose title yields no ASCII letters or digits
// at all — an empty title, or one written entirely in a script the slug
// cannot represent. The frontmatter still carries the title and the source.
const FallbackSlug = "untitled"

// Kinds are what a clip can be.
const (
	KindPage      = "page"
	KindSelection = "selection"
)

var (
	// ErrKind is returned for a kind that is neither page nor selection.
	ErrKind = errors.New("kind must be page or selection")
	// ErrCrowded is returned when the dated name and every suffix within
	// the search are taken.
	ErrCrowded = errors.New("too many clips of the same name on the same day")
)

// Clip is one clipping as the browser sends it.
type Clip struct {
	URL      string
	Title    string
	Markdown string
	Kind     string
}

// Write creates a note for c under dir inside root and returns its path
// relative to the root, slash-separated. dir is created when it is
// missing. The name is the clip's date, a slug of its title, and a numeric
// suffix if that name is taken.
func Write(root roots.Root, dir string, c Clip, now time.Time) (string, error) {
	if c.Kind != KindPage && c.Kind != KindSelection {
		return "", ErrKind
	}
	dir = path.Clean(strings.TrimSpace(dir))
	if dir == "" {
		dir = "."
	}
	body, err := note(c, now)
	if err != nil {
		return "", err
	}

	handle, err := root.Open()
	if err != nil {
		return "", err
	}
	defer handle.Close()
	if dir != "." {
		if err := ensureDir(root, handle, dir); err != nil {
			return "", err
		}
	}

	base := now.Format("2006-01-02") + "-" + Slug(c.Title)
	for n := 1; n <= maxCollisions; n++ {
		name := base + ".md"
		if n > 1 {
			name = fmt.Sprintf("%s-%d.md", base, n)
		}
		rel := name
		if dir != "." {
			rel = path.Join(dir, name)
		}
		f, err := handle.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := writeAll(f, body); err != nil {
			handle.Remove(rel)
			return "", err
		}
		return rel, nil
	}
	return "", ErrCrowded
}

// ensureDir makes dir inside the root, and reports a dir that resolves out
// of it as ErrOutside rather than as whatever the handle happens to say.
// The handle is the enforcement; this is the diagnosis.
func ensureDir(root roots.Root, handle *os.Root, dir string) error {
	switch _, err := root.Resolve(dir); {
	case err == nil:
		return nil
	case errors.Is(err, roots.ErrOutside):
		return err
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	if err := handle.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// A symlink raced in under the new directory is still outside.
	if _, err := root.Resolve(dir); err != nil {
		return err
	}
	return nil
}

// writeAll writes the note and makes its bytes durable before reporting
// success, the way the save path stages its replacement: a 201 the caller
// acts on should not name a note that a power cut leaves empty. The
// directory entry is not synced, so the API promises no more about a new
// name surviving a crash than the save path promises about a rename.
func writeAll(f *os.File, body []byte) error {
	if _, err := f.Write(body); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// flowStrings marshals as a YAML flow sequence, so the frontmatter reads
// `tags: [clip]` rather than a block list.
type flowStrings []string

func (v flowStrings) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, s := range v {
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: s})
	}
	return node, nil
}

// frontmatter is the note's header, in the order it is written.
type frontmatter struct {
	Title   string      `yaml:"title"`
	Source  string      `yaml:"source"`
	Clipped string      `yaml:"clipped"`
	Tags    flowStrings `yaml:"tags"`
}

// note renders the whole file: frontmatter, a blank line, and the markdown
// exactly as it was given.
func note(c Clip, now time.Time) ([]byte, error) {
	front, err := yaml.Marshal(frontmatter{
		Title:   Title(c.Title),
		Source:  c.URL,
		Clipped: now.Format(time.RFC3339),
		Tags:    flowStrings{"clip"},
	})
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(front)
	b.WriteString("---\n\n")
	b.WriteString(c.Markdown)
	return []byte(b.String()), nil
}

// Title is the title as the frontmatter carries it: one line, trimmed, and
// bounded to maxTitle runes, so that a page whose <title> is a paragraph
// cannot make the header unreadable. An empty title is left empty rather
// than invented.
//
// The bound counts runes rather than bytes because cutting a UTF-8
// sequence in half leaves a string YAML cannot emit as text at all: the
// encoder falls back to a base64 !!binary blob, and a long Japanese title
// — which already yields the `untitled` slug — would lose the one place
// the title was still readable.
func Title(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	if utf8.RuneCountInString(title) > maxTitle {
		title = strings.TrimSpace(string([]rune(title)[:maxTitle]))
	}
	return title
}

// Slug turns a title into the filename part: lower case, ASCII, words
// joined by single hyphens, bounded in length. Latin letters carrying an
// accent fold to the letter; everything else the alphabet cannot represent
// becomes a separator, so a title in another script yields FallbackSlug.
func Slug(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 0xc0 && r <= 0xff:
			b.WriteString(latin1[r-0xc0])
		default:
			b.WriteByte('-')
		}
	}
	// Every byte written above is ASCII, so the bound below counts
	// characters as well as bytes and cannot cut a rune in half.
	slug := trimRuns(b.String())
	if len(slug) > maxSlug {
		slug = slug[:maxSlug]
		if cut := strings.LastIndexByte(slug, '-'); cut > 0 {
			slug = slug[:cut]
		}
		slug = strings.Trim(slug, "-")
	}
	if slug == "" {
		return FallbackSlug
	}
	return slug
}

// trimRuns collapses runs of hyphens and trims them from both ends.
func trimRuns(s string) string {
	var b strings.Builder
	dash := true // leading hyphens are dropped
	for i := range len(s) {
		if s[i] == '-' {
			dash = true
			continue
		}
		if dash && b.Len() > 0 {
			b.WriteByte('-')
		}
		dash = false
		b.WriteByte(s[i])
	}
	return b.String()
}

// latin1 folds U+00C0..U+00FF — the accented Latin letters an English or
// European page title actually contains — to ASCII. Anything beyond it is
// a separator, which the fallback covers.
var latin1 = [64]string{
	"a", "a", "a", "a", "a", "a", "ae", "c", "e", "e", "e", "e", "i", "i", "i", "i",
	"d", "n", "o", "o", "o", "o", "o", "-", "o", "u", "u", "u", "u", "y", "th", "ss",
	"a", "a", "a", "a", "a", "a", "ae", "c", "e", "e", "e", "e", "i", "i", "i", "i",
	"d", "n", "o", "o", "o", "o", "o", "-", "o", "u", "u", "u", "u", "y", "th", "y",
}
