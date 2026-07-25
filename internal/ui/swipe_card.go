package ui

import (
	"image"
	"image/color"
	"math"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// swipeDir is the commit direction after a drag gesture on a card.
type swipeDir int

const (
	swipeNone swipeDir = iota
	swipeLeft
	swipeRight
	swipeDown
)

// Thresholds (logical Fyne units) before a drag commits on release.
const (
	swipeCommitX float32 = 72
	swipeCommitY float32 = 72
	// Preview feedback starts before the commit threshold so the user can
	// read the decision early.
	swipePreviewPx float32 = 18
)

// swipeSkipAccent is the neutral tone for ↓ skip (not a category assign).
var swipeSkipAccent = color.NRGBA{R: 0x9A, G: 0x9A, B: 0x94, A: 0xFF}

// swipeCard is one draggable PDF thumbnail on the swipe board.
// Drag left / right / down → assign left cat / assign right cat / skip.
//
// slot is the grid cell the card occupies (layout position). The board may
// reorder its paint list freely for z-order without moving this slot.
type swipeCard struct {
	widget.BaseWidget

	path    string
	title   string // full basename; nameLbl shows an ellipsized form
	slot    int
	board   *swipeBoard
	thumb   *canvas.Image
	nameLbl *canvas.Text
	bg      *canvas.Rectangle
	border  *canvas.Rectangle
	// Decision overlay: full-card wash + large center badge (icon + label).
	overlay  *canvas.Rectangle
	badgeBg  *canvas.Rectangle
	badge    *canvas.Text
	dragX    float32
	dragY    float32
	dragging bool
	// previewDir is the direction currently shown on the overlay (may be a
	// soft preview below the commit threshold).
	previewDir swipeDir
	// dragProgress 0..1 toward commit, drives overlay opacity.
	dragProgress float32
	onSwipe      func(path string, dir swipeDir)

	// snapBack animates dragX/dragY back to 0 after a drag ends without a
	// commit. Stopped and replaced whenever a new drag or rebind starts.
	snapBack *fyne.Animation

	// minSize is set by the board so every card shares cell geometry.
	minSize fyne.Size

	// nameCache / badgeCache avoid re-shaping text (MeasureText binary
	// search) on every drag frame — only recomputed when inputs change.
	nameCacheW     float32
	nameCacheText  string
	badgeCacheDir  swipeDir
	badgeCacheW    float32
	badgeCacheText string
}

func newSwipeCard(path string, onSwipe func(string, swipeDir)) *swipeCard {
	c := &swipeCard{
		path:        path,
		title:       filepath.Base(path),
		onSwipe:     onSwipe,
		minSize:     fyne.NewSize(140, 180),
		nameCacheW:  -1,
		badgeCacheW: -1,
	}

	c.bg = canvas.NewRectangle(color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})
	c.bg.CornerRadius = 12

	c.border = canvas.NewRectangle(color.Transparent)
	c.border.StrokeWidth = 4
	c.border.CornerRadius = 12

	c.thumb = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	c.thumb.FillMode = canvas.ImageFillContain
	c.thumb.SetMinSize(fyne.NewSize(100, 120))

	c.nameLbl = canvas.NewText(c.title, color.NRGBA{R: 0x1B, G: 0x1B, B: 0x22, A: 0xFF})
	c.nameLbl.TextSize = 11
	c.nameLbl.Alignment = fyne.TextAlignCenter
	c.nameLbl.TextStyle = fyne.TextStyle{Bold: true}

	c.overlay = canvas.NewRectangle(color.Transparent)
	c.overlay.CornerRadius = 12
	c.overlay.Hide()

	c.badgeBg = canvas.NewRectangle(color.Transparent)
	c.badgeBg.CornerRadius = 999
	c.badgeBg.Hide()

	c.badge = canvas.NewText("", color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})
	c.badge.TextSize = 22
	c.badge.Alignment = fyne.TextAlignCenter
	c.badge.TextStyle = fyne.TextStyle{Bold: true}
	c.badge.Hide()

	c.ExtendBaseWidget(c)
	return c
}

// BindPath reuses this card widget for a different PDF — keeps the same slot
// and geometry so neighbors never jump. Only the label + thumb change.
func (c *swipeCard) BindPath(path string) {
	if c.snapBack != nil {
		c.snapBack.Stop()
		c.snapBack = nil
	}
	c.path = path
	c.title = filepath.Base(path)
	c.dragX, c.dragY = 0, 0
	c.dragging = false
	c.previewDir = swipeNone
	c.dragProgress = 0
	// Title text changed — invalidate the ellipsize cache.
	c.nameCacheW = -1
	// Title is ellipsized to the current card width on the next Layout.
	c.nameLbl.Text = c.title
	c.nameLbl.Refresh()
	// Clear thumb until the new render lands — avoids flashing the old PDF.
	c.thumb.Image = image.NewRGBA(image.Rect(0, 0, 1, 1))
	c.thumb.Refresh()
	c.applyDragVisual()
	c.Refresh()
}

