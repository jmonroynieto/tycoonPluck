// Package ui is the Fyne window for tycoonPluck.
package ui

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"tycoonPluck/internal/categories"
	"tycoonPluck/internal/openext"
	"tycoonPluck/internal/preview"
	"tycoonPluck/internal/session"
	"tycoonPluck/internal/sorter"
	"tycoonPluck/internal/version"
)

// workMode is which triage interaction the main pane is showing.
type workMode int

const (
	modeReview workMode = iota // single-file preview + category buttons
	modeSwipe                  // multi-card drag board (← / → / ↓)
)

// App wires the sorter session to a Fyne window.
type App struct {
	fyneApp fyne.App
	win     fyne.Window
	sorter  sorter.Sorter
	journal *session.Store
	cats    []string

	filenameLabel *widget.Label
	progressLabel *widget.Label
	statusLabel   *widget.Label
	outputHint    *widget.Label
	placeholder   *canvas.Text
	emptyIcon     *canvas.Image
	previewImg    *canvas.Image
	previewStack  *fyne.Container
	categoryBox   *fyne.Container
	skipBtn       *widget.Button
	undoBtn       *widget.Button
	inspectBtn    *widget.Button
	openBtn       *widget.Button

	// Multi-page preview: up to 3 pages, rendered together, one shown at a
	// time via the pager buttons (hidden unless the current PDF has >1 page).
	pages     []image.Image
	pageIndex int
	pageBtns  [3]*widget.Button

	previewMu  sync.Mutex
	previewGen uint64
	// previewWG tracks in-flight preview renders so tests can deterministically
	// wait for one to finish (see waitForPreview) instead of racing it.
	previewWG sync.WaitGroup

	// modalOpen is true while a blocking dialog (e.g. Add category) is up,
	// so window-level keyboard shortcuts do not triage underneath it.
	modalOpen bool

	// openFile launches the system viewer for a PDF. Defaults to openext.Open;
	// tests override it to verify wiring without spawning a real process.
	openFile func(string) error

	// --- mode toggle + swipe board ------------------------------------------
	mode          workMode
	modeReviewBtn *widget.Button
	modeSwipeBtn  *widget.Button
	reviewPane    fyne.CanvasObject
	swipePane     fyne.CanvasObject

	swipeBoard       *swipeBoard
	swipeLeftSelect  *widget.Select
	swipeRightSelect *widget.Select
	swipeLeftCat     string
	swipeRightCat    string
	swipeProgress    *widget.Label
	swipeStatus      *widget.Label
	swipeUndoBtn     *widget.Button
	swipeHint        *widget.Label

	// swipeCardByPath lets async thumbnail renders land on the right card
	// after a refill; cleared/replaced each rebuild.
	swipeCardByPath map[string]*swipeCard
	swipeMu         sync.Mutex
	swipeGen        uint64
	swipeWG         sync.WaitGroup

	// thumbs caches swipe first-page rasters and prefetches upcoming queue
	// items; Want() halts work that is no longer interesting.
	thumbs *preview.Cache
}

// New builds the main window (not yet shown).
func New(a fyne.App) *App {
	a.Settings().SetTheme(newAppTheme())

	w := a.NewWindow("tycoonPluck")
	w.Resize(fyne.NewSize(1040, 720))

	ui := &App{
		fyneApp:  a,
		win:      w,
		journal:  session.Load(),
		cats:     categories.Load(), // empty until the user adds categories
		openFile: openext.Open,
		thumbs:   preview.NewCache(),
	}
	ui.build()
	ui.refresh()
	w.Canvas().SetOnTypedKey(ui.onKey)
	return ui
}

// ShowAndRun presents the window and runs the app event loop.
func (ui *App) ShowAndRun() {
	ui.win.ShowAndRun()
}

