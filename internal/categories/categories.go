// Package categories loads and saves the user category list.
package categories

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ConfigPath returns ~/.config/tycoonPluck/categories.json (or $XDG_CONFIG_HOME).
func ConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "tycoonPluck", "categories.json")
}

// Sanitize returns a cleaned category name, or "" if invalid.
func Sanitize(name string) string {
	cleaned := strings.TrimSpace(name)
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return ""
	}
	if strings.ContainsAny(cleaned, `/\:`+"\x00") {
		return ""
	}
	if filepath.Base(cleaned) != cleaned {
		return ""
	}
	return cleaned
}

// Load reads categories from disk.
// First run (no config file), corrupt JSON, or empty list → empty slice.
// Categories are never pre-seeded; the user adds them with "+ Add".
func Load() []string {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw []any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	names := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			names = append(names, s)
		}
	}
	return dedupe(names)
}

// Save writes categories to the config path (may be an empty list).
func Save(list []string) error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	clean := dedupe(list)
	// Always write a JSON array (including [] ) so Load can distinguish
	// "user cleared all" from "no file yet" the same way: both yield empty.
	data, err := json.MarshalIndent(clean, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func dedupe(list []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, item := range list {
		c := Sanitize(item)
		if c == "" {
			continue
		}
		key := strings.ToLower(c)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, c)
	}
	return out
}