func (c *swipeCard) SetThumbnail(img image.Image) {
	if img == nil {
		return
	}
	c.thumb.Image = img
	c.thumb.Refresh()
	c.Refresh()
}

func (c *swipeCard) SetMinCardSize(sz fyne.Size) {
	c.minSize = sz
	c.thumb.SetMinSize(fyne.NewSize(sz.Width-24, sz.Height-48))
}

func (c *swipeCard) CreateRenderer() fyne.WidgetRenderer {
	// Paint order: face → thumb → title → wash → border → badge (top).
	return &swipeCardRenderer{card: c, objects: []fyne.CanvasObject{
		c.bg, c.thumb, c.nameLbl, c.overlay, c.border, c.badgeBg, c.badge,
	}}
}

func (c *swipeCard) MinSize() fyne.Size {
	return c.minSize
}

// Dragged implements fyne.Draggable — accumulate delta, raise above siblings,
// and restyle for direction feedback.
//
// This runs once per pointer-move event, so it stays off the heavyweight
// widget.Refresh() path (which nils the theme cache and re-refreshes every
// face object): applyDragVisual restyles the objects it changes, and
// layoutFace repositions everything — canvas.Rectangle/Text/Image.Move
// already marks the canvas dirty, so no explicit Refresh is needed here.
func (c *swipeCard) Dragged(ev *fyne.DragEvent) {
	if !c.dragging {
		c.dragging = true
		if c.snapBack != nil {
			c.snapBack.Stop()
			c.snapBack = nil
		}
		if c.board != nil {
			c.board.RaiseCard(c)
		}
	}
	c.dragX += ev.Dragged.DX
	c.dragY += ev.Dragged.DY
	c.applyDragVisual()
	c.layoutFace(c.Size())
}

// DragEnd implements fyne.Draggable — commit or animate back to rest.
func (c *swipeCard) DragEnd() {
	dir := c.commitDir()
	c.dragging = false
	if dir == swipeNone {
		c.applyDragVisual()
		c.startSnapBack()
		return
	}
	// Capture path before the callback rebinds this card to a new PDF.
	path := c.path
	if c.onSwipe != nil {
		c.onSwipe(path, dir)
	}
	// If the card was rebound, BindPath already cleared drag state; if it was
	// removed, this widget is gone from the board. Safe either way.
	if c.path == path {
		c.dragX, c.dragY = 0, 0
		c.applyDragVisual()
		c.layoutFace(c.Size())
	}
}

// startSnapBack eases dragX/dragY back to 0 after an uncommitted drag,
// instead of teleporting the card back to rest. Falls back to an instant
// snap when there is no running app to drive the animation (e.g. bare unit
// tests that construct a swipeCard without a Fyne app).
func (c *swipeCard) startSnapBack() {
	if fyne.CurrentApp() == nil {
		c.dragX, c.dragY = 0, 0
		c.applyDragVisual()
		c.layoutFace(c.Size())
		return
	}
	fromX, fromY := c.dragX, c.dragY
	anim := fyne.NewAnimation(180*time.Millisecond, func(p float32) {
		c.dragX = fromX * (1 - p)
		c.dragY = fromY * (1 - p)
		c.applyDragVisual()
		c.layoutFace(c.Size())
	})
	anim.Curve = fyne.AnimationEaseOut
	c.snapBack = anim
	anim.Start()
}

func (c *swipeCard) commitDir() swipeDir {
	ax := float32(math.Abs(float64(c.dragX)))
	ay := float32(math.Abs(float64(c.dragY)))
	// Prefer the dominant axis so diagonal flicks still feel intentional.
	if ax >= ay && ax >= swipeCommitX {
		if c.dragX < 0 {
			return swipeLeft
		}
		return swipeRight
	}
	if ay > ax && c.dragY >= swipeCommitY {
		return swipeDown
	}
	return swipeNone
}

// previewDirFromDrag returns a soft direction once the pointer has moved
// enough to read intent, even before commit threshold.
func (c *swipeCard) previewDirFromDrag() swipeDir {
	if d := c.commitDir(); d != swipeNone {
		return d
	}
	ax := float32(math.Abs(float64(c.dragX)))
	ay := float32(math.Abs(float64(c.dragY)))
	if ax < swipePreviewPx && ay < swipePreviewPx {
		return swipeNone
	}
	if ax >= ay {
		if c.dragX < 0 {
			return swipeLeft
		}
		return swipeRight
	}
	if c.dragY > 0 {
		return swipeDown
	}
	return swipeNone
}

