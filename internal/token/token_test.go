package token

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func tokenPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "state", "token")
}

func TestLoadGeneratesOnceAtMode0600(t *testing.T) {
	path := tokenPath(t)
	value, created, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Error("first load did not report creating the token")
	}
	if len(value) < 20 {
		t.Errorf("token %q is too short to be a secret", value)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != FileMode {
		t.Errorf("token file mode %v, want %v", got, FileMode)
	}
	if dir, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	} else if got := dir.Mode().Perm(); got&0o077 != 0 {
		t.Errorf("state directory mode %v is group/world accessible", got)
	}

	again, created, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("second load reported creating the token again")
	}
	if again != value {
		t.Errorf("second load returned %q, want the stored %q", again, value)
	}
}

func TestLoadReplacesAnEmptyFile(t *testing.T) {
	path := tokenPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, created, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !created || value == "" {
		t.Fatalf("empty file: value %q created %v, want a generated token", value, created)
	}
}

func TestLoadTightensAWidePermission(t *testing.T) {
	path := tokenPath(t)
	if _, _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != FileMode {
		t.Errorf("mode after load %v, want %v", got, FileMode)
	}
}

func TestLoadRefusesAFileThatIsNotAToken(t *testing.T) {
	path := tokenPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(path); !errors.Is(err, ErrMalformed) {
		t.Errorf("error %v, want ErrMalformed", err)
	}
}

func TestRotateReplacesTheStoredToken(t *testing.T) {
	path := tokenPath(t)
	first, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Rotate(path)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("rotate returned the same token")
	}
	stored, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if stored != second {
		t.Errorf("stored %q, want the rotated %q", stored, second)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != FileMode {
		t.Errorf("mode after rotate %v, want %v", got, FileMode)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Error("the temporary file survived the rotation")
	}
}

// openStore is the daemon's own path to a Store, with the token it holds.
func openStore(t *testing.T, path string) (*Store, string) {
	t.Helper()
	s, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	value, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, value
}

func TestStoreValid(t *testing.T) {
	s, value := openStore(t, tokenPath(t))
	for presented, want := range map[string]bool{
		value:                    true,
		"":                       false,
		value + "x":              false,
		strings.ToLower(value):   false,
		value[:len(value)-1]:     false,
		"Bearer " + value:        false,
		strings.Repeat("a", 200): false,
	} {
		if got := s.Valid(presented); got != want {
			t.Errorf("Valid(%q) = %v, want %v", presented, got, want)
		}
	}
}

// Rotation is a revocation: the replaced token must stop working on the
// next request, whether or not anything ever presents the new one.
func TestStoreRefusesTheReplacedTokenImmediately(t *testing.T) {
	path := tokenPath(t)
	s, old := openStore(t, path)
	if !s.Valid(old) {
		t.Fatal("the current token was refused")
	}
	if _, err := Rotate(path); err != nil {
		t.Fatal(err)
	}
	if s.Valid(old) {
		t.Error("the replaced token is still accepted after rotation")
	}
}

func TestStoreSeesARotatedTokenWithoutRestart(t *testing.T) {
	path := tokenPath(t)
	s, old := openStore(t, path)
	fresh, err := Rotate(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Valid(fresh) {
		t.Error("the rotated token was refused; rotation needs no restart")
	}
	if s.Valid(old) {
		t.Error("the replaced token is still accepted")
	}
}

func TestStoreRefusesEverythingWhenTheFileIsGone(t *testing.T) {
	path := tokenPath(t)
	s, value := openStore(t, path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	// The held value still stands: a deleted file is not a revocation, and
	// the daemon has no way to tell one from a half-finished rotation.
	if !s.Valid(value) {
		t.Error("the held token was refused after the file was removed")
	}
	if s.Valid("nonsense") {
		t.Error("a wrong token was accepted")
	}
}

func TestStoreIsSafeForConcurrentUse(t *testing.T) {
	s, value := openStore(t, tokenPath(t))
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				s.Valid(value)
				s.Valid("wrong")
			}
		}()
	}
	wg.Wait()
}

// Reading the token repairs the file's permissions, so it must not follow
// a link: a --token-file pointed at one would otherwise read and chmod a
// file the user never nominated as the token.
func TestReadRefusesASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("secretline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if value, _, err := Load(link); !errors.Is(err, ErrMalformed) {
		t.Errorf("Load through a symlink = %q, %v; want ErrMalformed", value, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("the link's target was chmodded to %v", got)
	}
	// Rotation replaces the link rather than writing through it.
	if _, err := Rotate(link); err != nil {
		t.Fatal(err)
	}
	link2, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if !link2.Mode().IsRegular() {
		t.Error("rotation left a symlink in place")
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "secretline\n" {
		t.Errorf("the link's target was written through: %q, %v", data, err)
	}
}

func TestReadRefusesADirectory(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := Load(dir); !errors.Is(err, ErrMalformed) {
		t.Errorf("error = %v, want ErrMalformed", err)
	}
}

// Open reads the value and stamps the file as one operation. Were they
// separate, a rotation landing between them would leave the store holding
// the superseded token with the *new* file's identity, so nothing would
// ever prompt a re-read: the daemon would serve the old token and refuse
// the current one.
func TestOpenCannotPinASupersededToken(t *testing.T) {
	path := tokenPath(t)
	first, created, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("the token already existed")
	}
	s, created, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("Open reported creating a token that already existed")
	}
	if !s.Valid(first) {
		t.Fatal("the stored token was refused")
	}
	// Whatever the store holds, the file is the authority the moment it
	// changes.
	fresh, err := Rotate(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Valid(fresh) || s.Valid(first) {
		t.Error("the store did not follow the file")
	}
}

func TestOpenGeneratesWhenAbsent(t *testing.T) {
	path := tokenPath(t)
	s, created, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Error("Open did not report creating the token")
	}
	value, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Valid(value) {
		t.Error("the store does not hold the token it just generated")
	}
}

// A chmod changes no field the identity comparison sees, so the repair
// has to ride on the stat the store already does.
func TestStoreRepairsPermissionsWhileRunning(t *testing.T) {
	path := tokenPath(t)
	s, value := openStore(t, path)
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	if !s.Valid(value) {
		t.Fatal("the current token was refused")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != FileMode {
		t.Errorf("mode after an authenticated request %v, want %v", got, FileMode)
	}
}

// Two rotations at once must not share a staging file.
func TestConcurrentRotationsDoNotCollide(t *testing.T) {
	path := tokenPath(t)
	if _, _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Rotate(path); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent rotation: %v", err)
	}
	value, _, err := Load(path)
	if err != nil || value == "" {
		t.Fatalf("token after the rotations = %q, %v", value, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("%d files in the state directory, want only the token", len(entries))
	}
}
