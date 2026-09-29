package source

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"unicode/utf8"
)

// ResourcesDir is where uploaded images go: one directory at the top of
// the root, whatever folder the note linking to it is in. It is the layout
// the operator's notes already have, carried over from Joplin.
const ResourcesDir = "_resources"

// MaxUploadBytes is the largest image Upload accepts: twice the note cap,
// room for a phone's photo, and small enough that one request cannot fill
// a disk quickly.
const MaxUploadBytes = 16 << 20

// maxSuffix bounds the -2, -3, … candidates tried for a dropped file's
// name. A folder holding a thousand files called shot-N.png is a folder
// the caller should hear about rather than one this loop walks forever.
const maxSuffix = 1000

// maxStem is the most bytes of a dropped file's own name that are kept.
// Well inside NAME_MAX once a suffix and an extension are added.
const maxStem = 100

var (
	ErrUploadTooLarge = errors.New("image exceeds 16 MiB")
	ErrImageType      = errors.New("not a PNG, JPEG, GIF, WebP or SVG image")
)

// Uploaded is an image Upload has put in, or found in, _resources. Path is
// relative to the root and slash-separated; Name is its last segment.
// Created is false when a file already there held exactly these bytes and
// was reused rather than written again.
type Uploaded struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

// imageTypes maps what content sniffing reports to the extension the file
// is given. The extension is always the daemon's: a name the client sent
// never decides what the file is served as.
var imageTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// imageExt reports the extension for data, judged by its content alone,
// or false for anything that is not one of the accepted image types.
func imageExt(data []byte) (string, bool) {
	if ext, ok := imageTypes[http.DetectContentType(data)]; ok {
		return ext, true
	}
	if isSVG(data) {
		return ".svg", true
	}
	return "", false
}

const svgNamespace = "http://www.w3.org/2000/svg"

// isSVG reports whether data is a well-formed XML document whose root
// element is <svg> in the SVG namespace. An SVG's script is not refused
// here: the raw route serves every file under a sandbox policy, and that
// is what keeps it from running with the UI's origin.
func isSVG(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	root := false
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return root
		}
		if err != nil {
			return false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if !root {
				if t.Name.Space != svgNamespace || t.Name.Local != "svg" {
					return false
				}
				root = true
			}
		case xml.CharData:
			if !root && len(bytes.TrimSpace(t)) > 0 {
				return false
			}
		}
	}
}

// uploadStem is the name an upload is given, before any suffix and the
// extension. A dropped file keeps its own base name, reduced to letters,
// digits, dot, underscore and hyphen so a link to it needs no escaping
// anywhere; only the hint's last segment is read, so it is a name and
// never a path. A paste, or a name that reduces to nothing, is named by
// the first 32 hex digits of the image's SHA-256, which also makes the same
// image pasted twice the same file.
func uploadStem(hint string, data []byte) string {
	if i := strings.LastIndexAny(hint, `/\`); i >= 0 {
		hint = hint[i+1:]
	}
	hint = strings.TrimSuffix(hint, path.Ext(hint))
	var b strings.Builder
	dash := false
	for _, r := range hint {
		if r < 0x80 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	stem := strings.Trim(b.String(), ".-")
	if len(stem) > maxStem {
		stem = strings.TrimRight(stem[:maxStem], ".-")
	}
	if stem == "" {
		sum := sha256.Sum256(data)
		stem = hex.EncodeToString(sum[:16])
	}
	return stem
}

// Upload puts an image into the root's _resources directory, making the
// directory if it is missing, and returns where it went. The type is
// judged from the bytes; hint is the dropped file's name, or empty for a
// paste (see uploadStem).
//
// It never replaces anything. Every candidate name is created
// O_CREATE|O_EXCL through a handle on _resources, and a name that is
// already taken is only ever read: a regular file there holding exactly
// these bytes is reused, and anything else — a different file, a
// directory, a link — moves on to the next suffix. A _resources that
// resolves outside the root is refused before anything is made.
func (s *Store) Upload(slug, hint string, data []byte) (Uploaded, error) {
	if len(data) > MaxUploadBytes {
		return Uploaded{}, ErrUploadTooLarge
	}
	ext, ok := imageExt(data)
	if !ok {
		return Uploaded{}, ErrImageType
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, ok := s.reg.Get(slug)
	if !ok {
		return Uploaded{}, os.ErrNotExist
	}
	if err := root.EnsureDir(ResourcesDir); err != nil {
		return Uploaded{}, err
	}
	dir, _, err := root.OpenDir(ResourcesDir)
	if err != nil {
		return Uploaded{}, err
	}
	defer dir.Close()
	stem := uploadStem(hint, data)
	for i := 1; i <= maxSuffix; i++ {
		name := stem + ext
		if i > 1 {
			name = fmt.Sprintf("%s-%d%s", stem, i, ext)
		}
		done := func(created bool) (Uploaded, error) {
			return Uploaded{Path: ResourcesDir + "/" + name, Name: name, Created: created}, nil
		}
		if info, err := dir.Lstat(name); err == nil {
			if info.Mode().IsRegular() && info.Size() == int64(len(data)) && holds(dir, name, data) {
				return done(false)
			}
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return Uploaded{}, err
		}
		f, err := dir.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return Uploaded{}, err
		}
		if err := writeAll(f, data); err != nil {
			dir.Remove(name)
			return Uploaded{}, err
		}
		return done(true)
	}
	return Uploaded{}, ErrExists
}

// holds reports whether the file name in dir holds exactly data. It reads
// one byte more than data, so a file that has grown since it was statted
// is not taken for a match.
func holds(dir *os.Root, name string, data []byte) bool {
	f, err := dir.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	got, err := io.ReadAll(io.LimitReader(f, int64(len(data))+1))
	return err == nil && bytes.Equal(got, data)
}