func (c *swipeCard) dragProgressToward(dir swipeDir) float32 {
	var dist, need float32
	switch dir {
	case swipeLeft, swipeRight:
		dist = float32(math.Abs(float64(c.dragX)))
		need = swipeCommitX
	case swipeDown:
		dist = c.dragY
		if dist < 0 {
			dist = 0
		}
		need = swipeCommitY
	default:
		return 0
	}
	if need <= 0 {
		return 0
	}
	p := dist / need
	if p > 1 {
		return 1
	}
	if p < 0 {
		return 0
	}
	return p
}

func (c *swipeCard) applyDragVisual() {
	if !c.dragging {
		c.previewDir = swipeNone
		c.dragProgress = 0
		c.hideOverlay()
		return
	}

	dir := c.previewDirFromDrag()
	c.previewDir = dir
	if dir == swipeNone {
		c.dragProgress = 0
		c.hideOverlay()
		return
	}
	c.dragProgress = c.dragProgressToward(dir)

	accent, label, glyph := c.overlayStyle(dir)
	// Wash: builds from readable (~0.28) to strong (~0.72) as the drag commits.
	washA := uint8(70 + c.dragProgress*110)
	c.overlay.FillColor = withAlpha(accent, washA)
	c.overlay.Show()

	// Border thickens and solidifies toward commit.
	c.border.StrokeColor = withAlpha(accent, uint8(160+c.dragProgress*95))
	c.border.StrokeWidth = 3 + c.dragProgress*3

	// Badge: solid accent pill, white label — the primary decision readout.
	c.badgeBg.FillColor = withAlpha(accent, uint8(200+c.dragProgress*55))
	c.badgeBg.Show()
	c.badge.Text = glyph + "  " + label
	c.badge.Color = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	// Scale type slightly with progress so commit feels "locked in".
	c.badge.TextSize = 18 + c.dragProgress*6
	c.badge.Show()

	c.overlay.Refresh()
	c.border.Refresh()
	c.badgeBg.Refresh()
	c.badge.Refresh()
}

func (c *swipeCard) overlayStyle(dir swipeDir) (accent color.NRGBA, label, glyph string) {
	switch dir {
	case swipeLeft:
		accent = swipeLeftAccent
		glyph = "←"
		label = "LEFT"
		if c.board != nil && c.board.leftLabel != "" {
			label = c.board.leftLabel
		}
	case swipeRight:
		accent = swipeRightAccent
		glyph = "→"
		label = "RIGHT"
		if c.board != nil && c.board.rightLabel != "" {
			label = c.board.rightLabel
		}
	case swipeDown:
		accent = swipeSkipAccent
		glyph = "↓"
		label = "SKIP"
	}
	return accent, label, glyph
}

func (c *swipeCard) hideOverlay() {
	c.overlay.Hide()
	c.badgeBg.Hide()
	c.badge.Hide()
	c.badge.Text = ""
	c.border.StrokeColor = color.Transparent
	c.border.StrokeWidth = 4
	c.overlay.Refresh()
	c.badgeBg.Refresh()
	c.badge.Refresh()
	c.border.Refresh()
}

// withAlpha returns accent with the given alpha (0–255), keeping RGB.
func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
}

// Cursor implements desktop.Cursorable so the card advertises itself as grab-able.
func (c *swipeCard) Cursor() desktop.Cursor {
	return desktop.PointerCursor
}

var (
	_ fyne.Draggable     = (*swipeCard)(nil)
	_ desktop.Cursorable = (*swipeCard)(nil)
)

type swipeCardRenderer struct {
	card    *swipeCard
	objects []fyne.CanvasObject
}

func (r *swipeCardRenderer) Destroy() {}

func (r *swipeCardRenderer) Layout(size fyne.Size) {
	r.card.layoutFace(size)
}

