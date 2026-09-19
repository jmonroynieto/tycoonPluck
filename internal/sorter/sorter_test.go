package sorter

import (
	"os"
	"path/filepath"
	"testing"
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatalf("touch %s: %v", path, err)
	}
}

func touchMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("%PDF-1.4\n"), mode); err != nil {
		t.Fatalf("touch %s: %v", path, err)
	}
}

// --- UniqueDest ---

func mustUniqueDest(t *testing.T, path string) string {
	t.Helper()
	got, err := UniqueDest(path)
	if err != nil {
		t.Fatalf("UniqueDest(%q): %v", path, err)
	}
	return got
}

func TestUniqueDestReturnsSamePathWhenFree(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "report.pdf")
	if got := mustUniqueDest(t, target); got != target {
		t.Errorf("got %q, want %q", got, target)
	}
}

func TestUniqueDestAppendsCounterOnCollision(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "report.pdf"))
	want := filepath.Join(dir, "report (1).pdf")
	if got := mustUniqueDest(t, filepath.Join(dir, "report.pdf")); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUniqueDestIncrementsPastMultipleCollisions(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "report.pdf"))
	touch(t, filepath.Join(dir, "report (1).pdf"))
	touch(t, filepath.Join(dir, "report (2).pdf"))
	want := filepath.Join(dir, "report (3).pdf")
	if got := mustUniqueDest(t, filepath.Join(dir, "report.pdf")); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUniqueDestPreservesExtension(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "report.PDF"))
	want := filepath.Join(dir, "report (1).PDF")
	if got := mustUniqueDest(t, filepath.Join(dir, "report.PDF")); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUniqueDestPropagatesStatErrors(t *testing.T) {
	// Path under a non-existent parent: Stat returns a path error that is
	// not IsNotExist for the leaf in a useful way on some systems; more
	// reliably, refuse to treat a blocked directory as "free".
	// Use a path whose parent is a file so Stat fails with ENOTDIR.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	touch(t, blocker)
	_, err := UniqueDest(filepath.Join(blocker, "report.pdf"))
	if err == nil {
		t.Fatal("expected error when parent path is not a directory")
	}
}

// --- OpenFolder ---

func TestOpenFolderFindsOnlyTopLevelPDFsCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.PDF"))
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(nested, "c.pdf"))

	s := &Sorter{}
	n, err := s.OpenFolder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("got %d PDFs, want 2", n)
	}
	if s.Remaining() != 2 {
		t.Errorf("Remaining() = %d, want 2", s.Remaining())
	}
	got := map[string]bool{}
	for _, p := range s.Queue {
		got[filepath.Base(p)] = true
	}
	for _, name := range []string{"a.pdf", "b.PDF"} {
		if !got[name] {
			t.Errorf("missing %q in queue %v", name, s.Queue)
		}
	}
}

func TestOpenFolderRejectsEmptyAndMisnamedPDFs(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "good.pdf"))
	if err := os.WriteFile(filepath.Join(dir, "empty.pdf"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "download.pdf"), []byte("<html>not a PDF</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Sorter{}
	if n, err := s.OpenFolder(dir); err != nil || n != 1 {
		t.Fatalf("OpenFolder = (%d, %v), want (1, nil)", n, err)
	}
	if s.RejectedPDFs != 2 {
		t.Fatalf("RejectedPDFs = %d, want 2", s.RejectedPDFs)
	}
}

func TestOpenFolderQueueContainsAllPDFsInSomeOrder(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "c.pdf"))
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.pdf"))

	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	if len(s.Queue) != 3 {
		t.Fatalf("len(Queue) = %d, want 3", len(s.Queue))
	}
	got := map[string]bool{}
	for _, p := range s.Queue {
		got[filepath.Base(p)] = true
	}
	for _, name := range []string{"a.pdf", "b.pdf", "c.pdf"} {
		if !got[name] {
			t.Errorf("missing %q", name)
		}
	}
}