func (ui *App) build() {
	variant := ui.fyneApp.Settings().ThemeVariant()
	pal := paletteFor(variant)

	// --- typography -----------------------------------------------------
	ui.filenameLabel = widget.NewLabel("")
	ui.filenameLabel.TextStyle = fyne.TextStyle{Bold: true}
	ui.filenameLabel.SizeName = theme.SizeNameSubHeadingText
	ui.filenameLabel.Truncation = fyne.TextTruncateEllipsis

	ui.progressLabel = widget.NewLabel("0 / 0")
	ui.progressLabel.Alignment = fyne.TextAlignCenter
	ui.progressLabel.SizeName = theme.SizeNameCaptionText
	ui.progressLabel.Importance = widget.LowImportance

	ui.statusLabel = widget.NewLabel("")
	ui.statusLabel.SizeName = theme.SizeNameCaptionText
	ui.statusLabel.Importance = widget.LowImportance

	ui.outputHint = widget.NewLabel("Output: open a folder")
	ui.outputHint.Wrapping = fyne.TextWrapWord
	ui.outputHint.SizeName = theme.SizeNameCaptionText
	ui.outputHint.Importance = widget.LowImportance

	// --- preview area: empty state (icon + message) stacked over the image ---
	ui.emptyIcon = canvas.NewImageFromResource(theme.NewColoredResource(theme.FolderOpenIcon(), theme.ColorNamePlaceHolder))
	ui.emptyIcon.FillMode = canvas.ImageFillContain
	ui.emptyIcon.SetMinSize(fyne.NewSize(48, 48))

	ui.placeholder = canvas.NewText("Open a folder of PDFs to begin sorting.", pal.muted)
	ui.placeholder.Alignment = fyne.TextAlignCenter
	ui.placeholder.TextSize = 16

	emptyState := container.NewCenter(container.New(layout.NewCustomPaddedVBoxLayout(10),
		container.NewCenter(ui.emptyIcon),
		ui.placeholder,
	))

	ui.previewImg = canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	ui.previewImg.FillMode = canvas.ImageFillContain
	ui.previewImg.SetMinSize(fyne.NewSize(480, 560))
	ui.previewImg.Hide()

	ui.previewStack = container.NewStack(emptyState, ui.previewImg)

	// Page picker: up to 3 buttons, hidden individually until a multi-page
	// PDF is loaded (hidden children are excluded from HBox's MinSize, so
	// this row collapses to zero height rather than leaving a gap).
	pagerRow := container.NewHBox(layout.NewSpacer())
	for i := range ui.pageBtns {
		idx := i
		b := widget.NewButton(strconv.Itoa(i+1), func() { ui.showPage(idx) })
		b.Hide()
		ui.pageBtns[i] = b
		pagerRow.Add(b)
	}
	pagerRow.Add(layout.NewSpacer())

	previewCard := surfaceCard(
		container.New(layout.NewCustomPaddedLayout(28, 28, 28, 28),
			container.NewBorder(nil, pagerRow, nil, nil, ui.previewStack),
		),
		pal, 16,
	)

	// --- actions ----------------------------------------------------------
	ui.inspectBtn = widget.NewButtonWithIcon("Open PDF", theme.DocumentIcon(), ui.onInspect)
	ui.skipBtn = widget.NewButtonWithIcon("Skip", theme.MediaSkipNextIcon(), ui.onSkip)
	ui.undoBtn = widget.NewButtonWithIcon("Undo", theme.ContentUndoIcon(), ui.onUndo)
	ui.openBtn = widget.NewButtonWithIcon("Open folder", theme.FolderOpenIcon(), ui.onOpenFolder)
	ui.openBtn.Importance = widget.HighImportance

	// --- sidebar: categories ----------------------------------------------
	ui.categoryBox = container.New(layout.NewCustomPaddedVBoxLayout(8))
	ui.rebuildCategoryButtons()

	sectionLabel := widget.NewLabel("CATEGORIES")
	sectionLabel.TextStyle = fyne.TextStyle{Bold: true}
	sectionLabel.SizeName = theme.SizeNameCaptionText
	sectionLabel.Importance = widget.LowImportance

	addBtn := widget.NewButtonWithIcon("Add category", theme.ContentAddIcon(), ui.onAddCategory)
	addBtn.Alignment = widget.ButtonAlignLeading

	sidebarTop := container.New(layout.NewCustomPaddedVBoxLayout(14),
		sectionLabel,
		ui.categoryBox,
		addBtn,
	)
	sidebarBody := container.NewBorder(sidebarTop, ui.outputHint, nil, nil, layout.NewSpacer())
	sidebar := container.NewBorder(nil, nil, nil,
		hairline(pal),
		container.NewStack(
			&canvas.Rectangle{FillColor: pal.surface},
			container.New(layout.NewCustomPaddedLayout(20, 20, 22, 22), sidebarBody),
		),
	)

	// --- main content: filename / progress row + preview + status/actions ---
	progressPill := container.NewStack(
		&canvas.Rectangle{FillColor: pal.disabledBtn, CornerRadius: 999},
		container.New(layout.NewCustomPaddedLayout(4, 4, 12, 12), ui.progressLabel),
	)
	topRow := container.NewBorder(nil, nil, nil, progressPill, ui.filenameLabel)
	actions := container.NewHBox(layout.NewSpacer(), ui.inspectBtn, ui.skipBtn, ui.undoBtn)
	footer := container.New(layout.NewCustomPaddedVBoxLayout(6), ui.statusLabel, actions)

	ui.reviewPane = container.New(layout.NewCustomPaddedLayout(24, 24, 28, 28),
		container.NewBorder(topRow, footer, nil, nil, container.New(layout.NewCustomPaddedLayout(14, 14, 0, 0), previewCard)),
	)

	// --- swipe mode pane --------------------------------------------------
	ui.swipePane = ui.buildSwipePane(pal)

	// Stack both modes; only one is visible at a time.
	mainStack := container.NewStack(ui.reviewPane, ui.swipePane)
	ui.swipePane.Hide()

	// --- header -------------------------------------------------------------
	// Single-line lockup (title + subtitle side by side, not stacked) and a
	// smaller title size — this is app chrome framing the preview, not a
	// hero headline, so it shouldn't compete with it for vertical space.
	title := widget.NewLabel("tycoonPluck")
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.SizeName = theme.SizeNameSubHeadingText

	// Small, muted version tag riding right next to the name — not a UI
	// element anyone needs to read, just there for bug reports/screenshots.
	versionLabel := widget.NewLabel("v" + version.Version)
	versionLabel.SizeName = theme.SizeNameCaptionText
	versionLabel.Importance = widget.LowImportance
	nameBlock := container.New(layout.NewCustomPaddedHBoxLayout(4), title, versionLabel)

	subtitle := widget.NewLabel("PDF category sorter")
	subtitle.SizeName = theme.SizeNameCaptionText
	subtitle.Importance = widget.LowImportance

	titleBlock := container.New(layout.NewCustomPaddedHBoxLayout(8), nameBlock, subtitle)

	// Centered mode toggle: Review (classic) ↔ Swipe (quick match).
	ui.modeReviewBtn = widget.NewButton("Review", func() { ui.setMode(modeReview) })
	ui.modeSwipeBtn = widget.NewButton("Swipe", func() { ui.setMode(modeSwipe) })
	ui.modeReviewBtn.Importance = widget.HighImportance
	ui.modeSwipeBtn.Importance = widget.MediumImportance
	modeToggle := container.NewHBox(ui.modeReviewBtn, ui.modeSwipeBtn)

	headerRow := container.NewBorder(nil, nil, titleBlock, ui.openBtn, container.NewCenter(modeToggle))
	header := container.NewBorder(nil,
		hairline(pal),
		nil, nil,
		container.New(layout.NewCustomPaddedLayout(10, 10, 24, 24), headerRow),
	)

	body := container.NewHSplit(sidebar, mainStack)
	body.SetOffset(0.24)

	ui.win.SetContent(container.NewBorder(header, nil, nil, nil, body))
}

