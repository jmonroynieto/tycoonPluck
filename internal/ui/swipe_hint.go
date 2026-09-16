package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

// symbolText draws s with Fyne's bundled InterSymbols face.
// Noto Sans (the default UI font) has no ← → ↓ glyphs; using it as the
// primary face still emits a missing-glyph box even when a fallback paints
// the arrow.
func symbolText(s string, col color.Color, size float32) *canvas.Text {
	t := canvas.NewText(s, col)
	t.TextStyle = fyne.TextStyle{Symbol: true}
	t.TextSize = size
	return t
}

func captionText(s string, col color.Color, size float32) *canvas.Text {
	t := canvas.NewText(s, col)
	t.TextSize = size
	return t
}

// swipeActionHint is the always-visible swipe chrome: arrows from InterSymbols,
// words from Noto Sans, so the two faces never share a cluster.
func swipeActionHint(col color.Color) fyne.CanvasObject {
	size := theme.CaptionTextSize()
	return container.NewHBox(
		symbolText("←", col, size),
		captionText(" left", col, size),
		captionText("   ·   ", col, size),
		symbolText("↓", col, size),
		captionText(" skip", col, size),
		captionText("   ·   ", col, size),
		captionText("right ", col, size),
		symbolText("→", col, size),
	)
}