// layoutFace positions every face object at the card's rest geometry offset
// by the live drag delta. It is the single source of truth for "where does
// the card sit right now" — called both from the renderer's Layout (window
// resize / capacity change) and directly from Dragged/DragEnd/startSnapBack
// so the drag offset reaches the canvas every frame, not just on the next
// container-driven layout pass.
//
// canvas.Rectangle/Text/Image.Move already marks the canvas dirty when the
// position actually changes, so no extra Refresh call is needed here for
// repositioning alone.
func (c *swipeCard) layoutFace(size fyne.Size) {
	// Offset the whole card face by the live drag delta for the swipe feel.
	// The widget's own position stays on its grid slot; only the face moves,
	// so neighbors keep their seats while this card floats over them.
	ox, oy := c.dragX, c.dragY
	c.bg.Resize(size)
	c.bg.Move(fyne.NewPos(ox, oy))
	c.border.Resize(size)
	c.border.Move(fyne.NewPos(ox, oy))
	c.overlay.Resize(size)
	c.overlay.Move(fyne.NewPos(ox, oy))

	pad := float32(10)
	nameH := float32(18)
	thumbW := size.Width - pad*2
	thumbH := size.Height - pad*3 - nameH
	if thumbH < 40 {
		thumbH = 40
	}
	c.thumb.Resize(fyne.NewSize(thumbW, thumbH))
	c.thumb.Move(fyne.NewPos(pad+ox, pad+oy))

	// Keep the title within the thumbnail width (same content column).
	// Cached: title + width don't change between drag frames, and
	// MeasureText's binary search is a real cost in that hot path.
	if thumbW != c.nameCacheW {
		c.nameCacheText = ellipsizeToWidth(c.title, thumbW, c.nameLbl.TextSize, c.nameLbl.TextStyle)
		c.nameCacheW = thumbW
	}
	c.nameLbl.Text = c.nameCacheText
	c.nameLbl.Resize(fyne.NewSize(thumbW, nameH))
	c.nameLbl.Move(fyne.NewPos(pad+ox, pad*2+thumbH+oy))

	// Centered decision badge — large enough to read at a glance while dragging.
	badgeH := float32(44) + c.dragProgress*8
	badgeW := size.Width - pad*2
	if badgeW > 200 {
		badgeW = 200
	}
	if badgeW < 96 {
		badgeW = size.Width - 8
	}
	// Ellipsize long category names so the pill stays inside the card.
	// Cached per (direction, width) — direction only changes a few times
	// per drag, unlike position which changes every frame.
	if c.badge.Visible() && c.previewDir != swipeNone {
		if c.previewDir != c.badgeCacheDir || badgeW != c.badgeCacheW {
			_, label, glyph := c.overlayStyle(c.previewDir)
			full := glyph + "  " + label
			// Budget ~badge interior width.
			c.badgeCacheText = ellipsizeToWidth(full, badgeW-20, c.badge.TextSize, c.badge.TextStyle)
			c.badgeCacheDir = c.previewDir
			c.badgeCacheW = badgeW
		}
		c.badge.Text = c.badgeCacheText
	}
	bx := ox + (size.Width-badgeW)/2
	by := oy + (size.Height-badgeH)/2
	c.badgeBg.Resize(fyne.NewSize(badgeW, badgeH))
	c.badgeBg.Move(fyne.NewPos(bx, by))
	c.badge.Resize(fyne.NewSize(badgeW, badgeH))
	c.badge.Move(fyne.NewPos(bx, by))
}

// ellipsizeToWidth shortens s with a trailing ellipsis so its measured width
// does not exceed maxW (canvas.Text has no built-in truncation).
func ellipsizeToWidth(s string, maxW, textSize float32, style fyne.TextStyle) string {
	if maxW <= 0 || s == "" {
		return ""
	}
	if fyne.MeasureText(s, textSize, style).Width <= maxW {
		return s
	}
	const ell = "…"
	runes := []rune(s)
	lo, hi := 0, len(runes)
	best := ell
	for lo <= hi {
		mid := (lo + hi) / 2
		cand := string(runes[:mid]) + ell
		if fyne.MeasureText(cand, textSize, style).Width <= maxW {
			best = cand
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return best
}

func (r *swipeCardRenderer) MinSize() fyne.Size {
	return r.card.minSize
}

func (r *swipeCardRenderer) Objects() []fyne.CanvasObject { return r.objects }

func (r *swipeCardRenderer) Refresh() {
	// Widgets are expected to re-run their own layout on Refresh — Fyne does
	// not call Layout() again on its own just because state changed (see
	// e.g. widget.Button's renderer). Without this, BindPath/SetThumbnail
	// etc. would only reposition the face on the next real container resize.
	r.card.layoutFace(r.card.Size())

	// Don't stomp drag overlay colors — only refresh static face tokens.
	if !r.card.dragging {
		r.card.bg.FillColor = theme.Color(theme.ColorNameHeaderBackground)
		r.card.nameLbl.Color = theme.Color(theme.ColorNameForeground)
	} else {
		r.card.bg.FillColor = theme.Color(theme.ColorNameHeaderBackground)
	}
	r.card.bg.Refresh()
	r.card.thumb.Refresh()
	r.card.nameLbl.Refresh()
	r.card.overlay.Refresh()
	r.card.border.Refresh()
	r.card.badgeBg.Refresh()
	r.card.badge.Refresh()
}