// buildSwipePane constructs the quick-match board: left/right category
// pickers, adaptive card grid, and a light footer (undo + status).
func (ui *App) buildSwipePane(pal palette) fyne.CanvasObject {
	ui.swipeLeftSelect = widget.NewSelect(nil, func(name string) {
		ui.swipeLeftCat = name
		if ui.swipeBoard != nil {
			ui.swipeBoard.leftLabel = name
		}
	})
	ui.swipeLeftSelect.PlaceHolder = "Left category"
	ui.swipeRightSelect = widget.NewSelect(nil, func(name string) {
		ui.swipeRightCat = name
		if ui.swipeBoard != nil {
			ui.swipeBoard.rightLabel = name
		}
	})
	ui.swipeRightSelect.PlaceHolder = "Right category"

	ui.swipeHint = widget.NewLabel("← left   ·   ↓ skip   ·   right →")
	ui.swipeHint.Alignment = fyne.TextAlignCenter
	ui.swipeHint.SizeName = theme.SizeNameCaptionText
	ui.swipeHint.Importance = widget.LowImportance

	ui.swipeProgress = widget.NewLabel("0 / 0")
	ui.swipeProgress.Alignment = fyne.TextAlignCenter
	ui.swipeProgress.SizeName = theme.SizeNameCaptionText
	ui.swipeProgress.Importance = widget.LowImportance

	progressPill := container.NewStack(
		&canvas.Rectangle{FillColor: pal.disabledBtn, CornerRadius: 999},
		container.New(layout.NewCustomPaddedLayout(4, 4, 12, 12), ui.swipeProgress),
	)

	// Direction-colored selectors match card drag feedback (left rose / right sage).
	leftArrow := canvas.NewText("←", swipeLeftAccent)
	leftArrow.TextStyle = fyne.TextStyle{Bold: true}
	leftArrow.TextSize = 16
	rightArrow := canvas.NewText("→", swipeRightAccent)
	rightArrow.TextStyle = fyne.TextStyle{Bold: true}
	rightArrow.TextSize = 16

	pickerRow := container.NewBorder(nil, nil,
		container.NewHBox(leftArrow, tintedSelect(ui.swipeLeftSelect, swipeLeftAccent)),
		container.NewHBox(tintedSelect(ui.swipeRightSelect, swipeRightAccent), rightArrow),
		container.NewCenter(ui.swipeHint),
	)
	top := container.New(layout.NewCustomPaddedVBoxLayout(8),
		container.NewBorder(nil, nil, nil, progressPill, pickerRow),
	)

	ui.swipeBoard = newSwipeBoard(func(n, cols int) {
		// Capacity changed with window size — grow/shrink, keep survivors.
		ui.syncSwipeBoard()
	})

	ui.swipeStatus = widget.NewLabel("Pick left & right categories, then drag cards.")
	ui.swipeStatus.SizeName = theme.SizeNameCaptionText
	ui.swipeStatus.Importance = widget.LowImportance
	ui.swipeUndoBtn = widget.NewButtonWithIcon("Undo", theme.ContentUndoIcon(), ui.onUndo)

	footer := container.NewBorder(nil, nil, ui.swipeStatus, ui.swipeUndoBtn, layout.NewSpacer())

	boardCard := surfaceCard(
		container.New(layout.NewCustomPaddedLayout(16, 16, 16, 16), ui.swipeBoard),
		pal, 16,
	)

	return container.New(layout.NewCustomPaddedLayout(24, 24, 28, 28),
		container.NewBorder(top, footer, nil, nil, boardCard),
	)
}

// setMode switches the main pane between classic review and swipe board.
func (ui *App) setMode(m workMode) {
	if ui.mode == m {
		return
	}
	ui.mode = m
	switch m {
	case modeSwipe:
		ui.modeReviewBtn.Importance = widget.MediumImportance
		ui.modeSwipeBtn.Importance = widget.HighImportance
		ui.reviewPane.Hide()
		ui.swipePane.Show()
		ui.syncSwipeCategorySelects()
	default:
		ui.modeReviewBtn.Importance = widget.HighImportance
		ui.modeSwipeBtn.Importance = widget.MediumImportance
		ui.swipePane.Hide()
		ui.reviewPane.Show()
	}
	ui.modeReviewBtn.Refresh()
	ui.modeSwipeBtn.Refresh()
	if root := ui.win.Content(); root != nil {
		root.Refresh()
	}
	ui.refresh()
}

// syncSwipeCategorySelects refreshes the left/right Select options from
// ui.cats and keeps a sensible default selection when possible.
func (ui *App) syncSwipeCategorySelects() {
	if ui.swipeLeftSelect == nil || ui.swipeRightSelect == nil {
		return
	}
	opts := append([]string(nil), ui.cats...)
	ui.swipeLeftSelect.Options = opts
	ui.swipeRightSelect.Options = opts

	leftOK, rightOK := false, false
	for _, c := range opts {
		if c == ui.swipeLeftCat {
			leftOK = true
		}
		if c == ui.swipeRightCat {
			rightOK = true
		}
	}
	if !leftOK {
		ui.swipeLeftCat = ""
		if len(opts) > 0 {
			ui.swipeLeftCat = opts[0]
		}
	}
	if !rightOK {
		ui.swipeRightCat = ""
		if len(opts) > 1 {
			ui.swipeRightCat = opts[1]
		} else if len(opts) == 1 && opts[0] != ui.swipeLeftCat {
			ui.swipeRightCat = opts[0]
		} else if len(opts) > 0 {
			// Only one category — still allow selecting it on the right so the
			// user can at least skip; assign-right will no-op if same intent.
			ui.swipeRightCat = opts[0]
		}
	}
	ui.swipeLeftSelect.Selected = ui.swipeLeftCat
	ui.swipeRightSelect.Selected = ui.swipeRightCat
	ui.swipeLeftSelect.Refresh()
	ui.swipeRightSelect.Refresh()
	if ui.swipeBoard != nil {
		ui.swipeBoard.leftLabel = ui.swipeLeftCat
		ui.swipeBoard.rightLabel = ui.swipeRightCat
	}
}

// surfaceCard wraps content in a rounded, bordered panel using the given
// palette — the shared "elevated surface" look for the preview area.
func surfaceCard(content fyne.CanvasObject, pal palette, radius float32) fyne.CanvasObject {
	bg := &canvas.Rectangle{FillColor: pal.surface, StrokeColor: pal.surfaceBorder, StrokeWidth: 1}
	bg.CornerRadius = radius
	return container.NewStack(bg, content)
}