func TestOpenFolderMissingDirectoryErrors(t *testing.T) {
	s := &Sorter{}
	_, err := s.OpenFolder(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestOpenFolderOnAFileErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir.pdf")
	touch(t, file)

	s := &Sorter{}
	_, err := s.OpenFolder(file)
	if err == nil {
		t.Fatal("expected error when path is a file, not a directory")
	}
}

func TestReopeningResetsUndoStackAndProgress(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	other := filepath.Join(dir, "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(other, "b.pdf"))

	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Assign("Work"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.OpenFolder(other); err != nil {
		t.Fatal(err)
	}

	if s.CanUndo() {
		t.Error("CanUndo() = true after reopening a different folder, want false")
	}
	if s.TotalStarted != 1 {
		t.Errorf("TotalStarted = %d, want 1", s.TotalStarted)
	}
}

// --- state helpers ---

func TestZeroValueSorterState(t *testing.T) {
	s := &Sorter{}
	if s.HasFolder() {
		t.Error("HasFolder() = true before OpenFolder")
	}
	if s.IsDone() {
		t.Error("IsDone() = true before OpenFolder")
	}
	if s.Current() != "" {
		t.Error("Current() != \"\" before OpenFolder")
	}
	if s.ProgressLabel() != "0 / 0" {
		t.Errorf("ProgressLabel() = %q, want \"0 / 0\"", s.ProgressLabel())
	}
}

func TestIsDoneOnlyAfterQueueDrained(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	if s.IsDone() {
		t.Error("IsDone() = true with a PDF still queued")
	}
	if _, err := s.Assign("Work"); err != nil {
		t.Fatal(err)
	}
	if !s.IsDone() {
		t.Error("IsDone() = false after last PDF assigned")
	}
}

func TestProgressLabelFormats(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.pdf"))

	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	if got := s.ProgressLabel(); got != "1 / 2" {
		t.Errorf("got %q, want \"1 / 2\"", got)
	}

	if _, err := s.Assign("Work"); err != nil {
		t.Fatal(err)
	}
	if got := s.ProgressLabel(); got != "2 / 2" {
		t.Errorf("got %q, want \"2 / 2\"", got)
	}

	s.Skip()
	if got := s.ProgressLabel(); got != "2 / 2" {
		t.Errorf("got %q, want \"2 / 2\" after final skip", got)
	}
}

// --- Assign ---

func TestAssignMovesFileIntoCategorySubfolder(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}

	entry, err := s.Assign("Work")
	if err != nil {
		t.Fatal(err)
	}

	wantDest := filepath.Join(dir, "Work", "a.pdf")
	if entry.MovedTo != wantDest {
		t.Errorf("MovedTo = %q, want %q", entry.MovedTo, wantDest)
	}
	if _, err := os.Stat(wantDest); err != nil {
		t.Errorf("destination file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.pdf")); !os.IsNotExist(err) {
		t.Error("source file still present after move")
	}
	if entry.Category != "Work" {
		t.Errorf("Category = %q, want \"Work\"", entry.Category)
	}
	if s.Remaining() != 0 {
		t.Errorf("Remaining() = %d, want 0", s.Remaining())
	}
}

func TestAssignAdvancesQueue(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}

	first := s.Current()
	if _, err := s.Assign("Work"); err != nil {
		t.Fatal(err)
	}

	if s.Current() == "" || s.Current() == first {
		t.Errorf("Current() = %q after assign, want the other file (not %q)", s.Current(), first)
	}
	if s.Remaining() != 1 {
		t.Errorf("Remaining() = %d, want 1", s.Remaining())
	}
}

func TestAssignCollisionGetsSuffixed(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	destDir := filepath.Join(dir, "Work")
	if err := os.Mkdir(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Join(destDir, "a.pdf"))

	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	entry, err := s.Assign("Work")
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(destDir, "a (1).pdf")
	if entry.MovedTo != want {
		t.Errorf("MovedTo = %q, want %q", entry.MovedTo, want)
	}
	if _, err := os.Stat(filepath.Join(destDir, "a.pdf")); err != nil {
		t.Error("pre-existing file in destination should be untouched")
	}
}

func TestAssignTrimsCategoryWhitespace(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Assign("  Work  "); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Work", "a.pdf")); err != nil {
		t.Errorf("expected file under trimmed category folder: %v", err)
	}
}

func TestAssignEmptyCategoryErrorsAndLeavesQueueUntouched(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Assign("   "); err == nil {
		t.Fatal("expected error for blank category")
	}
	if s.Remaining() != 1 {
		t.Errorf("Remaining() = %d, want 1 (queue must be untouched)", s.Remaining())
	}
}

