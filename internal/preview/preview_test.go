package preview

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A hand-built, valid-enough single-page PDF that Poppler can rasterize.
// No embedded xref table — Poppler reconstructs it, same as many
// real-world "slightly broken" PDFs users will actually feed this app.
const minimalPDF = `%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>
endobj
trailer
<< /Size 4 /Root 1 0 R >>
%%EOF
`

// writeMultiPagePDF hand-builds a valid-enough n-page PDF (same style as
// minimalPDF: no xref table, Poppler reconstructs it).
func writeMultiPagePDF(t *testing.T, dir, name string, pages int) string {
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

func requirePdftoppm(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed; skipping preview render tests")
	}
}

func writeMinimalPDF(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(minimalPDF), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRenderFirstPageValidPDF(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMinimalPDF(t, dir, "a.pdf")

	img, err := RenderFirstPage(path, 72)
	if err != nil {
		t.Fatal(err)
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		t.Errorf("got empty image bounds %v", bounds)
	}
}

func TestRenderFirstPageHigherDPIProducesLargerImage(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMinimalPDF(t, dir, "a.pdf")

	small, err := RenderFirstPage(path, 72)
	if err != nil {
		t.Fatal(err)
	}
	large, err := RenderFirstPage(path, 300)
	if err != nil {
		t.Fatal(err)
	}
	if large.Bounds().Dx() <= small.Bounds().Dx() {
		t.Errorf("expected higher DPI to produce a wider image: %d vs %d",
			large.Bounds().Dx(), small.Bounds().Dx())
	}
}

func TestRenderFirstPageZeroDPIDefaultsTo120(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMinimalPDF(t, dir, "a.pdf")

	viaZero, err := RenderFirstPage(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	viaDefault, err := RenderFirstPage(path, 120)
	if err != nil {
		t.Fatal(err)
	}
	if viaZero.Bounds() != viaDefault.Bounds() {
		t.Errorf("dpi=0 bounds %v should match explicit dpi=120 bounds %v",
			viaZero.Bounds(), viaDefault.Bounds())
	}
}

func TestRenderFirstPageMissingFileErrors(t *testing.T) {
	requirePdftoppm(t)
	_, err := RenderFirstPage(filepath.Join(t.TempDir(), "nope.pdf"), 120)
	if err == nil {
		t.Fatal("expected error for a nonexistent file")
	}
}

func TestRenderFirstPageCorruptPDFErrors(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "garbage.pdf")
	if err := os.WriteFile(path, []byte("not a pdf at all"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := RenderFirstPage(path, 120)
	if err == nil {
		t.Fatal("expected error for a non-PDF file")
	}
}

func TestRenderFirstPageMissingPdftoppmBinaryErrors(t *testing.T) {
	dir := t.TempDir()
	path := writeMinimalPDF(t, dir, "a.pdf")

	t.Setenv("PATH", "")
	_, err := RenderFirstPage(path, 120)
	if err == nil {
		t.Fatal("expected error when pdftoppm is not on PATH")
	}
}

func TestRenderFirstPageDoesNotLeakTempFiles(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMinimalPDF(t, dir, "a.pdf")

	before, err := filepath.Glob(filepath.Join(os.TempDir(), "tycoonpluck-preview-*"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RenderFirstPage(path, 72); err != nil {
		t.Fatal(err)
	}
	after, err := filepath.Glob(filepath.Join(os.TempDir(), "tycoonpluck-preview-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(after) > len(before) {
		t.Errorf("temp render dirs leaked: before=%d after=%d", len(before), len(after))
	}
}

// --- RenderPages ---

func TestRenderPagesSinglePageDoc(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMinimalPDF(t, dir, "a.pdf")

	pages, err := RenderPages(path, 72, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
}

func TestRenderPagesCapsAtMaxPages(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMultiPagePDF(t, dir, "four.pdf", 4)

	pages, err := RenderPages(path, 72, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3 (capped)", len(pages))
	}
}

func TestRenderPagesFewerThanMaxIsNotAnError(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMultiPagePDF(t, dir, "two.pdf", 2)

	pages, err := RenderPages(path, 72, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2 (document only has 2)", len(pages))
	}
}

func TestRenderPagesAllImagesHaveValidBounds(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMultiPagePDF(t, dir, "three.pdf", 3)

	pages, err := RenderPages(path, 72, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
	for i, img := range pages {
		b := img.Bounds()
		if b.Dx() <= 0 || b.Dy() <= 0 {
			t.Errorf("page %d has empty bounds %v", i, b)
		}
	}
}

func TestRenderPagesZeroOrNegativeMaxPagesDefaultsToOne(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMultiPagePDF(t, dir, "four.pdf", 4)

	for _, maxPages := range []int{0, -1} {
		pages, err := RenderPages(path, 72, maxPages)
		if err != nil {
			t.Fatalf("maxPages=%d: %v", maxPages, err)
		}
		if len(pages) != 1 {
			t.Errorf("maxPages=%d: got %d pages, want 1", maxPages, len(pages))
		}
	}
}

func TestRenderPagesMissingFileErrors(t *testing.T) {
	requirePdftoppm(t)
	_, err := RenderPages(filepath.Join(t.TempDir(), "nope.pdf"), 120, 3)
	if err == nil {
		t.Fatal("expected error for a nonexistent file")
	}
}

func TestRenderPagesCorruptPDFErrors(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "garbage.pdf")
	if err := os.WriteFile(path, []byte("not a pdf at all"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := RenderPages(path, 120, 3)
	if err == nil {
		t.Fatal("expected error for a non-PDF file")
	}
}

func TestRenderPagesImageDoesNotNeedPdftoppm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dot.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	pages, err := RenderPages(path, 120, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Bounds().Dx() != 1 || pages[0].Bounds().Dy() != 1 {
		t.Fatalf("png preview = %d pages, bounds %v", len(pages), pages[0].Bounds())
	}
}

func TestRenderPagesTextHasNoPreview(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := RenderPages(path, 120, 1)
	if err == nil {
		t.Fatal("expected no preview for a text file")
	}
}

func TestRenderPagesMissingPdftoppmBinaryErrors(t *testing.T) {
	dir := t.TempDir()
	path := writeMinimalPDF(t, dir, "a.pdf")

	t.Setenv("PATH", "")
	_, err := RenderPages(path, 120, 3)
	if err == nil {
		t.Fatal("expected error when pdftoppm is not on PATH")
	}
}