// tintedSelect wraps a Select in a soft fill + accent border so left/right
// category pickers read as the same two-way options as the swipe gestures.
func tintedSelect(sel *widget.Select, accent color.Color) fyne.CanvasObject {
	// Soft wash of the accent over the elevated surface.
	r, g, b, _ := accent.RGBA()
	fill := color.NRGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: 0x38,
	}
	bg := canvas.NewRectangle(fill)
	bg.CornerRadius = 8
	bg.StrokeColor = accent
	bg.StrokeWidth = 1.5
	return container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(2, 2, 4, 4), sel))
}

// hairline is a 1px divider matching the palette's border tone.
func hairline(pal palette) fyne.CanvasObject {
	line := canvas.NewRectangle(pal.surfaceBorder)
	line.SetMinSize(fyne.NewSize(1, 1))
	return line
}

func (ui *App) rebuildCategoryButtons() {
	ui.categoryBox.RemoveAll()
	if len(ui.cats) == 0 {
		hint := widget.NewLabel("No categories yet.\nUse “Add category” to create some.")
		hint.Wrapping = fyne.TextWrapWord
		hint.Importance = widget.LowImportance
		hint.SizeName = theme.SizeNameCaptionText
		ui.categoryBox.Add(hint)
	} else {
		for i, name := range ui.cats {
			label := name
			if i < 9 {
				label = fmt.Sprintf("%d. %s", i+1, name)
			}
			cat := name
			row := newCategoryRow(label,
				func() { ui.assign(cat) },
				func() { ui.deleteCategory(cat) },
			)
			ui.categoryBox.Add(row)
		}
	}
	ui.categoryBox.Refresh()

	// categoryBox's height just changed, but Refresh() above only re-lays-out
	// categoryBox itself — its ancestors (which position siblings below it,
	// e.g. the "Add category" button) keep their previously computed sizes
	// until something forces a full re-layout. Refresh the window root to
	// pick up the new height everywhere; it's a no-op the first time this
	// runs from build(), before SetContent has been called.
	if root := ui.win.Content(); root != nil {
		root.Refresh()
	}
}

func (ui *App) onOpenFolder() {
	ui.modalOpen = true
	d := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		ui.modalOpen = false
		if err != nil {
			ui.setStatus(fmt.Sprintf("Error: %v", err))
			return
		}
		if uri == nil {
			return // cancelled
		}
		// Folder picker lists directories only (by design): pick the folder that
		// contains the PDFs at its top level — not individual files.
		n, err := ui.sorter.OpenFolder(uri.Path())
		if err != nil {
			dialog.ShowError(err, ui.win)
			return
		}
		// New folder → drop any thumbs from a previous session.
		if ui.thumbs != nil {
			ui.thumbs.Clear()
		}
		ui.applySessionForOpenFolder()
		ui.setStatus(fmt.Sprintf("Loaded %d PDF(s) to sort.", ui.sorter.Remaining()))
		_ = n
		ui.refresh()
	}, ui.win)
	// Fyne 2.8 FileDialog: Resize before Show panics — MinSize touches a nil
	// internal dialog. Show first (builds the body), then Resize.
	d.Show()
	d.Resize(fyne.NewSize(700, 500))
}

// applySessionForOpenFolder drops session-skipped names from the queue and
// rebuilds undo from the per-directory classification journal when files still exist.
func (ui *App) applySessionForOpenFolder() {
	if ui.journal == nil || !ui.sorter.HasFolder() {
		return
	}
	dir := ui.sorter.SourceDir
	ui.sorter.DropBasenames(ui.journal.SkippedSet(dir))
	ui.sorter.SetUndoStack(ui.journal.UndoEntriesStillOnDisk(dir))
}

func (ui *App) onAddCategory() {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Category name")
	ui.modalOpen = true
	form := dialog.NewForm(
		"Add category",
		"Add",
		"Cancel",
		[]*widget.FormItem{
			widget.NewFormItem("Name", entry),
		},
		func(ok bool) {
			ui.modalOpen = false
			if !ok {
				return
			}
			clean := categories.Sanitize(entry.Text)
			if clean == "" {
				dialog.ShowInformation("Invalid name", "Use a non-empty name without / \\ :", ui.win)
				return
			}
			for _, c := range ui.cats {
				if strings.EqualFold(c, clean) {
					dialog.ShowInformation("Exists", "That category already exists.", ui.win)
					return
				}
			}
			// Persist first; only mutate in-memory list after a successful save.
			next := append(append([]string(nil), ui.cats...), clean)
			if err := categories.Save(next); err != nil {
				dialog.ShowError(err, ui.win)
				return
			}
			ui.cats = next
			ui.rebuildCategoryButtons()
			ui.syncSwipeCategorySelects()
			ui.refresh()
		},
		ui.win,
	)
	form.Show()
	// Safe after Show for FormDialog (win already exists); size is a soft hint.
	form.Resize(fyne.NewSize(360, 160))
}

// deleteCategory removes name from the category list and persists the
// change. It only ever decouples the name — sourceDir/name on disk (and any
// files already sorted into it) is never touched. No confirmation dialog:
// this is meant to be a fast, low-stakes action (re-adding the same name
// recreates an identical entry, since the folder was never removed).
func (ui *App) deleteCategory(name string) {
	next := make([]string, 0, len(ui.cats))
	for _, c := range ui.cats {
		if c != name {
			next = append(next, c)
		}
	}
	if len(next) == len(ui.cats) {
		return // name not found; nothing to do
	}
	if err := categories.Save(next); err != nil {
		dialog.ShowError(err, ui.win)
		return
	}
	ui.cats = next
	ui.rebuildCategoryButtons()
	ui.syncSwipeCategorySelects()
	ui.refresh()
}

func (ui *App) assign(category string) {
	if ui.sorter.Current() == "" {
		return
	}
	ui.assignPath(ui.sorter.Current(), category)
}

