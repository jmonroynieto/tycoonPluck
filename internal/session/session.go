// Package session keeps a small JSON journal of classifications for each
// source directory the user works on during this app session (and until the
// OS clears the temp/cache file).
//
// The folder picker only lists directories by design — you open a folder of
// PDFs, not individual files. This journal records what you did *inside*
// that folder (assigns / skips) so reopen/resume stays coherent.
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"tycoonPluck/internal/sorter"
)

// Classification is one successful assign for a PDF under a source folder.
type Classification struct {
	Name         string `json:"name"`          // basename at assign time
	OriginalPath string `json:"original_path"` // full path before move
	MovedTo      string `json:"moved_to"`
	Category     string `json:"category"`
	At           string `json:"at"` // RFC3339
}

// DirLog is the journal for one absolute source directory.
type DirLog struct {
	SourceDir       string           `json:"source_dir"`
	Classifications []Classification `json:"classifications"`
	Skipped         []string         `json:"skipped"` // basenames skipped this session
}

// Store is the on-disk file: map of abs source dir → DirLog.
type Store struct {
	mu   sync.Mutex
	path string
	// Directories keyed by filepath.Clean(abs path).
	Directories map[string]*DirLog `json:"directories"`
}

// Path returns the session journal path under the user cache (tmp-ish, not config).
func Path() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "tycoonpluck-session.json")
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "tycoonPluck", "session.json")
}

// Load reads the journal from disk (or returns an empty store).
func Load() *Store {
	return LoadFrom(Path())
}

// LoadFrom is like Load but uses an explicit path (tests).
func LoadFrom(path string) *Store {
	s := &Store{
		path:        path,
		Directories: make(map[string]*DirLog),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var raw struct {
		Directories map[string]*DirLog `json:"directories"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || raw.Directories == nil {
		return s
	}
	s.Directories = raw.Directories
	return s
}

// Save writes the journal atomically-ish (write then rename not required for v0).
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		s.path = Path()
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	payload := struct {
		Directories map[string]*DirLog `json:"directories"`
	}{Directories: s.Directories}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(s.path, data, 0o644)
}

func key(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return filepath.Clean(dir)
	}
	return filepath.Clean(abs)
}

// For returns (creating if needed) the log for sourceDir.
func (s *Store) For(sourceDir string) *DirLog {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(sourceDir)
	if d, ok := s.Directories[k]; ok && d != nil {
		return d
	}
	d := &DirLog{SourceDir: k}
	if s.Directories == nil {
		s.Directories = make(map[string]*DirLog)
	}
	s.Directories[k] = d
	return d
}

// RecordAssign appends a classification and persists.
func (s *Store) RecordAssign(sourceDir string, entry sorter.UndoEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(sourceDir)
	d := s.Directories[k]
	if d == nil {
		d = &DirLog{SourceDir: k}
		if s.Directories == nil {
			s.Directories = make(map[string]*DirLog)
		}
		s.Directories[k] = d
	}
	d.Classifications = append(d.Classifications, Classification{
		Name:         filepath.Base(entry.OriginalPath),
		OriginalPath: entry.OriginalPath,
		MovedTo:      entry.MovedTo,
		Category:     entry.Category,
		At:           time.Now().UTC().Format(time.RFC3339),
	})
	return s.saveLocked()
}

// RecordSkip records a basename as skipped for this source dir and persists.
func (s *Store) RecordSkip(sourceDir, basename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(sourceDir)
	d := s.Directories[k]
	if d == nil {
		d = &DirLog{SourceDir: k}
		if s.Directories == nil {
			s.Directories = make(map[string]*DirLog)
		}
		s.Directories[k] = d
	}
	base := filepath.Base(basename)
	for _, existing := range d.Skipped {
		if existing == base {
			return s.saveLocked()
		}
	}
	d.Skipped = append(d.Skipped, base)
	return s.saveLocked()
}

// PopLastAssign removes the last classification for sourceDir (after successful undo).
func (s *Store) PopLastAssign(sourceDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(sourceDir)
	d := s.Directories[k]
	if d == nil || len(d.Classifications) == 0 {
		return nil
	}
	d.Classifications = d.Classifications[:len(d.Classifications)-1]
	return s.saveLocked()
}

// SkippedSet returns basenames skipped for sourceDir.
func (s *Store) SkippedSet(sourceDir string) map[string]struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]struct{})
	d := s.Directories[key(sourceDir)]
	if d == nil {
		return out
	}
	for _, name := range d.Skipped {
		out[name] = struct{}{}
	}
	return out
}

// UndoEntriesStillOnDisk returns sorter undo entries for classifications whose
// MovedTo still exists (oldest first — same order as Assign).
func (s *Store) UndoEntriesStillOnDisk(sourceDir string) []sorter.UndoEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.Directories[key(sourceDir)]
	if d == nil {
		return nil
	}
	var out []sorter.UndoEntry
	for _, c := range d.Classifications {
		info, err := os.Stat(c.MovedTo)
		if err != nil || info.IsDir() {
			continue
		}
		out = append(out, sorter.UndoEntry{
			OriginalPath: c.OriginalPath,
			MovedTo:      c.MovedTo,
			Category:     c.Category,
		})
	}
	return out
}
