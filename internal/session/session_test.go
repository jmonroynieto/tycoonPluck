package session

import (
	"os"
	"path/filepath"
	"testing"

	"tycoonPluck/internal/sorter"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRecordAssignAndPopRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := LoadFrom(path)
	dir := t.TempDir()
	src := filepath.Join(dir, "a.pdf")
	dest := filepath.Join(dir, "Work", "a.pdf")
	touch(t, dest)

	entry := sorter.UndoEntry{
		OriginalPath: src,
		MovedTo:      dest,
		Category:     "Work",
	}
	if err := s.RecordAssign(dir, entry); err != nil {
		t.Fatal(err)
	}

	s2 := LoadFrom(path)
	und := s2.UndoEntriesStillOnDisk(dir)
	if len(und) != 1 {
		t.Fatalf("undo entries = %d, want 1", len(und))
	}
	if und[0].Category != "Work" || und[0].MovedTo != dest {
		t.Errorf("unexpected entry: %+v", und[0])
	}

	if err := s2.PopLastAssign(dir); err != nil {
		t.Fatal(err)
	}
	s3 := LoadFrom(path)
	if n := len(s3.UndoEntriesStillOnDisk(dir)); n != 0 {
		t.Errorf("after pop, entries = %d", n)
	}
}

func TestRecordSkipPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := LoadFrom(path)
	dir := t.TempDir()
	if err := s.RecordSkip(dir, "skip-me.pdf"); err != nil {
		t.Fatal(err)
	}
	s2 := LoadFrom(path)
	set := s2.SkippedSet(dir)
	if _, ok := set["skip-me.pdf"]; !ok {
		t.Fatalf("skipped set = %v, want skip-me.pdf", set)
	}
}

func TestUndoEntriesSkipsMissingMovedFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := LoadFrom(path)
	dir := t.TempDir()
	if err := s.RecordAssign(dir, sorter.UndoEntry{
		OriginalPath: filepath.Join(dir, "a.pdf"),
		MovedTo:      filepath.Join(dir, "Work", "gone.pdf"),
		Category:     "Work",
	}); err != nil {
		t.Fatal(err)
	}
	if n := len(s.UndoEntriesStillOnDisk(dir)); n != 0 {
		t.Errorf("missing moved_to should not restore undo, got %d", n)
	}
}

func TestSeparateDirectoriesAreIsolated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	s := LoadFrom(path)
	a := t.TempDir()
	b := t.TempDir()
	_ = s.RecordSkip(a, "only-a.pdf")
	_ = s.RecordSkip(b, "only-b.pdf")
	if _, ok := s.SkippedSet(a)["only-b.pdf"]; ok {
		t.Error("skip from B leaked into A")
	}
	if _, ok := s.SkippedSet(b)["only-a.pdf"]; ok {
		t.Error("skip from A leaked into B")
	}
}