// assignPath moves a specific queued PDF into category (review or swipe).
func (ui *App) assignPath(path, category string) {
	if path == "" {
		return
	}
	if len(ui.cats) == 0 {
		ui.setStatus("Add a category first.")
		return
	}
	entry, err := ui.sorter.AssignPath(path, category)
	if err != nil {
		dialog.ShowError(err, ui.win)
		return
	}
	// File left its original path — drop any cached raster for it.
	if ui.thumbs != nil {
		ui.thumbs.Drop(path)
	}
	msg := fmt.Sprintf("Moved → %s/%s", entry.Category, filepath.Base(entry.MovedTo))
	if ui.journal != nil && ui.sorter.HasFolder() {
		if err := ui.journal.RecordAssign(ui.sorter.SourceDir, entry); err != nil {
			// Classification on disk still moved; journal is best-effort.
			msg = fmt.Sprintf("%s (session log: %v)", msg, err)
		}
	}
	ui.setStatus(msg)
	if ui.mode == modeSwipe {
		// Keep neighbor cards put — only the decided slot is replaced.
		ui.refreshSwipeChrome()
		ui.replaceSwipeSlot(path)
		return
	}
	ui.refresh()
}

// skipPath drops a specific queued PDF from the session (swipe ↓).
func (ui *App) skipPath(path string) {
	skipped := ui.sorter.SkipPath(path)
	if skipped == "" {
		return
	}
	if ui.journal != nil && ui.sorter.HasFolder() {
		_ = ui.journal.RecordSkip(ui.sorter.SourceDir, filepath.Base(skipped))
	}
	ui.setStatus(fmt.Sprintf("Skipped %s", filepath.Base(skipped)))
	if ui.mode == modeSwipe {
		ui.refreshSwipeChrome()
		ui.replaceSwipeSlot(path)
		return
	}
	ui.refresh()
}

func (ui *App) onInspect() {
	path := ui.sorter.Current()
	if path == "" {
		return
	}
	if err := ui.openFile(path); err != nil {
		dialog.ShowError(err, ui.win)
		return
	}
	ui.setStatus(fmt.Sprintf("Opened in system viewer: %s", filepath.Base(path)))
}

func (ui *App) onSkip() {
	skipped := ui.sorter.Skip()
	if skipped == "" {
		return
	}
	if ui.journal != nil && ui.sorter.HasFolder() {
		_ = ui.journal.RecordSkip(ui.sorter.SourceDir, filepath.Base(skipped))
	}
	ui.setStatus(fmt.Sprintf("Skipped %s", filepath.Base(skipped)))
	ui.refresh()
}

func (ui *App) onUndo() {
	restored, err := ui.sorter.Undo()
	if err != nil {
		dialog.ShowError(err, ui.win)
		ui.refresh()
		return
	}
	if restored == "" {
		ui.setStatus("Nothing to undo.")
		ui.refresh()
		return
	}
	if ui.journal != nil && ui.sorter.HasFolder() {
		_ = ui.journal.PopLastAssign(ui.sorter.SourceDir)
	}
	ui.setStatus(fmt.Sprintf("Undid move: %s", filepath.Base(restored)))
	ui.refresh()
}

func (ui *App) onKey(ev *fyne.KeyEvent) {
	if ui.modalOpen {
		return
	}
	// Undo is shared across modes.
	if ev.Name == fyne.KeyZ {
		if ui.sorter.CanUndo() {
			ui.onUndo()
		}
		return
	}

	if ui.mode == modeSwipe {
		ui.onSwipeKey(ev)
		return
	}

	switch ev.Name {
	case fyne.KeyO:
		if ui.sorter.Current() != "" {
			ui.onInspect()
		}
	case fyne.KeyS:
		if ui.sorter.Current() != "" {
			ui.onSkip()
		}
	case fyne.Key1, fyne.Key2, fyne.Key3, fyne.Key4, fyne.Key5,
		fyne.Key6, fyne.Key7, fyne.Key8, fyne.Key9:
		idx := int(ev.Name[0] - '1')
		if idx >= 0 && idx < len(ui.cats) && ui.sorter.Current() != "" {
			ui.assign(ui.cats[idx])
		}
	}
}

// onSwipeKey applies keyboard triage to the first visible card (queue head).
// Left/Right assign to the configured categories; Down or S skips.
func (ui *App) onSwipeKey(ev *fyne.KeyEvent) {
	head := ui.sorter.Current()
	if head == "" {
		return
	}
	switch ev.Name {
	case fyne.KeyLeft:
		ui.onSwipeCard(head, swipeLeft)
	case fyne.KeyRight:
		ui.onSwipeCard(head, swipeRight)
	case fyne.KeyDown, fyne.KeyS:
		ui.onSwipeCard(head, swipeDown)
	case fyne.KeyO:
		ui.onInspect()
	}
}

func (ui *App) refresh() {
	if ui.sorter.HasFolder() {
		ui.outputHint.SetText("Output: subfolders in\n" + ui.sorter.SourceDir)
	} else {
		ui.outputHint.SetText("Output: open a folder")
	}

	if ui.mode == modeSwipe {
		ui.refreshSwipe()
		return
	}
	ui.refreshReview()
}