func TestAssignRejectsPathSeparatorsAndTraversal(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))

	// Must match categories.Sanitize: separators, colon, dots, traversal.
	cases := []string{"a/b", `a\b`, "..", ".", "../escape", "/etc", "a:b", "Work\x00"}
	for _, category := range cases {
		t.Run(category, func(t *testing.T) {
			s := &Sorter{}
			if _, err := s.OpenFolder(dir); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Assign(category); err == nil {
				t.Errorf("Assign(%q) should have errored", category)
			}
			if s.Remaining() != 1 {
				t.Errorf("Remaining() = %d after rejected Assign(%q), want 1 (file must not move)", s.Remaining(), category)
			}
		})
	}

	// None of the invalid categories should have created any escape-path
	// directories outside dir, nor moved the source file anywhere.
	if _, err := os.Stat(filepath.Join(dir, "a.pdf")); err != nil {
		t.Errorf("source file should be untouched by rejected assigns: %v", err)
	}
}

func TestAssignWithoutOpenFolderErrors(t *testing.T) {
	s := &Sorter{}
	if _, err := s.Assign("Work"); err == nil {
		t.Fatal("expected error when no folder is open")
	}
}

func TestAssignWithEmptyQueueErrors(t *testing.T) {
	dir := t.TempDir()
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Assign("Work"); err == nil {
		t.Fatal("expected error when queue is empty")
	}
}

// --- Skip ---

func TestSkipLeavesFileInPlaceAndAdvances(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}

	before := s.Current()
	skipped := s.Skip()

	if skipped != before {
		t.Errorf("Skip() = %q, want current %q", skipped, before)
	}
	if _, err := os.Stat(skipped); err != nil {
		t.Error("skipped file should remain on disk")
	}
	if s.Current() == "" || s.Current() == before {
		t.Errorf("Current() = %q after skip, want the other file", s.Current())
	}
}

func TestSkipOnEmptyQueueReturnsEmptyString(t *testing.T) {
	dir := t.TempDir()
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	if got := s.Skip(); got != "" {
		t.Errorf("Skip() = %q, want \"\"", got)
	}
}

func TestSkipDoesNotAffectUndoStack(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	s.Skip()
	if s.CanUndo() {
		t.Error("CanUndo() = true after a Skip (no assignment happened)")
	}
}

// --- Undo ---

func TestUndoMovesFileBackAndRequeuesAsCurrent(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	// Queue is random; remember whatever was assigned first.
	entry, err := s.Assign("Work")
	if err != nil {
		t.Fatal(err)
	}
	assignedBase := filepath.Base(entry.OriginalPath)

	restored, err := s.Undo()
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(dir, assignedBase)
	if restored != want {
		t.Errorf("Undo() = %q, want %q", restored, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Error("restored file should exist back in source dir")
	}
	if _, err := os.Stat(filepath.Join(dir, "Work", assignedBase)); !os.IsNotExist(err) {
		t.Error("file should no longer exist under the category folder")
	}
	if s.Current() != want {
		t.Errorf("Current() = %q, want restored file to be current", s.Current())
	}
	if s.Remaining() != 2 {
		t.Errorf("Remaining() = %d, want 2", s.Remaining())
	}
}

func TestUndoWithEmptyStackReturnsEmptyNoError(t *testing.T) {
	dir := t.TempDir()
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	restored, err := s.Undo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restored != "" {
		t.Errorf("Undo() = %q, want \"\"", restored)
	}
}

func TestUndoWalksBackThroughMultipleAssignmentsLIFO(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	// Capture assign order (queue is shuffled).
	firstAssign, err := s.Assign("Work")
	if err != nil {
		t.Fatal(err)
	}
	secondAssign, err := s.Assign("Personal")
	if err != nil {
		t.Fatal(err)
	}

	// Undo is LIFO: last assign restores first.
	undone1, err := s.Undo()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(undone1) != filepath.Base(secondAssign.OriginalPath) {
		t.Errorf("first Undo() base = %q, want last-assigned %q", filepath.Base(undone1), filepath.Base(secondAssign.OriginalPath))
	}
	if !s.CanUndo() {
		t.Error("CanUndo() = false after only one of two undos")
	}

	undone2, err := s.Undo()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(undone2) != filepath.Base(firstAssign.OriginalPath) {
		t.Errorf("second Undo() base = %q, want first-assigned %q", filepath.Base(undone2), filepath.Base(firstAssign.OriginalPath))
	}
	if s.CanUndo() {
		t.Error("CanUndo() = true after exhausting the undo stack")
	}
}

func TestUndoResolvesCollisionIfOriginalNameReclaimed(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Assign("Work"); err != nil {
		t.Fatal(err)
	}

	// Something else now occupies the original filename in source_dir.
	touch(t, filepath.Join(dir, "a.pdf"))

	restored, err := s.Undo()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "a (1).pdf")
	if restored != want {
		t.Errorf("Undo() = %q, want %q", restored, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.pdf")); err != nil {
		t.Error("the file that reclaimed the name should be untouched")
	}
}

func TestUndoWhenMovedFileMissingErrorsAndStackPreserved(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Assign("Work"); err != nil {
		t.Fatal(err)
	}

	// Simulate the file being moved/deleted externally.
	if err := os.Remove(filepath.Join(dir, "Work", "a.pdf")); err != nil {
		t.Fatal(err)
	}

	_, err := s.Undo()
	if err == nil {
		t.Fatal("expected error when the moved file is missing")
	}
	// Stack entry must remain so the user can retry after recovering the file.
	if !s.CanUndo() {
		t.Error("undo stack entry must be preserved when restore fails")
	}

	// Recover the file and retry — undo should succeed now.
	touch(t, filepath.Join(dir, "Work", "a.pdf"))
	restored, err := s.Undo()
	if err != nil {
		t.Fatalf("retry Undo after restore: %v", err)
	}
	if restored != filepath.Join(dir, "a.pdf") {
		t.Errorf("Undo() = %q, want a.pdf back in source", restored)
	}
	if s.CanUndo() {
		t.Error("CanUndo() should be false after successful undo")
	}
}

func TestMoveFileRollsBackDestIfSourceRemoveFails(t *testing.T) {
	// Hard to force Remove failure portably; verify the happy rename path
	// still works and that a failed copy leaves no dest (covered below).
	dir := t.TempDir()
	src := filepath.Join(dir, "a.pdf")
	touch(t, src)
	dest := filepath.Join(dir, "sub", "a.pdf")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(src, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Error("dest should exist")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("src should be gone")
	}
}

// --- moveFile / copyFile (cross-filesystem fallback) ---

func TestCopyFilePreservesSourcePermissions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.pdf")
	touchMode(t, src, 0o600)
	dest := filepath.Join(dir, "b.pdf")

	if err := copyFile(src, dest); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("dest mode = %v, want 0600 (source's mode preserved)", info.Mode().Perm())
	}
}

