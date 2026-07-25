package categories

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// --- Sanitize ---

func TestSanitizeTrimsWhitespace(t *testing.T) {
	if got := Sanitize("  Work  "); got != "Work" {
		t.Errorf("got %q, want \"Work\"", got)
	}
}

func TestSanitizeRejectsEmptyOrBlank(t *testing.T) {
	for _, in := range []string{"", "   "} {
		if got := Sanitize(in); got != "" {
			t.Errorf("Sanitize(%q) = %q, want \"\"", in, got)
		}
	}
}

func TestSanitizeRejectsDotAndDotDot(t *testing.T) {
	for _, in := range []string{".", ".."} {
		if got := Sanitize(in); got != "" {
			t.Errorf("Sanitize(%q) = %q, want \"\"", in, got)
		}
	}
}

func TestSanitizeRejectsSeparatorsAndTraversal(t *testing.T) {
	cases := []string{"a/b", `a\b`, "../etc", "../../etc/passwd", "/etc/passwd"}
	for _, in := range cases {
		if got := Sanitize(in); got != "" {
			t.Errorf("Sanitize(%q) = %q, want \"\"", in, got)
		}
	}
}

func TestSanitizeRejectsColonAndNullByte(t *testing.T) {
	for _, in := range []string{"a:b", "Work\x00"} {
		if got := Sanitize(in); got != "" {
			t.Errorf("Sanitize(%q) = %q, want \"\"", in, got)
		}
	}
}

func TestSanitizeAcceptsOrdinaryName(t *testing.T) {
	if got := Sanitize("Taxes 2025"); got != "Taxes 2025" {
		t.Errorf("got %q, want \"Taxes 2025\"", got)
	}
}

// --- ConfigPath ---

func TestConfigPathUsesXDGConfigHomeWhenSet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdgtest")
	want := filepath.Join("/tmp/xdgtest", "tycoonPluck", "categories.json")
	if got := ConfigPath(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestConfigPathFallsBackToHomeDotConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir available in this environment")
	}
	want := filepath.Join(home, ".config", "tycoonPluck", "categories.json")
	if got := ConfigPath(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- Load / Save round trip ---

func withTempConfigHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestLoadReturnsEmptyWhenNoConfigFile(t *testing.T) {
	withTempConfigHome(t)
	if got := Load(); len(got) != 0 {
		t.Errorf("got %v, want empty (no preloaded categories)", got)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	withTempConfigHome(t)
	want := []string{"Work", "Taxes", "Personal"}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	if got := Load(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSaveCreatesParentDirectories(t *testing.T) {
	withTempConfigHome(t)
	if err := Save([]string{"Work"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ConfigPath()); err != nil {
		t.Errorf("config file missing after Save: %v", err)
	}
}

func TestSaveDeduplicatesCaseInsensitivelyKeepingFirst(t *testing.T) {
	withTempConfigHome(t)
	if err := Save([]string{"Work", "work", "WORK", "Personal"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"Work", "Personal"}
	if got := Load(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSaveDropsInvalidNames(t *testing.T) {
	withTempConfigHome(t)
	if err := Save([]string{"Work", "../etc", "  ", "Personal"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"Work", "Personal"}
	if got := Load(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLoadReturnsEmptyOnCorruptJSON(t *testing.T) {
	withTempConfigHome(t)
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(); len(got) != 0 {
		t.Errorf("got %v, want empty on corrupt JSON", got)
	}
}

func TestLoadReturnsEmptyWhenJSONIsNotAList(t *testing.T) {
	withTempConfigHome(t)
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]int{"a": 1})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(); len(got) != 0 {
		t.Errorf("got %v, want empty when JSON is not a list", got)
	}
}

// Regression test: a raw []string unmarshal target would error out on any
// non-string element and silently discard the whole (otherwise valid) list.
// Load must instead skip just the bad entries, matching the Python sibling
// app's behaviour.
func TestLoadSkipsNonStringEntriesInsteadOfDiscardingWholeList(t *testing.T) {
	withTempConfigHome(t)
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal([]any{"Work", 42, nil, "Personal"})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	want := []string{"Work", "Personal"}
	if got := Load(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLoadReturnsEmptyWhenAllEntriesInvalid(t *testing.T) {
	withTempConfigHome(t)
	if err := Save([]string{"../etc", "   ", "."}); err != nil {
		t.Fatal(err)
	}
	if got := Load(); len(got) != 0 {
		t.Errorf("got %v, want empty when all entries invalid", got)
	}
}

func TestSaveWritesPrettyPrintedJSONWithTrailingNewline(t *testing.T) {
	withTempConfigHome(t)
	if err := Save([]string{"Work"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.HasSuffix(text, "\n") {
		t.Error("expected trailing newline")
	}
	var decoded []string
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}
	if !reflect.DeepEqual(decoded, []string{"Work"}) {
		t.Errorf("got %v, want [Work]", decoded)
	}
}
