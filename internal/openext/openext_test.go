package openext

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenEmptyPathErrors(t *testing.T) {
	if err := Open(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestOpenMissingFileErrors(t *testing.T) {
	err := Open(filepath.Join(t.TempDir(), "does-not-exist.pdf"))
	if err == nil {
		t.Fatal("expected error for a nonexistent file")
	}
}

func TestOpenMissingHandlerBinaryErrors(t *testing.T) {
	// Open shells out to an OS-specific handler (xdg-open on Linux); with an
	// empty PATH it must report that clearly rather than panic or hang.
	dir := t.TempDir()
	path := filepath.Join(dir, "a.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", "")
	err := Open(path)
	if err == nil {
		t.Fatal("expected error when no opener binary is on PATH")
	}
}
