// Package sorter implements the PDF triage queue: assign, skip, undo.
package sorter

import (
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	"tycoonPluck/internal/categories"
	"tycoonPluck/internal/formats"
)

// UndoEntry records one category assignment that can be reversed.
type UndoEntry struct {
	OriginalPath string
	MovedTo      string
	Category     string
}

// Sorter holds an in-memory session over one source directory.
type Sorter struct {
	SourceDir    string
	Queue        []string
	UndoStack    []UndoEntry
	TotalStarted int
	// Expanded includes images, documents, tables, and text files in the
	// queue (extension or MIME). PDFs are always eligible.
	Expanded bool
}

// OpenFolder loads top-level eligible files from dir (non-recursive).
func (s *Sorter) OpenFolder(dir string) (int, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("not a directory: %s", abs)
	}

	files, err := listEligible(abs, s.Expanded, nil)
	if err != nil {
		return 0, err
	}
	// Random order each open so alphabetical sequence doesn't prime users.
	rand.Shuffle(len(files), func(i, j int) {
		files[i], files[j] = files[j], files[i]
	})

	s.SourceDir = abs
	s.Queue = files
	s.UndoStack = nil
	s.TotalStarted = len(files)
	return s.TotalStarted, nil
}

// Rescan rebuilds the queue from SourceDir using the current Expanded flag.
// Files still eligible keep their order; newly eligible names are shuffled
// onto the tail. skip is a set of basenames (session skips) to leave out.
// Assign/skip progress (TotalStarted − remaining) is preserved.
func (s *Sorter) Rescan(skip map[string]struct{}) error {
	if s.SourceDir == "" {
		return nil
	}
	files, err := listEligible(s.SourceDir, s.Expanded, skip)
	if err != nil {
		return err
	}
	eligible := make(map[string]struct{}, len(files))
	for _, p := range files {
		eligible[p] = struct{}{}
	}
	handled := s.TotalStarted - len(s.Queue)
	if handled < 0 {
		handled = 0
	}
	kept := make([]string, 0, len(s.Queue))
	seen := make(map[string]struct{}, len(s.Queue))
	for _, p := range s.Queue {
		if _, ok := eligible[p]; !ok {
			continue
		}
		kept = append(kept, p)
		seen[p] = struct{}{}
	}
	var added []string
	for _, p := range files {
		if _, ok := seen[p]; ok {
			continue
		}
		added = append(added, p)
	}
	rand.Shuffle(len(added), func(i, j int) {
		added[i], added[j] = added[j], added[i]
	})
	s.Queue = append(kept, added...)
	s.TotalStarted = handled + len(s.Queue)
	return nil
}

func listEligible(dir string, expanded bool, skip map[string]struct{}) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if _, drop := skip[name]; drop {
			continue
		}
		path := filepath.Join(dir, name)
		if !formats.Accept(path, expanded) {
			continue
		}
		files = append(files, path)
	}
	return files, nil
}

// Current returns the path of the PDF being triaged, or "".
func (s *Sorter) Current() string {
	if len(s.Queue) == 0 {
		return ""
	}
	return s.Queue[0]
}

// Remaining is how many PDFs are still in the session queue.
func (s *Sorter) Remaining() int { return len(s.Queue) }

// HasFolder reports whether OpenFolder succeeded at least once.
func (s *Sorter) HasFolder() bool { return s.SourceDir != "" }

// IsDone is true when a folder is open and the queue is empty.
func (s *Sorter) IsDone() bool {
	return s.HasFolder() && len(s.Queue) == 0
}

// CanUndo reports whether Undo would do something.
func (s *Sorter) CanUndo() bool { return len(s.UndoStack) > 0 }

// DropBasenames removes queued files whose base name is in names (e.g. session skips).
// TotalStarted is reset to the remaining count so progress matches the active queue.
func (s *Sorter) DropBasenames(names map[string]struct{}) {
	if len(names) == 0 || len(s.Queue) == 0 {
		return
	}
	kept := s.Queue[:0]
	for _, p := range s.Queue {
		if _, drop := names[filepath.Base(p)]; drop {
			continue
		}
		kept = append(kept, p)
	}
	// If kept reuses Queue's array, copy to avoid later append stomping.
	s.Queue = append([]string(nil), kept...)
	s.TotalStarted = len(s.Queue)
}

// SetUndoStack replaces the undo stack (e.g. restore from session journal).
func (s *Sorter) SetUndoStack(stack []UndoEntry) {
	if len(stack) == 0 {
		s.UndoStack = nil
		return
	}
	s.UndoStack = append([]UndoEntry(nil), stack...)
}

// ProgressLabel returns a human string like "3 / 42".
func (s *Sorter) ProgressLabel() string {
	if !s.HasFolder() || s.TotalStarted == 0 {
		return "0 / 0"
	}
	if len(s.Queue) == 0 {
		return fmt.Sprintf("%d / %d", s.TotalStarted, s.TotalStarted)
	}
	handled := s.TotalStarted - len(s.Queue)
	return fmt.Sprintf("%d / %d", handled+1, s.TotalStarted)
}

