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

func TestStoreValid(t *testing.T) {
	path := tokenPath(t)
	value, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore(path, value)
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

func TestStoreSeesARotatedTokenWithoutRestart(t *testing.T) {
	path := tokenPath(t)
	old, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore(path, old)
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
	value, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore(path, value)
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
	path := tokenPath(t)
	value, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore(path, value)
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