func (ui *App) refreshReview() {
	hasCurrent := ui.sorter.Current() != ""
	canAssign := hasCurrent && len(ui.cats) > 0
	for _, obj := range ui.categoryBox.Objects {
		if row, ok := obj.(*categoryRow); ok {
			row.SetEnabled(canAssign)
		}
	}
	if hasCurrent {
		ui.skipBtn.Enable()
		ui.inspectBtn.Enable()
	} else {
		ui.skipBtn.Disable()
		ui.inspectBtn.Disable()
	}
	if ui.sorter.CanUndo() {
		ui.undoBtn.Enable()
	} else {
		ui.undoBtn.Disable()
	}

	ui.progressLabel.SetText(ui.sorter.ProgressLabel())

	current := ui.sorter.Current()
	if current == "" {
		// Invalidate any in-flight preview so it cannot touch widgets after teardown.
		ui.previewMu.Lock()
		ui.previewGen++
		ui.previewMu.Unlock()
		// Halt review-kind prefetch; leave swipe-kind interest alone.
		if ui.thumbs != nil {
			ui.thumbs.Want(nil, preview.ReviewPreviewDPI, len(ui.pageBtns))
		}

		ui.filenameLabel.SetText("")
		ui.previewImg.Hide()
		ui.hidePager()
		ui.placeholder.Show()
		ui.emptyIcon.Show()
		switch {
		case !ui.sorter.HasFolder():
			ui.placeholder.Text = "Open a folder of PDFs to begin sorting."
			ui.emptyIcon.Resource = theme.NewColoredResource(theme.FolderOpenIcon(), theme.ColorNamePlaceHolder)
		case ui.sorter.TotalStarted == 0:
			ui.placeholder.Text = "No PDFs in this folder."
			ui.emptyIcon.Resource = theme.NewColoredResource(theme.DocumentIcon(), theme.ColorNamePlaceHolder)
		default:
			ui.placeholder.Text = "Queue empty — all assigned or skipped."
			ui.emptyIcon.Resource = theme.NewColoredResource(theme.ConfirmIcon(), theme.ColorNamePlaceHolder)
		}
		ui.placeholder.Refresh()
		ui.emptyIcon.Refresh()
		ui.previewStack.Refresh()
		return
	}

	ui.filenameLabel.SetText(filepath.Base(current))
	// Prefetch current + next queue items at review resolution while we paint.
	ui.prefetchReviewPreviews()
	ui.showPreview(current)
}

// refreshSwipe updates chrome and syncs the board to capacity/queue state.
// Per-card decisions use replaceSwipeSlot instead so neighbors stay put.
func (ui *App) refreshSwipe() {
	// Sidebar assign is review-mode only; keep rows disabled while swiping
	// so a mis-click doesn't pull the queue head out of order with the board.
	for _, obj := range ui.categoryBox.Objects {
		if row, ok := obj.(*categoryRow); ok {
			row.SetEnabled(false)
		}
	}
	ui.refreshSwipeChrome()
	ui.syncSwipeCategorySelects()
	ui.syncSwipeBoard()
}

// refreshSwipeChrome updates progress/undo without touching card widgets.
func (ui *App) refreshSwipeChrome() {
	if ui.sorter.CanUndo() {
		ui.swipeUndoBtn.Enable()
	} else {
		ui.swipeUndoBtn.Disable()
	}
	ui.swipeProgress.SetText(ui.sorter.ProgressLabel())
	// Keep drag-overlay badges in sync with the category selectors.
	if ui.swipeBoard != nil {
		ui.swipeBoard.leftLabel = ui.swipeLeftCat
		ui.swipeBoard.rightLabel = ui.swipeRightCat
	}
}

// syncSwipeBoard grows/shrinks the card set to match window capacity and the
// current queue. Existing cards whose path is still queued are kept (and keep
// their thumbnails). Only brand-new slots render. Called on mode enter, folder
// open, capacity change — not on every swipe decision.
func (ui *App) syncSwipeBoard() {
	if ui.swipeBoard == nil {
		return
	}
	capN := ui.swipeBoard.Capacity()

	// Keep cards that still refer to a queued path; drop the rest.
	queued := make(map[string]struct{}, ui.sorter.Remaining())
	for _, p := range ui.sorter.Queue {
		queued[p] = struct{}{}
	}
	kept := make([]*swipeCard, 0, capN)
	byPath := make(map[string]*swipeCard, capN)
	for _, c := range ui.swipeBoard.Cards() {
		if _, ok := queued[c.path]; !ok {
			continue
		}
		if len(kept) >= capN {
			break
		}
		kept = append(kept, c)
		byPath[c.path] = c
	}

	// Fill free slots with the next unshown queue paths.
	newPaths := make([]string, 0)
	for _, p := range ui.sorter.Queue {
		if len(kept) >= capN {
			break
		}
		if _, shown := byPath[p]; shown {
			continue
		}
		card := newSwipeCard(p, ui.onSwipeCard)
		kept = append(kept, card)
		byPath[p] = card
		newPaths = append(newPaths, p)
	}

	ui.swipeMu.Lock()
	ui.swipeCardByPath = byPath
	ui.swipeGen++
	gen := ui.swipeGen
	ui.swipeMu.Unlock()
	ui.swipeBoard.SetCards(kept)
	ui.updateSwipeEmptyHint(len(kept))

	// Prefetch board + upcoming queue; halt anything no longer interesting.
	ui.prefetchSwipeThumbs()

	// Only kick UI apply for brand-new cards (survivors already have thumbs).
	for _, p := range newPaths {
		ui.loadSwipeThumb(p, gen)
	}
}

// replaceSwipeSlot puts the next unseen queue PDF into the slot that just
// decided, without recreating or re-rendering neighbor cards.
func (ui *App) replaceSwipeSlot(oldPath string) {
	if ui.swipeBoard == nil {
		return
	}
	ui.swipeMu.Lock()
	card := ui.swipeCardByPath[oldPath]
	if card == nil {
		ui.swipeMu.Unlock()
		// Card already gone (e.g. race) — fall back to a full sync.
		ui.syncSwipeBoard()
		return
	}
	delete(ui.swipeCardByPath, oldPath)
	ui.swipeMu.Unlock()

	next := ui.nextPathNotOnBoard()
	if next == "" {
		ui.swipeBoard.RemoveCard(card)
		ui.updateSwipeEmptyHint(len(ui.swipeBoard.Cards()))
		return
	}

	// Rebind in place: same widget, same slot, same neighbors.
	card.BindPath(next)
	ui.swipeMu.Lock()
	ui.swipeCardByPath[next] = card
	ui.swipeMu.Unlock()
	// Refresh interest set (halt dropped path work, prefetch next lookahead).
	ui.prefetchSwipeThumbs()
	// Do not bump swipeGen — other cards' in-flight thumbs must still land.
	// Prefer cache hit (often already prefetched) before spawning work.
	ui.loadSwipeThumb(next, 0)
}