// Peek returns up to n paths from the front of the queue (copy; never mutates).
// Used by swipe mode to fill a multi-card board.
func (s *Sorter) Peek(n int) []string {
	if n <= 0 || len(s.Queue) == 0 {
		return nil
	}
	if n > len(s.Queue) {
		n = len(s.Queue)
	}
	return append([]string(nil), s.Queue[:n]...)
}

// IndexOf returns the queue index of path, or -1 if not queued.
func (s *Sorter) IndexOf(path string) int {
	for i, p := range s.Queue {
		if p == path {
			return i
		}
	}
	return -1
}

// Assign moves the current PDF into sourceDir/category and advances.
func (s *Sorter) Assign(category string) (UndoEntry, error) {
	if len(s.Queue) == 0 {
		return UndoEntry{}, fmt.Errorf("no PDF left to assign")
	}
	return s.AssignPath(s.Queue[0], category)
}

// AssignPath moves the given queued PDF into sourceDir/category and removes it
// from the queue. The path must currently be in the queue (any position) so
// swipe-mode cards can triage out of order relative to the front.
func (s *Sorter) AssignPath(path, category string) (UndoEntry, error) {
	var zero UndoEntry
	if s.SourceDir == "" {
		return zero, fmt.Errorf("no folder open")
	}
	idx := s.IndexOf(path)
	if idx < 0 {
		return zero, fmt.Errorf("PDF not in queue: %s", filepath.Base(path))
	}

	// Same rules as categories.Sanitize (trim, no separators / : / null, no . / ..).
	category = categories.Sanitize(category)
	if category == "" {
		return zero, fmt.Errorf("invalid category name")
	}

	src := s.Queue[idx]
	destDir := filepath.Join(s.SourceDir, category)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return zero, err
	}
	dest, err := UniqueDest(filepath.Join(destDir, filepath.Base(src)))
	if err != nil {
		return zero, err
	}
	if err := moveFile(src, dest); err != nil {
		return zero, err
	}

	s.Queue = append(s.Queue[:idx], s.Queue[idx+1:]...)
	entry := UndoEntry{OriginalPath: src, MovedTo: dest, Category: category}
	s.UndoStack = append(s.UndoStack, entry)
	return entry, nil
}

// Skip drops the current file from the session queue without moving it.
func (s *Sorter) Skip() string {
	if len(s.Queue) == 0 {
		return ""
	}
	return s.SkipPath(s.Queue[0])
}

// SkipPath drops path from the session queue without moving it on disk.
// Returns the skipped path, or "" if it was not queued.
func (s *Sorter) SkipPath(path string) string {
	idx := s.IndexOf(path)
	if idx < 0 {
		return ""
	}
	skipped := s.Queue[idx]
	s.Queue = append(s.Queue[:idx], s.Queue[idx+1:]...)
	return skipped
}

// Undo reverses the last Assign and makes that file current again.
// The undo stack entry is only removed after the reverse move succeeds,
// so a failed restore can be retried.
func (s *Sorter) Undo() (string, error) {
	if len(s.UndoStack) == 0 || s.SourceDir == "" {
		return "", nil
	}
	entry := s.UndoStack[len(s.UndoStack)-1] // peek; pop only on success

	info, err := os.Stat(entry.MovedTo)
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("moved file missing: %s", entry.MovedTo)
	}

	target := entry.OriginalPath
	_, err = os.Stat(target)
	switch {
	case err == nil:
		// Name reclaimed — find a free sibling under the source dir.
		target, err = UniqueDest(filepath.Join(s.SourceDir, filepath.Base(entry.OriginalPath)))
		if err != nil {
			return "", err
		}
	case os.IsNotExist(err):
		// original path free — use it
	default:
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := moveFile(entry.MovedTo, target); err != nil {
		return "", err
	}

	s.UndoStack = s.UndoStack[:len(s.UndoStack)-1]
	s.Queue = append([]string{target}, s.Queue...)
	return target, nil
}

// UniqueDest returns path if free, or "name (1).ext", "name (2).ext", … if taken.
// Non-NotExist stat errors are returned rather than treated as collisions.
func UniqueDest(path string) (string, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		// exists — fall through to suffix search
	case os.IsNotExist(err):
		return path, nil
	default:
		return "", err
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for n := 1; ; n++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, n, ext))
		_, err := os.Stat(candidate)
		switch {
		case err == nil:
			continue
		case os.IsNotExist(err):
			return candidate, nil
		default:
			return "", err
		}
	}
}

func moveFile(src, dest string) error {
	if err := os.Rename(src, dest); err == nil {
		return nil
	}
	// Cross-device rename fails; copy then remove.
	if err := copyFile(src, dest); err != nil {
		return err
	}
	if err := os.Remove(src); err != nil {
		// Roll back the orphan copy so Assign failure leaves no duplicate.
		_ = os.Remove(dest)
		return err
	}
	return nil
}

func copyFile(src, dest string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Preserve source permission bits (user correction).
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode().Perm())
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return nil
}
