package connector

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	content := []byte("hello, gloss\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}

	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Errorf("hashFile = %q, want %q", got, want)
	}
}

func TestHashFileSameContentSameHash(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	content := []byte("identical content")
	if err := os.WriteFile(a, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, content, 0o644); err != nil {
		t.Fatal(err)
	}

	ha, err := hashFile(a)
	if err != nil {
		t.Fatalf("hashFile(a): %v", err)
	}
	hb, err := hashFile(b)
	if err != nil {
		t.Fatalf("hashFile(b): %v", err)
	}
	if ha != hb {
		t.Errorf("hashes differ for identical content: %q vs %q", ha, hb)
	}
}

func TestHashFileNonexistent(t *testing.T) {
	if _, err := hashFile(filepath.Join(t.TempDir(), "nope.txt")); err == nil {
		t.Error("hashFile on nonexistent path succeeded, want error")
	}
}