// nextPathNotOnBoard returns the first queued PDF not already shown as a card.
func (ui *App) nextPathNotOnBoard() string {
	ui.swipeMu.Lock()
	on := make(map[string]struct{}, len(ui.swipeCardByPath))
	for p := range ui.swipeCardByPath {
		on[p] = struct{}{}
	}
	ui.swipeMu.Unlock()
	// Also treat live card paths as shown (map may lag one tick during replace).
	for _, c := range ui.swipeBoard.Cards() {
		on[c.path] = struct{}{}
	}
	for _, p := range ui.sorter.Queue {
		if _, shown := on[p]; !shown {
			return p
		}
	}
	return ""
}

func (ui *App) updateSwipeEmptyHint(cardCount int) {
	if cardCount > 0 {
		return
	}
	switch {
	case !ui.sorter.HasFolder():
		ui.swipeBoard.emptyHint.SetText("Open a folder of PDFs to begin swiping.")
	case ui.sorter.TotalStarted == 0:
		ui.swipeBoard.emptyHint.SetText("No PDFs in this folder.")
	default:
		ui.swipeBoard.emptyHint.SetText("Queue empty — all assigned or skipped.")
	}
}

// prefetchSwipeThumbs declares the interest set for the thumbnail cache:
// currently visible board paths plus a lookahead of upcoming queue items.
// Want() cancels pdftoppm for anything outside that set and starts missing
// renders in the background.
func (ui *App) prefetchSwipeThumbs() {
	if ui.thumbs == nil {
		return
	}
	capN := 1
	if ui.swipeBoard != nil {
		capN = ui.swipeBoard.Capacity()
	}
	// Lookahead: roughly another boardful, capped so we don't thrash workers.
	lookahead := capN * 2
	if lookahead < 4 {
		lookahead = 4
	}
	if lookahead > 16 {
		lookahead = 16
	}

	ui.swipeMu.Lock()
	seen := make(map[string]struct{}, len(ui.swipeCardByPath)+lookahead)
	want := make([]string, 0, len(ui.swipeCardByPath)+lookahead)
	for p := range ui.swipeCardByPath {
		if p == "" {
			continue
		}
		seen[p] = struct{}{}
		want = append(want, p)
	}
	ui.swipeMu.Unlock()

	extra := 0
	for _, p := range ui.sorter.Queue {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		want = append(want, p)
		extra++
		if extra >= lookahead {
			break
		}
	}
	ui.thumbs.Want(want, preview.SwipeThumbDPI, 1)
}

func (ui *App) loadSwipeThumb(path string, gen uint64) {
	if path == "" {
		return
	}
	// Fast path: already cached (including from prefetch) → paint now.
	if ui.thumbs != nil {
		if img, ok := ui.thumbs.Get(path, preview.SwipeThumbDPI); ok {
			ui.applySwipeThumb(path, gen, img, nil)
			return
		}
	}

	ui.swipeWG.Add(1)
	deliver := func(img image.Image, err error) {
		fyne.Do(func() {
			defer ui.swipeWG.Done()
			ui.applySwipeThumb(path, gen, img, err)
		})
	}
	if ui.thumbs != nil {
		ui.thumbs.GetAsync(path, preview.SwipeThumbDPI, deliver)
		return
	}
	// Fallback without cache (shouldn't happen in production New()).
	go func() {
		img, err := preview.RenderFirstPage(path, preview.SwipeThumbDPI)
		deliver(img, err)
	}()
}

// applySwipeThumb paints a raster onto the card for path if that card is still
// bound and the generation is current.
func (ui *App) applySwipeThumb(path string, gen uint64, img image.Image, err error) {
	ui.swipeMu.Lock()
	card := ui.swipeCardByPath[path]
	curGen := ui.swipeGen
	var cardPath string
	if card != nil {
		cardPath = card.path
	}
	ui.swipeMu.Unlock()
	// Drop stale work: card rebound/removed, or a full sync invalidated
	// this generation (gen==0 means "path identity only", used by
	// in-place replace so neighbor renders are never cancelled).
	if card == nil || cardPath != path || err != nil || img == nil {
		return
	}
	if gen != 0 && gen != curGen {
		return
	}
	card.SetThumbnail(img)
}

// onSwipeCard handles a committed drag (or keyboard stand-in) on one card.
func (ui *App) onSwipeCard(path string, dir swipeDir) {
	switch dir {
	case swipeLeft:
		if ui.swipeLeftCat == "" {
			ui.setStatus("Pick a left category first.")
			return
		}
		ui.assignPath(path, ui.swipeLeftCat)
	case swipeRight:
		if ui.swipeRightCat == "" {
			ui.setStatus("Pick a right category first.")
			return
		}
		ui.assignPath(path, ui.swipeRightCat)
	case swipeDown:
		ui.skipPath(path)
	}
}

// waitForSwipeThumbs blocks until in-flight swipe thumbnails finish applying
// (tests only — same rationale as waitForPreview).
func (ui *App) waitForSwipeThumbs() {
	ui.swipeWG.Wait()
}

// prefetchReviewPreviews marks the current PDF and the next few queue items
// as wanted at review DPI/page count so assign→next often hits the cache.
func (ui *App) prefetchReviewPreviews() {
	if ui.thumbs == nil {
		return
	}
	maxPages := len(ui.pageBtns)
	if maxPages < 1 {
		maxPages = preview.ReviewMaxPages
	}
	// Current + up to 2 ahead (queue[0] is current).
	n := 3
	if n > len(ui.sorter.Queue) {
		n = len(ui.sorter.Queue)
	}
	want := append([]string(nil), ui.sorter.Queue[:n]...)
	ui.thumbs.Want(want, preview.ReviewPreviewDPI, maxPages)
}