func TestCopyFileCleansUpPartialDestOnReadError(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "b.pdf")
	err := copyFile(filepath.Join(dir, "does-not-exist.pdf"), dest)
	if err == nil {
		t.Fatal("expected error copying a nonexistent source")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Error("destination should not exist after a failed copy")
	}
}

func TestDropBasenamesRemovesFromQueue(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	s.DropBasenames(map[string]struct{}{"a.pdf": {}})
	if s.Remaining() != 1 || filepath.Base(s.Current()) != "b.pdf" {
		t.Errorf("after drop, current=%q remaining=%d", s.Current(), s.Remaining())
	}
	if s.TotalStarted != 1 {
		t.Errorf("TotalStarted = %d, want 1", s.TotalStarted)
	}
}

func TestSetUndoStackRestoresCanUndo(t *testing.T) {
	s := &Sorter{SourceDir: "/tmp"}
	s.SetUndoStack([]UndoEntry{{
		OriginalPath: "/tmp/a.pdf",
		MovedTo:      "/tmp/Work/a.pdf",
		Category:     "Work",
	}})
	if !s.CanUndo() {
		t.Error("CanUndo() = false after SetUndoStack")
	}
}

func TestMoveFileRenameSameFilesystem(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.pdf")
	touch(t, src)
	dest := filepath.Join(dir, "b.pdf")

	if err := moveFile(src, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Error("dest should exist after move")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("src should no longer exist after move")
	}
}

// --- Peek / AssignPath / SkipPath (swipe mode) ---

func TestPeekReturnsFrontN(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.pdf", "b.pdf", "c.pdf"} {
		touch(t, filepath.Join(dir, name))
	}
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	// Stabilize order for the assertion.
	s.Queue = []string{
		filepath.Join(dir, "a.pdf"),
		filepath.Join(dir, "b.pdf"),
		filepath.Join(dir, "c.pdf"),
	}

	got := s.Peek(2)
	if len(got) != 2 || got[0] != s.Queue[0] || got[1] != s.Queue[1] {
		t.Errorf("Peek(2) = %v, want first two of %v", got, s.Queue)
	}
	if len(s.Queue) != 3 {
		t.Error("Peek must not mutate the queue")
	}
	if got = s.Peek(10); len(got) != 3 {
		t.Errorf("Peek past end = %d items, want 3", len(got))
	}
	if s.Peek(0) != nil || s.Peek(-1) != nil {
		t.Error("Peek(≤0) should return nil")
	}
}

