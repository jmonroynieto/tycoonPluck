package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// categoryRow is one row in the category sidebar: the category button itself,
// plus a delete ("×") button that only appears on hover. Deleting only ever
// removes the name from the in-memory/persisted category list — it never
// touches sourceDir/CategoryName on disk.
//
// Hover handling is subtle in Fyne: widget.Button implements desktop.Hoverable,
// and the desktop driver delivers hover events only to the *deepest* Hoverable
// under the pointer. If the row relied solely on its own MouseIn/MouseOut, the
// child buttons would steal those events and the × would never appear. Both
// buttons therefore forward enter/exit into the row, which keeps a small
// refcount so moving between name and × does not flicker the delete control
// away mid-gesture.
type categoryRow struct {
	widget.BaseWidget
	nameBtn    *rowButton
	deleteBtn  *rowButton
	hoverCount int
}

// rowButton is a widget.Button that reports hover enter/exit to its parent row.
type rowButton struct {
	widget.Button
	row *categoryRow
}

func (b *rowButton) MouseIn(ev *desktop.MouseEvent) {
	b.Button.MouseIn(ev)
	if b.row != nil {
		b.row.hoverDelta(+1)
	}
}

func (b *rowButton) MouseOut() {
	b.Button.MouseOut()
	if b.row != nil {
		b.row.hoverDelta(-1)
	}
}

var _ desktop.Hoverable = (*rowButton)(nil)

func newCategoryRow(label string, onAssign, onDelete func()) *categoryRow {
	r := &categoryRow{}

	r.nameBtn = &rowButton{row: r}
	r.nameBtn.ExtendBaseWidget(r.nameBtn)
	r.nameBtn.SetText(label)
	r.nameBtn.OnTapped = onAssign
	r.nameBtn.Alignment = widget.ButtonAlignLeading

	r.deleteBtn = &rowButton{row: r}
	r.deleteBtn.ExtendBaseWidget(r.deleteBtn)
	r.deleteBtn.SetIcon(theme.ContentClearIcon())
	r.deleteBtn.OnTapped = onDelete
	r.deleteBtn.Importance = widget.LowImportance
	r.deleteBtn.Hide()

	r.ExtendBaseWidget(r)
	return r
}

func (r *categoryRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, nil, r.deleteBtn, r.nameBtn))
}

// MouseIn, MouseMoved, and MouseOut implement desktop.Hoverable for any
// pointer position that lands on the row itself (e.g. padding between the
// two buttons) rather than on a child button.
func (r *categoryRow) MouseIn(*desktop.MouseEvent) {
	r.hoverDelta(+1)
}

func (r *categoryRow) MouseMoved(*desktop.MouseEvent) {}

func (r *categoryRow) MouseOut() {
	r.hoverDelta(-1)
}

// hoverDelta adjusts the in-row hover refcount and shows/hides the delete
// button when the count crosses zero. A refcount is required because leave
// events on one child fire before enter events on the sibling during a
// single mouse-move; without it the × would hide under the cursor on the
// way from the name button to the delete button.
func (r *categoryRow) hoverDelta(delta int) {
	r.hoverCount += delta
	if r.hoverCount < 0 {
		r.hoverCount = 0
	}
	wantVisible := r.hoverCount > 0
	if wantVisible == r.deleteBtn.Visible() {
		return
	}
	if wantVisible {
		r.deleteBtn.Show()
	} else {
		r.deleteBtn.Hide()
	}
	// Show/Hide on the child only refreshes the child. The Border layout
	// inside this row must re-run so the × actually receives a non-zero
	// size and the name button yields space for it.
	r.Refresh()
}

// SetEnabled toggles only the assign button. The delete button stays enabled
// regardless — delisting a category shouldn't require a PDF to be queued.
func (r *categoryRow) SetEnabled(enabled bool) {
	if enabled {
		r.nameBtn.Enable()
	} else {
		r.nameBtn.Disable()
	}
}

var _ desktop.Hoverable = (*categoryRow)(nil)