func (ui *App) showPreview(pdfPath string) {
	ui.previewMu.Lock()
	ui.previewGen++
	gen := ui.previewGen
	ui.previewMu.Unlock()

	// canvas.Text is single-line; keep messages flat for the test driver + real UI.
	ui.placeholder.Text = "Rendering preview…"
	ui.placeholder.Refresh()
	ui.placeholder.Show()
	ui.emptyIcon.Hide()
	ui.previewImg.Hide()
	ui.hidePager() // don't leave the previous file's pager lingering

	path := pdfPath
	maxPages := len(ui.pageBtns)
	if maxPages < 1 {
		maxPages = preview.ReviewMaxPages
	}
	dpi := preview.ReviewPreviewDPI

	// Fast path: already cached (including from prefetch of the previous file).
	if ui.thumbs != nil {
		if pages, ok := ui.thumbs.GetPages(path, dpi, maxPages); ok {
			ui.applyReviewPreview(gen, pages, nil)
			return
		}
	}

	ui.previewWG.Add(1)
	deliver := func(pages []image.Image, err error) {
		// fyne.Do is the supported way to touch widgets off the main thread (Fyne ≥ 2.6).
		fyne.Do(func() {
			// Done must fire from inside this callback, not right after the
			// goroutine's synchronous portion: fyne.Do only *schedules* this
			// closure (the test driver runs it on its own goroutine, not
			// serialized with the caller), so waitForPreview() needs to
			// block until the callback itself — the part that actually
			// touches shared widgets/state — has finished.
			defer ui.previewWG.Done()
			ui.applyReviewPreview(gen, pages, err)
		})
	}

	if ui.thumbs != nil {
		ui.thumbs.GetPagesAsync(path, dpi, maxPages, deliver)
		return
	}
	// Fallback without cache.
	go func() {
		pages, err := preview.RenderPages(path, dpi, maxPages)
		deliver(pages, err)
	}()
}

// applyReviewPreview paints multi-page preview results if gen is still current.
func (ui *App) applyReviewPreview(gen uint64, pages []image.Image, err error) {
	ui.previewMu.Lock()
	currentGen := ui.previewGen
	ui.previewMu.Unlock()
	if gen != currentGen {
		// previewGen increments exactly once per display-state change
		// (every showPreview call, and refresh()'s empty-current branch),
		// strictly before this goroutine could observe the change — so
		// this mutex-guarded counter alone is sufficient to detect a
		// stale render. A direct ui.sorter.Current() read here would be
		// redundant *and* unsynchronized: this callback runs via
		// fyne.Do on a separate goroutine that is not actually
		// serialized with the caller in the test driver (only the
		// real GLFW driver runs both on one thread), so it can race
		// with a concurrent sorter mutation (assign/skip/undo).
		return
	}
	if err != nil {
		ui.previewImg.Hide()
		// Flatten error text: canvas.Text does not layout newlines reliably,
		// and some error strings upset the software font pipeline in tests.
		// Keep the total (with the "Cannot preview: " prefix) short enough
		// that it doesn't overflow the preview card at typical window widths —
		// canvas.Text has no wrapping, so this is a hard character budget, not
		// just cosmetic.
		msg := strings.ReplaceAll(err.Error(), "\n", " ")
		const maxMsgLen = 55
		if len(msg) > maxMsgLen {
			msg = msg[:maxMsgLen-3] + "..."
		}
		ui.placeholder.Text = "Cannot preview: " + msg
		ui.placeholder.Refresh()
		ui.placeholder.Show()
		ui.emptyIcon.Resource = theme.NewColoredResource(theme.WarningIcon(), theme.ColorNamePlaceHolder)
		ui.emptyIcon.Refresh()
		ui.emptyIcon.Show()
		return
	}
	ui.placeholder.Hide()
	ui.emptyIcon.Hide()
	ui.setPages(pages)
}

// setPages stores freshly rendered pages and shows page 1. The pager (up to
// 3 number buttons) only appears when there's more than one page — "when
// available", per the multi-page preview request; a single-page document
// looks exactly as it did before this feature existed.
func (ui *App) setPages(pages []image.Image) {
	// Set ui.pages before touching any pager buttons: showPage(0) below
	// needs it, and hidePager() (used elsewhere to fully reset) clears it —
	// don't call that here or it wipes the very pages we're about to show.
	ui.pages = pages
	showPager := len(pages) > 1
	for i, b := range ui.pageBtns {
		if showPager && i < len(pages) {
			b.Show()
		} else {
			b.Hide()
		}
	}
	if root := ui.win.Content(); root != nil {
		// Same reasoning as rebuildCategoryButtons: showing/hiding pager
		// buttons changes the row's height, and ancestors (the preview
		// card's Border layout) won't re-layout on their own just because a
		// descendant's Refresh() ran.
		root.Refresh()
	}
	ui.showPage(0)
}

// showPage displays page index i (already rendered, in ui.pages) and marks
// the corresponding pager button as the selected one.
func (ui *App) showPage(i int) {
	if i < 0 || i >= len(ui.pages) {
		return
	}
	ui.pageIndex = i
	ui.previewImg.Image = ui.pages[i]
	ui.previewImg.Refresh()
	ui.previewImg.Show()
	ui.previewStack.Refresh()

	for idx, b := range ui.pageBtns {
		if idx == i {
			b.Importance = widget.HighImportance
		} else {
			b.Importance = widget.MediumImportance
		}
		b.Refresh()
	}
}

// hidePager clears any rendered pages and hides all pager buttons — called
// whenever the preview area stops showing a rendered page (a new render
// starting, an error, or the empty state), so a previous file's pager never
// lingers over the wrong content.
func (ui *App) hidePager() {
	ui.pages = nil
	ui.pageIndex = 0
	for _, b := range ui.pageBtns {
		b.Hide()
	}
	if root := ui.win.Content(); root != nil {
		root.Refresh()
	}
}

// waitForPreview blocks until any in-flight preview render has finished
// applying its result. Production code never calls this (rendering is meant
// to stay off the UI-blocking path); it exists for tests, which otherwise
// race the background render against whatever they do immediately after
// triggering it (assign/skip/undo, another refresh, …). In the real GLFW
// driver fyne.Do callbacks and UI-event callbacks all run on one thread, so
// this ordering is naturally serialized — only the test driver needs help.
func (ui *App) waitForPreview() {
	ui.previewWG.Wait()
}

func (ui *App) setStatus(msg string) {
	if ui.statusLabel != nil {
		ui.statusLabel.SetText(msg)
	}
	if ui.swipeStatus != nil {
		ui.swipeStatus.SetText(msg)
	}
}
