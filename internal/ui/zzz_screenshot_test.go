package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
)

func touchScreenshot(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeMultiPageScreenshotPDF hand-builds a valid-enough n-page PDF (same
// technique as internal/preview's test fixtures) so the multi-page pager has
// something real to render for the screenshot.
func writeMultiPageScreenshotPDF(t *testing.T, dir, name string, pages int) string {
	t.Helper()
	var kids, objs strings.Builder
	for i := range pages {
		objNum := 3 + i
		if i > 0 {
			kids.WriteString(" ")
		}
		fmt.Fprintf(&kids, "%d 0 R", objNum)
		fmt.Fprintf(&objs, "%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj\n", objNum)
	}
	content := fmt.Sprintf(`%%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [%s] /Count %d >>
endobj
%strailer
<< /Size %d /Root 1 0 R >>
%%%%EOF
`, kids.String(), pages, objs.String(), 3+pages)

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func screenshotDir(t *testing.T) string {
	t.Helper()
	// Optional: export screenshots for visual review.
	if d := os.Getenv("TYCOONPLUCK_SCREENSHOT_DIR"); d != "" {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}
	return t.TempDir()
}

func capture(t *testing.T, dir, name string, ui *App) {
	t.Helper()
	c, ok := ui.win.Canvas().(interface{ Capture() image.Image })
	if !ok {
		t.Fatalf("canvas does not support Capture()")
	}
	img := c.Capture()
	out, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := png.Encode(out, img); err != nil {
		t.Fatal(err)
	}
}

func TestScreenshotForReview(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	outDir := screenshotDir(t)

	// Empty state (no preloaded categories).
	a1 := test.NewTempApp(t)
	ui1 := New(a1)
	capture(t, outDir, "shot-1-empty.png", ui1)

	// Loaded folder, current file selected, category list visible.
	a2 := test.NewTempApp(t)
	ui2 := New(a2)
	ui2.cats = []string{"Work", "Personal", "Taxes"}
	ui2.rebuildCategoryButtons()
	dir := t.TempDir()
	touchScreenshot(t, filepath.Join(dir, "Invoice-2026-Q1.pdf"))
	touchScreenshot(t, filepath.Join(dir, "Contract-Draft.pdf"))
	touchScreenshot(t, filepath.Join(dir, "Receipt.pdf"))
	if _, err := ui2.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui2.refresh()
	capture(t, outDir, "shot-2-loaded.png", ui2)

	// After one assignment (status + progress + undo enabled).
	if _, err := ui2.sorter.Assign(ui2.cats[0]); err != nil {
		t.Fatal(err)
	}
	ui2.setStatus("Moved → " + ui2.cats[0] + "/Invoice-2026-Q1.pdf")
	ui2.refresh()
	capture(t, outDir, "shot-3-assigned.png", ui2)

	// Queue fully drained (celebratory empty state).
	for ui2.sorter.Current() != "" {
		if _, err := ui2.sorter.Assign(ui2.cats[0]); err != nil {
			t.Fatal(err)
		}
	}
	ui2.refresh()
	capture(t, outDir, "shot-4-done.png", ui2)

	// Multi-page pager (only if pdftoppm is actually available — otherwise
	// this would just show the "cannot preview" error state again).
	if _, err := exec.LookPath("pdftoppm"); err == nil {
		a3 := test.NewTempApp(t)
		ui3 := New(a3)
		ui3.cats = []string{"Work", "Personal", "Taxes"}
		ui3.rebuildCategoryButtons()
		dir3 := t.TempDir()
		writeMultiPageScreenshotPDF(t, dir3, "Report-3-Pages.pdf", 3)
		if _, err := ui3.sorter.OpenFolder(dir3); err != nil {
			t.Fatal(err)
		}
		ui3.refresh()
		ui3.waitForPreview()
		capture(t, outDir, "shot-5-multipage-p1.png", ui3)

		ui3.showPage(1)
		capture(t, outDir, "shot-5-multipage-p2.png", ui3)
	}
}
