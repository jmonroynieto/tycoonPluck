package ui

import (
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// Card size targets for capacity math (logical Fyne units).
const (
	swipeCardMinW float32 = 150
	swipeCardMinH float32 = 190
	swipeCardGap  float32 = 12
	swipeMaxCards         = 12 // cap concurrent pdftoppm work
)

// swipeCapacity returns how many cards fit in the given board size, plus the
// column count to use for the grid. Always at least 1×1 when size is positive.
func swipeCapacity(size fyne.Size) (n, cols int) {
	if size.Width < 1 || size.Height < 1 {
		return 1, 1
	}
	cols = int(math.Max(1, math.Floor(float64((size.Width+swipeCardGap)/(swipeCardMinW+swipeCardGap)))))
	rows := int(math.Max(1, math.Floor(float64((size.Height+swipeCardGap)/(swipeCardMinH+swipeCardGap)))))
	n = cols * rows
	if n > swipeMaxCards {
		n = swipeMaxCards
		cols = int(math.Min(float64(cols), float64(n)))
		if cols < 1 {
			cols = 1
		}
	}
	return n, cols
}

// swipeBoard hosts an adaptive grid of swipe cards.
//
// Cards keep a fixed slot index for layout position. Object order is only used
// for paint z-order (the card being dragged is moved last so it draws on top).
// Replacing one PDF rebinds a single card in place — neighbors do not move and
// do not re-render.
type swipeBoard struct {
	widget.BaseWidget

	emptyHint  *widget.Label
	cards      []*swipeCard // paint order; layout uses each card.slot
	cols       int
	capacity   int
	cellSize   fyne.Size
	onCapacity func(n, cols int)
	lastSize   fyne.Size
	raising    bool // suppress capacity callbacks while only z-order changes

	// Category names shown on the drag overlay badge (set by the app when
	// left/right selects change). Empty → generic LEFT/RIGHT labels.
	leftLabel  string
	rightLabel string
}

func newSwipeBoard(onCapacity func(n, cols int)) *swipeBoard {
	b := &swipeBoard{
		onCapacity: onCapacity,
		cols:       2,
		capacity:   1,
		cellSize:   fyne.NewSize(swipeCardMinW, swipeCardMinH),
	}
	b.emptyHint = widget.NewLabel("Open a folder, pick left & right categories,\nthen drag cards.")
	b.emptyHint.Alignment = fyne.TextAlignCenter
	b.emptyHint.Wrapping = fyne.TextWrapWord
	b.emptyHint.Importance = widget.LowImportance

	b.ExtendBaseWidget(b)
	return b
}

func (b *swipeBoard) CreateRenderer() fyne.WidgetRenderer {
	return &swipeBoardRenderer{board: b}
}

func (b *swipeBoard) Resize(size fyne.Size) {
	b.BaseWidget.Resize(size)
	b.considerSize(size)
}

func (b *swipeBoard) considerSize(size fyne.Size) {
	if b.raising {
		return
	}
	if size.Width < 40 || size.Height < 40 {
		return
	}
	if nearlySameSize(size, b.lastSize) {
		return
	}
	b.lastSize = size
	n, cols := swipeCapacity(size)
	b.recomputeCellSize(size, cols, n)
	if n == b.capacity && cols == b.cols {
		// Still push new cell sizes onto existing cards when the window
		// stretched without changing capacity.
		for _, c := range b.cards {
			c.SetMinCardSize(b.cellSize)
		}
		b.Refresh()
		return
	}
	b.capacity = n
	b.cols = cols
	if b.onCapacity != nil {
		b.onCapacity(n, cols)
	}
}

func (b *swipeBoard) recomputeCellSize(size fyne.Size, cols, capN int) {
	if cols < 1 {
		cols = 1
	}
	rows := (capN + cols - 1) / cols
	if rows < 1 {
		rows = 1
	}
	w := (size.Width - swipeCardGap*float32(cols-1)) / float32(cols)
	h := (size.Height - swipeCardGap*float32(rows-1)) / float32(rows)
	if w < swipeCardMinW*0.85 {
		w = swipeCardMinW * 0.85
	}
	if h < swipeCardMinH*0.85 {
		h = swipeCardMinH * 0.85
	}
	b.cellSize = fyne.NewSize(w, h)
}

func nearlySameSize(a, b fyne.Size) bool {
	return math.Abs(float64(a.Width-b.Width)) < 8 && math.Abs(float64(a.Height-b.Height)) < 8
}

// Capacity returns the last computed card capacity (at least 1 after layout).
func (b *swipeBoard) Capacity() int {
	if b.capacity < 1 {
		return 1
	}
	return b.capacity
}

// Cards returns the current card widgets (paint order).
func (b *swipeBoard) Cards() []*swipeCard {
	return b.cards
}

// SetCards installs a full card set (initial open / capacity change only).
// Prefer ReplaceCard / RemoveCard for per-decision updates.
func (b *swipeBoard) SetCards(cards []*swipeCard) {
	for i, c := range cards {
		c.slot = i
		c.board = b
		c.SetMinCardSize(b.cellSize)
	}
	b.cards = cards
	if len(cards) == 0 {
		b.emptyHint.Show()
	} else {
		b.emptyHint.Hide()
	}
	b.Refresh()
}

// RaiseCard draws c above every neighbor for the duration of a drag.
// Position is unchanged (slot-based layout); only paint order moves.
func (b *swipeBoard) RaiseCard(c *swipeCard) {
	if c == nil || len(b.cards) < 2 {
		return
	}
	idx := -1
	for i, x := range b.cards {
		if x == c {
			idx = i
			break
		}
	}
	if idx < 0 || idx == len(b.cards)-1 {
		return // already topmost (or not found)
	}
	b.raising = true
	b.cards = append(append(b.cards[:idx:idx], b.cards[idx+1:]...), c)
	b.Refresh()
	b.raising = false
}

// RemoveCard drops one card and compacts slot indices of the survivors
// without recreating them (thumbs stay). Used when the queue has nothing
// left to put in the freed slot.
func (b *swipeBoard) RemoveCard(c *swipeCard) {
	if c == nil {
		return
	}
	out := b.cards[:0]
	for _, x := range b.cards {
		if x != c {
			out = append(out, x)
		}
	}
	// Re-slice into a fresh backing array so we don't retain the removed card.
	b.cards = append([]*swipeCard(nil), out...)
	for i, x := range b.cards {
		x.slot = i
	}
	if len(b.cards) == 0 {
		b.emptyHint.Show()
	}
	b.Refresh()
}

// slotPos returns the top-left of grid slot i inside the board.
func (b *swipeBoard) slotPos(slot int) fyne.Position {
	cols := b.cols
	if cols < 1 {
		cols = 1
	}
	col := slot % cols
	row := slot / cols
	x := float32(col) * (b.cellSize.Width + swipeCardGap)
	y := float32(row) * (b.cellSize.Height + swipeCardGap)
	return fyne.NewPos(x, y)
}

type swipeBoardRenderer struct {
	board *swipeBoard
}

func (r *swipeBoardRenderer) Destroy() {}

func (r *swipeBoardRenderer) Layout(size fyne.Size) {
	// emptyHint centered when no cards
	if len(r.board.cards) == 0 {
		r.board.emptyHint.Resize(size)
		r.board.emptyHint.Move(fyne.NewPos(0, 0))
		return
	}
	for _, c := range r.board.cards {
		c.Resize(r.board.cellSize)
		c.Move(r.board.slotPos(c.slot))
	}
}

func (r *swipeBoardRenderer) MinSize() fyne.Size {
	return fyne.NewSize(swipeCardMinW, swipeCardMinH)
}

func (r *swipeBoardRenderer) Objects() []fyne.CanvasObject {
	// emptyHint first (under), then cards in paint order — last card is topmost.
	objs := make([]fyne.CanvasObject, 0, len(r.board.cards)+1)
	objs = append(objs, r.board.emptyHint)
	for _, c := range r.board.cards {
		objs = append(objs, c)
	}
	return objs
}

func (r *swipeBoardRenderer) Refresh() {
	r.board.emptyHint.Refresh()
	for _, c := range r.board.cards {
		c.Refresh()
	}
}