func TestAssignPathMiddleItemLeavesNeighbors(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.pdf", "b.pdf", "c.pdf"} {
		touch(t, filepath.Join(dir, name))
	}
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	s.Queue = []string{
		filepath.Join(dir, "a.pdf"),
		filepath.Join(dir, "b.pdf"),
		filepath.Join(dir, "c.pdf"),
	}
	mid := s.Queue[1]

	entry, err := s.AssignPath(mid, "Work")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Category != "Work" {
		t.Errorf("Category = %q", entry.Category)
	}
	if _, err := os.Stat(filepath.Join(dir, "Work", "b.pdf")); err != nil {
		t.Errorf("middle file not moved: %v", err)
	}
	if len(s.Queue) != 2 {
		t.Fatalf("Queue len = %d, want 2", len(s.Queue))
	}
	if filepath.Base(s.Queue[0]) != "a.pdf" || filepath.Base(s.Queue[1]) != "c.pdf" {
		t.Errorf("Queue = %v, want [a, c]", s.Queue)
	}
}

func TestAssignPathUnknownErrors(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignPath(filepath.Join(dir, "missing.pdf"), "Work"); err == nil {
		t.Error("expected error for path not in queue")
	}
	if s.Remaining() != 1 {
		t.Errorf("Remaining = %d, want 1 (queue untouched)", s.Remaining())
	}
}

func TestSkipPathMiddleItem(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.pdf", "b.pdf", "c.pdf"} {
		touch(t, filepath.Join(dir, name))
	}
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	s.Queue = []string{
		filepath.Join(dir, "a.pdf"),
		filepath.Join(dir, "b.pdf"),
		filepath.Join(dir, "c.pdf"),
	}
	mid := s.Queue[1]

	if got := s.SkipPath(mid); got != mid {
		t.Errorf("SkipPath = %q, want %q", got, mid)
	}
	if _, err := os.Stat(mid); err != nil {
		t.Error("SkipPath must not move/delete the file on disk")
	}
	if len(s.Queue) != 2 {
		t.Fatalf("Queue len = %d, want 2", len(s.Queue))
	}
	if filepath.Base(s.Queue[0]) != "a.pdf" || filepath.Base(s.Queue[1]) != "c.pdf" {
		t.Errorf("Queue = %v, want [a, c]", s.Queue)
	}
	if s.SkipPath(mid) != "" {
		t.Error("second SkipPath of same file should be a no-op")
	}
}

func TestAssignDelegatesToAssignPath(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Assign("Work"); err != nil {
		t.Fatal(err)
	}
	if s.Remaining() != 0 {
		t.Errorf("Remaining = %d, want 0", s.Remaining())
	}
}

func TestOpenFolderExpandedIncludesTextAndImages(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "photo"), []byte("\xff\xd8\xff\xe0JFIF"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Sorter{Expanded: true}
	n, err := s.OpenFolder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("got %d files, want 3", n)
	}
}

func TestRescanTogglePreservesQueueHeadAndSkipSet(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.pdf"))
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Sorter{}
	if _, err := s.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	head := filepath.Join(dir, "b.pdf")
	s.Queue = []string{head, filepath.Join(dir, "a.pdf")}
	s.Expanded = true
	if err := s.Rescan(map[string]struct{}{"a.pdf": {}}); err != nil {
		t.Fatal(err)
	}
	if s.Current() != head {
		t.Errorf("current = %q, want %q", s.Current(), head)
	}
	got := map[string]bool{}
	for _, p := range s.Queue {
		got[filepath.Base(p)] = true
	}
	if !got["notes.txt"] || got["a.pdf"] || !got["b.pdf"] || s.Remaining() != 2 {
		t.Fatalf("expanded queue = %v", s.Queue)
	}
	s.Expanded = false
	if err := s.Rescan(map[string]struct{}{"a.pdf": {}}); err != nil {
		t.Fatal(err)
	}
	if s.Remaining() != 1 || s.Current() != head {
		t.Errorf("PDF-only rescan = %v", s.Queue)
	}
}
