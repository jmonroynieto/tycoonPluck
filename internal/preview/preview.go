// Package preview renders pages of a PDF to images via pdftoppm.
package preview

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// RenderFirstPage draws page 1 of pdfPath as a PNG-backed image.
// Requires the `pdftoppm` binary from Poppler (pacman: poppler).
// dpi controls raster resolution (e.g. 120).
func RenderFirstPage(pdfPath string, dpi int) (image.Image, error) {
	return RenderFirstPageContext(context.Background(), pdfPath, dpi)
}

// RenderFirstPageContext is RenderFirstPage with cancellation: ctx cancel
// kills the pdftoppm subprocess (via exec.CommandContext).
func RenderFirstPageContext(ctx context.Context, pdfPath string, dpi int) (image.Image, error) {
	pages, err := RenderPagesContext(ctx, pdfPath, dpi, 1)
	if err != nil {
		return nil, err
	}
	return pages[0], nil
}

// RenderPages draws up to maxPages pages of pdfPath, starting at page 1, as
// PNG-backed images in page order. Returns fewer than maxPages images if the
// document has fewer pages — pdftoppm handles an out-of-range page request
// gracefully (no error, just whatever pages actually exist).
// Requires the `pdftoppm` binary from Poppler (pacman: poppler).
func RenderPages(pdfPath string, dpi int, maxPages int) ([]image.Image, error) {
	return RenderPagesContext(context.Background(), pdfPath, dpi, maxPages)
}

// RenderPagesContext is RenderPages with cancellation support.
func RenderPagesContext(ctx context.Context, pdfPath string, dpi int, maxPages int) ([]image.Image, error) {
	if dpi <= 0 {
		dpi = 120
	}
	if maxPages <= 0 {
		maxPages = 1
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := os.Stat(pdfPath); err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return nil, fmt.Errorf("pdftoppm not found — install Poppler (Manjaro: sudo pacman -S poppler)")
	}

	tmpDir, err := os.MkdirTemp("", "tycoonpluck-preview-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	// pdftoppm -png -f 1 -l N -r DPI INPUT OUTPREFIX
	// No -singlefile: that flag forces exactly one un-numbered output file,
	// which can't represent "however many pages exist, up to N". Without it,
	// pdftoppm numbers each page "OUTPREFIX-<n>.png".
	outPrefix := filepath.Join(tmpDir, "page")
	cmd := exec.CommandContext(
		ctx,
		"pdftoppm",
		"-png",
		"-f", "1",
		"-l", strconv.Itoa(maxPages),
		"-r", fmt.Sprintf("%d", dpi),
		pdfPath,
		outPrefix,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Prefer context error when the process was killed by cancel.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("pdftoppm: %s", msg)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	matches, err := filepath.Glob(outPrefix + "-*.png")
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("pdftoppm produced no output for %s", pdfPath)
	}
	// filepath.Glob's order isn't guaranteed to be numeric; sort explicitly.
	// (In practice maxPages is always small in this app, so the "-N" suffix
	// is always a single digit and lexical order would happen to agree — but
	// sorting by the parsed number is cheap and doesn't rely on that.)
	sort.Slice(matches, func(i, j int) bool {
		return pageNumber(matches[i]) < pageNumber(matches[j])
	})

	images := make([]image.Image, 0, len(matches))
	for _, m := range matches {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f, err := os.Open(m)
		if err != nil {
			return nil, err
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			return nil, err
		}
		images = append(images, img)
	}
	return images, nil
}

// pageNumber extracts the trailing "-N" page index from a pdftoppm output
// filename like ".../page-3.png". Returns 0 if unparsable, which just sorts
// that (shouldn't-happen) entry first rather than panicking on it.
func pageNumber(path string) int {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	dash := strings.LastIndex(base, "-")
	if dash < 0 {
		return 0
	}
	n, err := strconv.Atoi(base[dash+1:])
	if err != nil {
		return 0
	}
	return n
}
