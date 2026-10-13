package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestOpenMissingFileReturnsDefaults(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get(); got != Default() {
		t.Fatalf("got %+v, want defaults", got)
	}
}

func TestUpdatePersistsWithPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(c *Config) { c.KimaiURL = "https://kimai.example.com" }); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no Unix modes; the profile directory is private by ACL.
	if perm := info.Mode().Perm(); perm != 0o600 && runtime.GOOS != "windows" {
		t.Errorf("permissions = %o, want 600", perm)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Get().KimaiURL; got != "https://kimai.example.com" {
		t.Errorf("KimaiURL = %q", got)
	}
	if got := reopened.Get().Language; got != "de" {
		t.Errorf("Language = %q, want default de", got)
	}
}

func TestOpenInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestKeyring(t *testing.T) {
	keyring.MockInit()
	var k Keyring
	if _, err := k.Get(SecretKimaiToken); err != ErrSecretNotFound {
		t.Fatalf("Get on empty keyring: %v", err)
	}
	if err := k.Set(SecretKimaiToken, "s3cret"); err != nil {
		t.Fatal(err)
	}
	if v, err := k.Get(SecretKimaiToken); err != nil || v != "s3cret" {
		t.Fatalf("Get = %q, %v", v, err)
	}
	if err := k.Delete(SecretKimaiToken); err != nil {
		t.Fatal(err)
	}
	if err := k.Delete(SecretKimaiToken); err != nil {
		t.Fatalf("deleting a missing secret should succeed: %v", err)
	}
}

func TestFileSecrets(t *testing.T) {
	f := &FileSecrets{Path: filepath.Join(t.TempDir(), "secrets.json")}
	if _, err := f.Get("a"); err != ErrSecretNotFound {
		t.Fatalf("Get on empty: %v", err)
	}
	if err := f.Set("a", "1"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(f.Path)
	if err != nil || (info.Mode().Perm() != 0o600 && runtime.GOOS != "windows") {
		t.Fatalf("stat = %v, %v; want mode 600", info, err)
	}
	if v, err := f.Get("a"); err != nil || v != "1" {
		t.Fatalf("Get = %q, %v", v, err)
	}
	if err := f.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.Path); !os.IsNotExist(err) {
		t.Errorf("file should be removed when empty: %v", err)
	}
	if err := f.Delete("a"); err != nil {
		t.Errorf("deleting a missing secret: %v", err)
	}
}
