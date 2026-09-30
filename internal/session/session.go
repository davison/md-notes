// Package session makes and checks the browser sessions the daemon issues
// on the tailnet host. A browser cannot put the bearer token in a header
// on every request, so it presents it once to a login page and is given a
// session instead; this is what that session is.
//
// A session is a signed value and nothing else: nothing about it is held
// in memory or written to disk (davison/md-notes#231). The value carries
// the time it was issued, a random id and the host name it was issued
// under, with an HMAC-SHA256 over all of them keyed by a key derived from
// the daemon's token (token.Store.Derive with KeyPurpose). So:
//
//   - a restart, an upgrade or a reboot changes nothing: the daemon that
//     starts over the same token file derives the same key, and the cookie
//     the browser kept is still a session;
//   - `mdn token --rotate`, or a token file replaced while the daemon was
//     stopped, ends every session at once: the new token derives a new key,
//     and no value signed with the old one checks;
//   - nothing beyond the token file is a secret at rest, which is what the
//     in-memory design on #39 was protecting, and the token itself is never
//     in the cookie, nor recoverable from it.
//
// What bounds a session on a device nobody uses is the idle limit: a
// session is reissued, at most once a day, on the requests that use it,
// and refused once it has gone Idle without one.
package session

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

// KeyPurpose names the key a session is signed with, among any others the
// token might one day derive. The version is in it so that changing what a
// session is changes the key too, and every older value simply fails.
const KeyPurpose = "mdn session key v1"

// Idle is how long a session lasts without being used: the browser is
// told the same thing, as the cookie's Max-Age. A device in use never
// reaches it, since every use at least a day after the last reissue
// reissues the session; a browser used once and left behind — the
// borrowed one #39 worried about — is logged out when it runs out.
// Chromium caps a cookie's lifetime at 400 days, so this cannot be longer.
const Idle = 30 * 24 * time.Hour

// RefreshAfter is how old a session must be before a request that uses it
// is answered with a fresh one. Not every response: #39 rejected a
// Set-Cookie on each one, and a day is short beside Idle.
const RefreshAfter = 24 * time.Hour

// maxSkew is how far in the future a session's issue time may be. The
// daemon's own clock issued it, so one from the future means the clock
// moved back; a few minutes of that is a clock being corrected, not a
// reason to log anyone out.
const maxSkew = 5 * time.Minute

// version is the first field of a value, and part of what is signed.
const version = "v1"

// idBytes is the size of the random id, which is there so that two
// sessions issued in the same second are two different values.
const idBytes = 16

var b64 = base64.RawURLEncoding.Strict()

// Issue returns a new session value for host, signed with key, issued at
// now. The value is `v1.<unix seconds>.<id>.<mac>`, every field of it safe
// in a cookie without quoting.
func Issue(key []byte, host string, now time.Time) string {
	id := make([]byte, idBytes)
	rand.Read(id)
	issued := strconv.FormatInt(now.Unix(), 10)
	enc := b64.EncodeToString(id)
	return version + "." + issued + "." + enc + "." + b64.EncodeToString(sign(key, host, version, issued, enc))
}

// Why a session is refused, as Verify reports it. Each names a category
// and nothing more: none quotes the value, so any of them can go in the
// daemon's log (M13-R2).
var (
	// ErrMalformed is a value that is not the shape Issue makes: a field
	// missing or extra, a MAC or id of the wrong length, an issue time
	// written any other way — or a session id from v0.3.0, which was a
	// bare random string.
	ErrMalformed = errors.New("not a session value")
	// ErrSignature is a well-formed value whose MAC does not check under
	// this key and host: signed with another token (one rotated away, or
	// another daemon's), for another tailnet name, or altered.
	ErrSignature = errors.New("bad signature")
	// ErrExpired is a session unused for Idle.
	ErrExpired = errors.New("expired: unused for the idle limit")
	// ErrFuture is a session issued further ahead than the clock allows.
	ErrFuture = errors.New("issued in the future")
)

// Check reports whether value is a session signed with key for host and
// still live at now, and whether it is old enough to be reissued. It is
// Verify without the reason.
func Check(key []byte, host, value string, now time.Time) (ok, due bool) {
	due, err := Verify(key, host, value, now)
	return err == nil, due
}

// Verify is Check that says why a value was refused: one of ErrMalformed,
// ErrSignature, ErrExpired or ErrFuture. Anything that is not exactly the
// shape Issue makes is refused before the MAC is compared, and the
// comparison is constant-time.
func Verify(key []byte, host, value string, now time.Time) (due bool, err error) {
	if len(key) == 0 || host == "" {
		return false, ErrSignature
	}
	parts := strings.Split(value, ".")
	if len(parts) != 4 || parts[0] != version {
		return false, ErrMalformed
	}
	issued, id, mac := parts[1], parts[2], parts[3]
	secs, err := strconv.ParseInt(issued, 10, 64)
	if err != nil || strconv.FormatInt(secs, 10) != issued {
		return false, ErrMalformed
	}
	if raw, err := b64.DecodeString(id); err != nil || len(raw) != idBytes {
		return false, ErrMalformed
	}
	got, err := b64.DecodeString(mac)
	if err != nil || len(got) != sha256.Size {
		return false, ErrMalformed
	}
	if !hmac.Equal(got, sign(key, host, version, issued, id)) {
		return false, ErrSignature
	}
	at := time.Unix(secs, 0)
	if at.After(now.Add(maxSkew)) {
		return false, ErrFuture
	}
	if !now.Before(at.Add(Idle)) {
		return false, ErrExpired
	}
	return !now.Before(at.Add(RefreshAfter)), nil
}

// sign is the MAC over every field, and the host. The fields are joined
// with a byte none of them can contain, so no two different sets of
// fields sign the same bytes.
func sign(key []byte, host, version, issued, id string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(version + "\x00" + host + "\x00" + issued + "\x00" + id))
	return m.Sum(nil)
}
