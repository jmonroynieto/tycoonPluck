package ui

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"tycoonPluck/internal/categories"
)

var (
	keyEvent1 = &fyne.KeyEvent{Name: fyne.Key1}
	keyEventS = &fyne.KeyEvent{Name: fyne.KeyS}
	keyEventZ = &fyne.KeyEvent{Name: fyne.KeyZ}
)

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatalf("touch %s: %v", path, err)
	}
}

// newTestApp isolates categories + session journal so tests never touch real paths.
func newTestApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	a := test.NewTempApp(t)
	return New(a)
}

// withCategory seeds one category (save + rebuild) for assign/keyboard tests.
func withCategory(t *testing.T, ui *App, name string) {
	t.Helper()
	next := append(append([]string(nil), ui.cats...), name)
	if err := categories.Save(next); err != nil {
		t.Fatal(err)
	}
	ui.cats = next
	ui.rebuildCategoryButtons()
}

func TestNewStartsWithNoPreloadedCategories(t *testing.T) {
	ui := newTestApp(t)

	if len(ui.cats) != 0 {
		t.Fatalf("cats = %v, want empty on first run", ui.cats)
	}
	// Sidebar shows a hint label, not category buttons.
	if len(ui.categoryBox.Objects) != 1 {
		t.Fatalf("categoryBox objects = %d, want 1 hint", len(ui.categoryBox.Objects))
	}
	if _, ok := ui.categoryBox.Objects[0].(*categoryRow); ok {
		t.Error("did not expect a category row with empty cats")
	}
}

func TestRebuildCategoryButtonsNumbersOnlyFirstNine(t *testing.T) {
	ui := newTestApp(t)
	ui.cats = []string{
		"C1", "C2", "C3", "C4", "C5", "C6", "C7", "C8", "C9", "C10",
	}
	ui.rebuildCategoryButtons()

	if len(ui.categoryBox.Objects) != 10 {
		t.Fatalf("got %d buttons, want 10", len(ui.categoryBox.Objects))
	}
	for i, obj := range ui.categoryBox.Objects {
		row := obj.(*categoryRow)
		btn := row.nameBtn
		wantNumbered := i < 9
		hasPrefix := strings.HasPrefix(btn.Text, strconv.Itoa(i+1)+". ")
		if wantNumbered && !hasPrefix {
			t.Errorf("button %d text %q should have numeric prefix %d.", i, btn.Text, i+1)
		}
		if !wantNumbered && hasPrefix {
			t.Errorf("button %d (10th+) text %q should not have a numeric prefix", i, btn.Text)
		}
	}
}

func TestRefreshDisablesCategoryAndSkipButtonsWhenNoFolderOpen(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	ui.refresh()
	ui.waitForPreview()

	for i, obj := range ui.categoryBox.Objects {
		row, ok := obj.(*categoryRow)
		if !ok {
			t.Fatalf("categoryBox.Objects[%d] is not a category row", i)
		}
		btn := row.nameBtn
		if !btn.Disabled() {
			t.Errorf("category button %d should be disabled with no folder open", i)
		}
	}
	if !ui.skipBtn.Disabled() {
		t.Error("skip button should be disabled with no folder open")
	}
	if !ui.undoBtn.Disabled() {
		t.Error("undo button should be disabled with nothing to undo")
	}
	if ui.progressLabel.Text != "0 / 0" {
		t.Errorf("progressLabel = %q, want \"0 / 0\"", ui.progressLabel.Text)
	}
	if ui.outputHint.Text != "Output: open a folder" {
		t.Errorf("outputHint = %q", ui.outputHint.Text)
	}
}

func TestRefreshEnablesButtonsWhenQueueHasCurrent(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))

	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()

	for i, obj := range ui.categoryBox.Objects {
		row, ok := obj.(*categoryRow)
		if !ok {
			t.Fatalf("categoryBox.Objects[%d] is not a category row", i)
		}
		btn := row.nameBtn
		if btn.Disabled() {
			t.Errorf("category button %d should be enabled with a current PDF", i)
		}
	}
	if ui.skipBtn.Disabled() {
		t.Error("skip button should be enabled with a current PDF")
	}
	if ui.filenameLabel.Text != "a.pdf" {
		t.Errorf("filenameLabel = %q, want \"a.pdf\"", ui.filenameLabel.Text)
	}
	if ui.progressLabel.Text != "1 / 1" {
		t.Errorf("progressLabel = %q, want \"1 / 1\"", ui.progressLabel.Text)
	}
}

func TestRefreshShowsEmptyFolderMessage(t *testing.T) {
	ui := newTestApp(t)
	dir := t.TempDir() // no PDFs inside

	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()

	if ui.placeholder.Text != "No PDFs in this folder." {
		t.Errorf("placeholder = %q", ui.placeholder.Text)
	}
}

func TestRefreshShowsQueueEmptyMessageAfterAllAssigned(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))

	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := ui.sorter.Assign("Work"); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()

	if ui.placeholder.Text != "Queue empty — all assigned or skipped." {
		t.Errorf("placeholder = %q", ui.placeholder.Text)
	}
	for i, obj := range ui.categoryBox.Objects {
		row, ok := obj.(*categoryRow)
		if !ok {
			continue
		}
		if !row.nameBtn.Disabled() {
			t.Errorf("category button %d should be disabled once queue is empty", i)
		}
	}
	if ui.undoBtn.Disabled() {
		t.Error("undo button should be enabled after an assignment")
	}
}

func TestAssignMovesFileAndAdvancesUI(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.pdf"))

	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()

	first := ui.filenameLabel.Text
	ui.assign("Work")

	if _, err := os.Stat(filepath.Join(dir, "Work", first)); err != nil {
		t.Errorf("expected %s moved into Work/: %v", first, err)
	}
	if ui.filenameLabel.Text == "" || ui.filenameLabel.Text == first {
		t.Errorf("filenameLabel = %q after advancing, want the other PDF", ui.filenameLabel.Text)
	}
	if !strings.Contains(ui.statusLabel.Text, "Work") {
		t.Errorf("statusLabel = %q, expected it to mention the category", ui.statusLabel.Text)
	}
}

func TestAssignIsNoOpWhenNothingCurrent(t *testing.T) {
	ui := newTestApp(t)
	ui.refresh()
	ui.waitForPreview()

	// No folder open, no current file: must not panic and must not touch
	// ui.sorter.Assign (which would error and populate an error dialog).
	ui.assign("Work")

	if ui.statusLabel.Text != "" {
		t.Errorf("statusLabel = %q, want unchanged/empty", ui.statusLabel.Text)
	}
}

func TestOnSkipAdvancesQueueAndLeavesFileInPlace(t *testing.T) {
	ui := newTestApp(t)
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.pdf"))

	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()

	first := ui.filenameLabel.Text
	ui.onSkip()

	if _, err := os.Stat(filepath.Join(dir, first)); err != nil {
		t.Error("skipped file should remain on disk")
	}
	if ui.filenameLabel.Text == "" || ui.filenameLabel.Text == first {
		t.Errorf("filenameLabel = %q after skip, want the other PDF", ui.filenameLabel.Text)
	}
}

func TestOnUndoRestoresFileAndUpdatesStatus(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))

	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()
	ui.assign("Work")

	ui.onUndo()

	if _, err := os.Stat(filepath.Join(dir, "a.pdf")); err != nil {
		t.Error("undo should restore the file to the source dir")
	}
	if !strings.Contains(ui.statusLabel.Text, "Undid") {
		t.Errorf("statusLabel = %q, expected an undo confirmation", ui.statusLabel.Text)
	}
	if ui.undoBtn.Disabled() != true {
		t.Error("undo button should be disabled again once the stack is exhausted")
	}
}

func TestOnUndoWithNothingToUndoSetsStatusWithoutError(t *testing.T) {
	ui := newTestApp(t)
	ui.refresh()
	ui.waitForPreview()

	ui.onUndo()

	if ui.statusLabel.Text != "Nothing to undo." {
		t.Errorf("statusLabel = %q, want \"Nothing to undo.\"", ui.statusLabel.Text)
	}
}

// --- keyboard shortcuts ---

func TestOnKeyDigitAssignsToMatchingCategory(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))

	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()

	firstCategory := ui.cats[0]
	ui.onKey(keyEvent1)

	if _, err := os.Stat(filepath.Join(dir, firstCategory, "a.pdf")); err != nil {
		t.Errorf("expected file assigned to first category %q via key '1': %v", firstCategory, err)
	}
}

func TestOnKeyDigitDoesNothingWithoutCurrentFile(t *testing.T) {
	ui := newTestApp(t)
	ui.refresh() // no folder open

	ui.onKey(keyEvent1)

	// Nothing to assert on disk; just confirm it didn't panic and status
	// stayed empty (no assign attempted).
	if ui.statusLabel.Text != "" {
		t.Errorf("statusLabel = %q, want empty (no-op)", ui.statusLabel.Text)
	}
}

func TestOnKeySSkipsCurrentFile(t *testing.T) {
	ui := newTestApp(t)
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.pdf"))

	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()

	first := ui.filenameLabel.Text
	ui.onKey(keyEventS)

	if ui.filenameLabel.Text == "" || ui.filenameLabel.Text == first {
		t.Errorf("filenameLabel = %q after 'S' skip, want the other PDF", ui.filenameLabel.Text)
	}
}

func TestOnKeyZUndoesOnlyWhenPossible(t *testing.T) {
	ui := newTestApp(t)
	ui.refresh()
	ui.waitForPreview()

	// Nothing assigned yet: 'Z' must be a no-op (CanUndo() guards it).
	ui.onKey(keyEventZ)
	if ui.statusLabel.Text != "" {
		t.Errorf("statusLabel = %q, want empty before anything is undoable", ui.statusLabel.Text)
	}

	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	withCategory(t, ui, "Work")
	ui.refresh()
	ui.waitForPreview()
	ui.assign(ui.cats[0])

	ui.onKey(keyEventZ)
	if _, err := os.Stat(filepath.Join(dir, "a.pdf")); err != nil {
		t.Error("'Z' should have undone the assignment")
	}
}

func TestSessionJournalRecordsAssign(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()
	ui.assign("Work")

	entries := ui.journal.UndoEntriesStillOnDisk(dir)
	if len(entries) != 1 {
		t.Fatalf("session journal entries = %d, want 1", len(entries))
	}
	if entries[0].Category != "Work" {
		t.Errorf("category = %q", entries[0].Category)
	}
}

func TestOnKeyIgnoredWhileModalOpen(t *testing.T) {
	ui := newTestApp(t)
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.pdf"))

	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()

	// OpenFolder shuffles the queue (deliberately — see sorter.OpenFolder),
	// so don't assume which of the two files sorts first; just capture it.
	first := ui.filenameLabel.Text
	if first != "a.pdf" && first != "b.pdf" {
		t.Fatalf("filenameLabel = %q, want a.pdf or b.pdf", first)
	}

	ui.modalOpen = true
	ui.onKey(keyEvent1)
	ui.onKey(keyEventS)
	ui.onKey(keyEventZ)

	// File must still be current; nothing assigned or skipped under a modal.
	if ui.filenameLabel.Text != first {
		t.Errorf("filenameLabel = %q, want %q (keys ignored while modalOpen)", ui.filenameLabel.Text, first)
	}
	if _, err := os.Stat(filepath.Join(dir, first)); err != nil {
		t.Errorf("%s should still be in the source folder", first)
	}
	if ui.sorter.Remaining() != 2 {
		t.Errorf("Remaining() = %d, want 2", ui.sorter.Remaining())
	}

	ui.modalOpen = false
	ui.onKey(keyEventS)
	if ui.filenameLabel.Text == "" || ui.filenameLabel.Text == first {
		t.Errorf("filenameLabel = %q after modal closed and S pressed, want the other PDF", ui.filenameLabel.Text)
	}
}

// --- Open PDF (inspect) wiring ---

func TestOnInspectOpensCurrentFileAndSetsStatus(t *testing.T) {
	ui := newTestApp(t)
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()

	var gotPath string
	ui.openFile = func(p string) error {
		gotPath = p
		return nil
	}

	ui.onInspect()

	want := filepath.Join(dir, "a.pdf")
	if gotPath != want {
		t.Errorf("openFile called with %q, want %q", gotPath, want)
	}
	if !strings.Contains(ui.statusLabel.Text, "a.pdf") {
		t.Errorf("statusLabel = %q, expected it to mention the opened file", ui.statusLabel.Text)
	}
}

func TestOnInspectNoOpWhenNothingCurrent(t *testing.T) {
	ui := newTestApp(t)
	ui.refresh()
	ui.waitForPreview()

	called := false
	ui.openFile = func(string) error {
		called = true
		return nil
	}

	ui.onInspect()

	if called {
		t.Error("openFile should not be called with no current PDF")
	}
	if ui.statusLabel.Text != "" {
		t.Errorf("statusLabel = %q, want empty (no-op)", ui.statusLabel.Text)
	}
}

func TestOnInspectLeavesStatusUnchangedOnError(t *testing.T) {
	ui := newTestApp(t)
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()
	ui.setStatus("before")

	ui.openFile = func(string) error { return fmt.Errorf("no viewer registered") }
	ui.onInspect()

	// onInspect shows an error dialog on failure; it must not also claim
	// success by overwriting statusLabel with an "Opened in system viewer" message.
	if ui.statusLabel.Text != "before" {
		t.Errorf("statusLabel = %q, want unchanged \"before\"", ui.statusLabel.Text)
	}
}

func TestRefreshTogglesInspectButtonWithCurrent(t *testing.T) {
	ui := newTestApp(t)
	ui.refresh()
	ui.waitForPreview()
	if !ui.inspectBtn.Disabled() {
		t.Error("inspect button should be disabled with no current PDF")
	}

	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()
	if ui.inspectBtn.Disabled() {
		t.Error("inspect button should be enabled with a current PDF")
	}
}

func TestOnKeyOTriggersInspect(t *testing.T) {
	ui := newTestApp(t)
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()

	called := false
	ui.openFile = func(string) error {
		called = true
		return nil
	}

	ui.onKey(&fyne.KeyEvent{Name: fyne.KeyO})

	if !called {
		t.Error("'O' key should trigger openFile for the current PDF")
	}
}

func TestOnKeyODoesNothingWithoutCurrentFile(t *testing.T) {
	ui := newTestApp(t)
	ui.refresh()
	ui.waitForPreview()

	called := false
	ui.openFile = func(string) error {
		called = true
		return nil
	}

	ui.onKey(&fyne.KeyEvent{Name: fyne.KeyO})

	if called {
		t.Error("'O' key should be a no-op with no current PDF")
	}
}

// --- category delisting ---

func TestCategoryRowDeleteButtonStartsHidden(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")

	row, ok := ui.categoryBox.Objects[0].(*categoryRow)
	if !ok {
		t.Fatalf("categoryBox.Objects[0] is not a *categoryRow")
	}
	if row.deleteBtn.Visible() {
		t.Error("delete button should start hidden")
	}
}

func TestCategoryRowHoverTogglesDeleteButton(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	row := ui.categoryBox.Objects[0].(*categoryRow)

	row.MouseIn(nil)
	if !row.deleteBtn.Visible() {
		t.Error("delete button should be visible after MouseIn")
	}

	row.MouseOut()
	if row.deleteBtn.Visible() {
		t.Error("delete button should be hidden again after MouseOut")
	}
}

// widget.Button is itself desktop.Hoverable. Fyne delivers hover only to the
// deepest Hoverable under the pointer, so in real use the name/delete buttons
// receive MouseIn — not the row. The row must learn about hover via the
// child buttons; calling row.MouseIn alone would green-pass a broken UI.
func TestCategoryRowHoverViaNameButtonShowsDelete(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	row := ui.categoryBox.Objects[0].(*categoryRow)

	row.nameBtn.MouseIn(nil)
	if !row.deleteBtn.Visible() {
		t.Error("delete button should appear when the name button is hovered")
	}

	row.nameBtn.MouseOut()
	if row.deleteBtn.Visible() {
		t.Error("delete button should hide when the pointer leaves the name button")
	}
}

// Moving from the name button onto the × fires MouseOut on the name button
// before MouseIn on the delete button (same processMouseMoved). A naive
// hide-on-MouseOut would remove the × under the cursor and make it
// unclickable. Refcounting keeps it visible across the sibling handoff.
func TestCategoryRowHoverSurvivesMoveFromNameToDelete(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	row := ui.categoryBox.Objects[0].(*categoryRow)

	row.nameBtn.MouseIn(nil)
	if !row.deleteBtn.Visible() {
		t.Fatal("setup: delete should be visible after hovering the name")
	}

	// Order matches the desktop driver: leave old hoverable, then enter new.
	row.nameBtn.MouseOut()
	row.deleteBtn.MouseIn(nil)

	if !row.deleteBtn.Visible() {
		t.Error("delete button must stay visible when moving from name → ×")
	}
	if row.hoverCount != 1 {
		t.Errorf("hoverCount = %d, want 1 after name→delete handoff", row.hoverCount)
	}

	row.deleteBtn.MouseOut()
	if row.deleteBtn.Visible() {
		t.Error("delete button should hide after leaving the ×")
	}
}

func TestDeleteCategoryRemovesAndPersists(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	withCategory(t, ui, "Personal")

	ui.deleteCategory("Work")

	if len(ui.cats) != 1 || ui.cats[0] != "Personal" {
		t.Errorf("cats = %v, want [Personal]", ui.cats)
	}
	loaded := categories.Load()
	for _, c := range loaded {
		if c == "Work" {
			t.Errorf("Load() = %v, \"Work\" should have been removed from disk", loaded)
		}
	}
	if len(ui.categoryBox.Objects) != 1 {
		t.Errorf("categoryBox has %d objects, want 1 after delete", len(ui.categoryBox.Objects))
	}
}

func TestDeleteCategoryDoesNotTouchFilesOnDisk(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()
	if _, err := ui.sorter.Assign("Work"); err != nil {
		t.Fatal(err)
	}
	ui.refresh()
	ui.waitForPreview()

	assignedPath := filepath.Join(dir, "Work", "a.pdf")
	if _, err := os.Stat(assignedPath); err != nil {
		t.Fatalf("setup: expected a.pdf under Work/: %v", err)
	}

	ui.deleteCategory("Work")

	if _, err := os.Stat(assignedPath); err != nil {
		t.Errorf("Work/a.pdf should still exist on disk after delisting the category: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Work")); err != nil {
		t.Errorf("Work/ folder should still exist on disk after delisting the category: %v", err)
	}
}

func TestDeleteCategoryLastOneShowsHintAgain(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")

	ui.deleteCategory("Work")

	if len(ui.cats) != 0 {
		t.Errorf("cats = %v, want empty", ui.cats)
	}
	if len(ui.categoryBox.Objects) != 1 {
		t.Fatalf("categoryBox objects = %d, want 1 hint", len(ui.categoryBox.Objects))
	}
	if _, ok := ui.categoryBox.Objects[0].(*categoryRow); ok {
		t.Error("did not expect a category row after deleting the last category")
	}
}

func TestDeleteCategoryUnknownNameIsNoOp(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")

	ui.deleteCategory("DoesNotExist")

	if len(ui.cats) != 1 || ui.cats[0] != "Work" {
		t.Errorf("cats = %v, want unchanged [Work]", ui.cats)
	}
}

// --- multi-page pager ---

func fakePage() image.Image {
	return image.NewRGBA(image.Rect(0, 0, 10, 10))
}

func TestSetPagesSinglePageHidesPager(t *testing.T) {
	ui := newTestApp(t)
	only := fakePage()

	ui.setPages([]image.Image{only})

	for i, b := range ui.pageBtns {
		if b.Visible() {
			t.Errorf("pageBtns[%d] should be hidden for a single-page document", i)
		}
	}
	if ui.previewImg.Image != only {
		t.Error("previewImg should show the single page that was set")
	}
}

func TestSetPagesMultiPageShowsOnlyMatchingButtons(t *testing.T) {
	ui := newTestApp(t)

	ui.setPages([]image.Image{fakePage(), fakePage()})

	if !ui.pageBtns[0].Visible() || !ui.pageBtns[1].Visible() {
		t.Error("pageBtns[0] and [1] should be visible for a 2-page document")
	}
	if ui.pageBtns[2].Visible() {
		t.Error("pageBtns[2] should stay hidden for a 2-page document")
	}
}

func TestSetPagesThreePagesShowsAllButtons(t *testing.T) {
	ui := newTestApp(t)

	ui.setPages([]image.Image{fakePage(), fakePage(), fakePage()})

	for i, b := range ui.pageBtns {
		if !b.Visible() {
			t.Errorf("pageBtns[%d] should be visible for a 3-page document", i)
		}
	}
}

func TestSetPagesShowsFirstPageAndMarksItSelected(t *testing.T) {
	ui := newTestApp(t)
	first := fakePage()

	ui.setPages([]image.Image{first, fakePage(), fakePage()})

	if ui.previewImg.Image != first {
		t.Error("previewImg should show page 1 initially")
	}
	if ui.pageBtns[0].Importance != widget.HighImportance {
		t.Error("page 1 button should be marked selected (HighImportance)")
	}
	if ui.pageBtns[1].Importance == widget.HighImportance {
		t.Error("page 2 button should not be marked selected")
	}
}

func TestShowPageSwitchesImageAndSelection(t *testing.T) {
	ui := newTestApp(t)
	p1, p2, p3 := fakePage(), fakePage(), fakePage()
	ui.setPages([]image.Image{p1, p2, p3})

	ui.showPage(1)

	if ui.previewImg.Image != p2 {
		t.Error("previewImg should show page 2 after showPage(1)")
	}
	if ui.pageBtns[1].Importance != widget.HighImportance {
		t.Error("page 2 button should now be marked selected")
	}
	if ui.pageBtns[0].Importance == widget.HighImportance {
		t.Error("page 1 button should no longer be marked selected")
	}
}

func TestShowPageOutOfRangeIsNoOp(t *testing.T) {
	ui := newTestApp(t)
	p1 := fakePage()
	ui.setPages([]image.Image{p1})

	ui.showPage(5)
	ui.showPage(-1)

	if ui.previewImg.Image != p1 {
		t.Error("out-of-range showPage calls should not change the displayed image")
	}
}

func TestHidePagerClearsPagesAndHidesButtons(t *testing.T) {
	ui := newTestApp(t)
	ui.setPages([]image.Image{fakePage(), fakePage(), fakePage()})

	ui.hidePager()

	if len(ui.pages) != 0 {
		t.Errorf("pages = %v, want empty after hidePager", ui.pages)
	}
	for i, b := range ui.pageBtns {
		if b.Visible() {
			t.Errorf("pageBtns[%d] should be hidden after hidePager", i)
		}
	}
}

func TestRefreshToEmptyStateHidesPager(t *testing.T) {
	ui := newTestApp(t)
	ui.setPages([]image.Image{fakePage(), fakePage()})

	ui.refresh() // no folder open -> empty-current branch
	ui.waitForPreview()

	for i, b := range ui.pageBtns {
		if b.Visible() {
			t.Errorf("pageBtns[%d] should be hidden once refresh() shows the empty state", i)
		}
	}
}

func TestAddCategoryPersistsOnlyAfterSuccessfulSave(t *testing.T) {
	// Drive the save-then-mutate path by calling the same logic the form uses.
	ui := newTestApp(t)
	before := len(ui.cats)
	clean := "BrandNewCat"

	next := append(append([]string(nil), ui.cats...), clean)
	if err := categories.Save(next); err != nil {
		t.Fatal(err)
	}
	ui.cats = next
	ui.rebuildCategoryButtons()

	if len(ui.cats) != before+1 {
		t.Fatalf("cats len = %d, want %d", len(ui.cats), before+1)
	}
	// Reload from disk — must include the new name (config was written first).
	loaded := categories.Load()
	found := false
	for _, c := range loaded {
		if c == clean {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Load() = %v, missing %q after save-then-mutate", loaded, clean)
	}
}

// --- swipe mode ---

func TestSwipeCapacityGrowsWithWindow(t *testing.T) {
	smallN, smallCols := swipeCapacity(fyne.NewSize(200, 220))
	if smallN < 1 || smallCols < 1 {
		t.Fatalf("small capacity = %d cols=%d, want ≥1", smallN, smallCols)
	}
	bigN, bigCols := swipeCapacity(fyne.NewSize(900, 700))
	if bigN <= smallN {
		t.Errorf("large window capacity %d should exceed small %d", bigN, smallN)
	}
	if bigCols < smallCols {
		t.Errorf("large cols %d should be ≥ small cols %d", bigCols, smallCols)
	}
	capped, _ := swipeCapacity(fyne.NewSize(4000, 4000))
	if capped > swipeMaxCards {
		t.Errorf("capacity %d exceeds cap %d", capped, swipeMaxCards)
	}
}

func TestSwipeCardCommitDirections(t *testing.T) {
	var got swipeDir
	c := newSwipeCard("/tmp/a.pdf", func(_ string, d swipeDir) { got = d })

	c.dragX, c.dragY = -swipeCommitX-1, 0
	if d := c.commitDir(); d != swipeLeft {
		t.Errorf("left commit = %v, want swipeLeft", d)
	}
	c.dragX, c.dragY = swipeCommitX+1, 0
	if d := c.commitDir(); d != swipeRight {
		t.Errorf("right commit = %v, want swipeRight", d)
	}
	c.dragX, c.dragY = 0, swipeCommitY+1
	if d := c.commitDir(); d != swipeDown {
		t.Errorf("down commit = %v, want swipeDown", d)
	}
	c.dragX, c.dragY = 10, 10
	if d := c.commitDir(); d != swipeNone {
		t.Errorf("small drag = %v, want swipeNone", d)
	}

	// DragEnd should invoke callback on commit.
	got = swipeNone
	c.dragX, c.dragY = -100, 0
	c.DragEnd()
	if got != swipeLeft {
		t.Errorf("DragEnd left callback dir = %v, want swipeLeft", got)
	}
}

// Dragged must move the card's face (bg/thumb) to track the pointer every
// frame, not just recolor the overlay. Refresh() alone does not re-run
// Layout(), so this pins the requirement that the drag offset actually
// reaches the canvas objects.
func TestSwipeCardDraggedMovesFaceVisually(t *testing.T) {
	c := newSwipeCard("/tmp/a.pdf", func(_ string, _ swipeDir) {})
	c.Resize(fyne.NewSize(150, 190))
	bgBefore := c.bg.Position()
	thumbBefore := c.thumb.Position()

	c.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(-30, 4)})

	if got := c.bg.Position(); got.X != bgBefore.X-30 || got.Y != bgBefore.Y+4 {
		t.Errorf("bg did not follow drag: got %v, want offset (-30,+4) from %v", got, bgBefore)
	}
	if got := c.thumb.Position(); got.X != thumbBefore.X-30 || got.Y != thumbBefore.Y+4 {
		t.Errorf("thumb did not follow drag: got %v, want offset (-30,+4) from %v", got, thumbBefore)
	}
}

func TestSetModeShowsSwipePane(t *testing.T) {
	ui := newTestApp(t)
	if ui.mode != modeReview {
		t.Fatalf("default mode = %v, want modeReview", ui.mode)
	}
	if ui.reviewPane.Visible() == false {
		t.Error("review pane should start visible")
	}
	if ui.swipePane.Visible() {
		t.Error("swipe pane should start hidden")
	}

	ui.setMode(modeSwipe)
	if ui.mode != modeSwipe {
		t.Fatalf("mode = %v, want modeSwipe", ui.mode)
	}
	if ui.reviewPane.Visible() {
		t.Error("review pane should hide in swipe mode")
	}
	if !ui.swipePane.Visible() {
		t.Error("swipe pane should show in swipe mode")
	}

	ui.setMode(modeReview)
	if !ui.reviewPane.Visible() || ui.swipePane.Visible() {
		t.Error("switching back should restore review pane")
	}
}

func TestSwipeLeftReplacesSlotInPlace(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	withCategory(t, ui, "Personal")
	dir := t.TempDir()
	for _, name := range []string{"a.pdf", "b.pdf", "c.pdf"} {
		touch(t, filepath.Join(dir, name))
	}
	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	// Stable order for assertions.
	ui.sorter.Queue = []string{
		filepath.Join(dir, "a.pdf"),
		filepath.Join(dir, "b.pdf"),
		filepath.Join(dir, "c.pdf"),
	}

	ui.setMode(modeSwipe)
	ui.swipeLeftCat = "Work"
	ui.swipeRightCat = "Personal"
	ui.swipeBoard.capacity = 2
	ui.syncSwipeBoard()
	ui.waitForSwipeThumbs()

	if len(ui.swipeBoard.cards) != 2 {
		t.Fatalf("cards = %d, want 2 (capacity)", len(ui.swipeBoard.cards))
	}

	// Snapshot the surviving neighbor — it must be the same widget after replace.
	cardA := ui.swipeCardByPath[filepath.Join(dir, "a.pdf")]
	cardB := ui.swipeCardByPath[filepath.Join(dir, "b.pdf")]
	if cardA == nil || cardB == nil {
		t.Fatal("expected cards for a.pdf and b.pdf")
	}
	slotB := cardB.slot

	// Swipe b.pdf left → Work; b's slot is rebound to c.pdf; a stays put.
	ui.onSwipeCard(filepath.Join(dir, "b.pdf"), swipeLeft)
	ui.waitForSwipeThumbs()

	if _, err := os.Stat(filepath.Join(dir, "Work", "b.pdf")); err != nil {
		t.Errorf("b.pdf should be under Work/: %v", err)
	}
	if ui.sorter.Remaining() != 2 {
		t.Errorf("Remaining = %d, want 2", ui.sorter.Remaining())
	}
	if len(ui.swipeBoard.cards) != 2 {
		t.Errorf("after swipe, cards = %d, want 2 (slot replaced, not rebuilt)", len(ui.swipeBoard.cards))
	}
	// Neighbor widget identity preserved (no yank/re-render of a).
	if ui.swipeCardByPath[filepath.Join(dir, "a.pdf")] != cardA {
		t.Error("a.pdf card was recreated — neighbors must stay in place")
	}
	// Same widget that held b is now bound to c, same slot.
	if cardB.path != filepath.Join(dir, "c.pdf") {
		t.Errorf("replaced card path = %s, want c.pdf", cardB.path)
	}
	if cardB.slot != slotB {
		t.Errorf("replaced card slot = %d, want %d (in-place)", cardB.slot, slotB)
	}
	if ui.swipeCardByPath[filepath.Join(dir, "c.pdf")] != cardB {
		t.Error("c.pdf should occupy the rebound card widget")
	}
}

func TestSwipeDragOverlayUsesAccentAndCategory(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	withCategory(t, ui, "Personal")
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.setMode(modeSwipe)
	ui.swipeLeftCat = "Work"
	ui.swipeRightCat = "Personal"
	ui.swipeBoard.leftLabel = "Work"
	ui.swipeBoard.rightLabel = "Personal"
	ui.swipeBoard.capacity = 1
	ui.syncSwipeBoard()

	card := ui.swipeBoard.cards[0]
	// Soft left preview (below commit threshold, above preview threshold).
	card.dragging = true
	card.dragX = -40
	card.dragY = 0
	card.applyDragVisual()

	if !card.overlay.Visible() || !card.badge.Visible() || !card.badgeBg.Visible() {
		t.Fatal("overlay wash + badge should be visible while dragging left")
	}
	if card.previewDir != swipeLeft {
		t.Errorf("previewDir = %v, want swipeLeft", card.previewDir)
	}
	if !strings.Contains(card.badge.Text, "Work") {
		t.Errorf("badge %q should name the left category", card.badge.Text)
	}
	// Wash should carry the left accent RGB (alpha varies with progress).
	wash, ok := card.overlay.FillColor.(color.NRGBA)
	if !ok {
		t.Fatalf("overlay fill type %T", card.overlay.FillColor)
	}
	if wash.R != swipeLeftAccent.R || wash.G != swipeLeftAccent.G || wash.B != swipeLeftAccent.B {
		t.Errorf("wash RGB = #%02X%02X%02X, want left accent", wash.R, wash.G, wash.B)
	}
	if wash.A < 70 {
		t.Errorf("wash alpha %d too faint", wash.A)
	}

	// Hard right commit drag.
	card.dragX = swipeCommitX + 10
	card.applyDragVisual()
	if card.previewDir != swipeRight {
		t.Errorf("previewDir = %v, want swipeRight", card.previewDir)
	}
	if !strings.Contains(card.badge.Text, "Personal") {
		t.Errorf("badge %q should name the right category", card.badge.Text)
	}
	if card.dragProgress < 1 {
		t.Errorf("dragProgress = %v, want 1 at/over commit", card.dragProgress)
	}

	// Snap back hides overlay.
	card.dragging = false
	card.dragX, card.dragY = 0, 0
	card.applyDragVisual()
	if card.overlay.Visible() || card.badge.Visible() {
		t.Error("overlay should hide when drag ends without commit path")
	}
}

func TestSwipeRaiseCardDrawsOnTop(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	withCategory(t, ui, "Personal")
	dir := t.TempDir()
	for _, name := range []string{"a.pdf", "b.pdf", "c.pdf"} {
		touch(t, filepath.Join(dir, name))
	}
	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.sorter.Queue = []string{
		filepath.Join(dir, "a.pdf"),
		filepath.Join(dir, "b.pdf"),
		filepath.Join(dir, "c.pdf"),
	}
	ui.setMode(modeSwipe)
	ui.swipeBoard.capacity = 3
	ui.syncSwipeBoard()

	first := ui.swipeBoard.cards[0]
	slotBefore := first.slot
	ui.swipeBoard.RaiseCard(first)

	top := ui.swipeBoard.cards[len(ui.swipeBoard.cards)-1]
	if top != first {
		t.Error("raised card should be last in paint order (drawn on top)")
	}
	if first.slot != slotBefore {
		t.Errorf("raise must not change slot (layout pos); got %d want %d", first.slot, slotBefore)
	}
}

func TestSwipeDownSkipsWithoutMovingFile(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	withCategory(t, ui, "Personal")
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.pdf"))
	touch(t, filepath.Join(dir, "b.pdf"))
	if _, err := ui.sorter.OpenFolder(dir); err != nil {
		t.Fatal(err)
	}
	ui.sorter.Queue = []string{
		filepath.Join(dir, "a.pdf"),
		filepath.Join(dir, "b.pdf"),
	}
	ui.setMode(modeSwipe)
	ui.swipeLeftCat = "Work"
	ui.swipeRightCat = "Personal"

	target := filepath.Join(dir, "a.pdf")
	ui.onSwipeCard(target, swipeDown)

	if _, err := os.Stat(target); err != nil {
		t.Errorf("skip must leave file on disk: %v", err)
	}
	if ui.sorter.Remaining() != 1 {
		t.Errorf("Remaining = %d, want 1", ui.sorter.Remaining())
	}
	if ui.sorter.IndexOf(target) >= 0 {
		t.Error("skipped path should leave the queue")
	}
}

func TestSyncSwipeCategorySelectsDefaults(t *testing.T) {
	ui := newTestApp(t)
	withCategory(t, ui, "Work")
	withCategory(t, ui, "Taxes")
	ui.syncSwipeCategorySelects()
	if ui.swipeLeftCat != "Work" || ui.swipeRightCat != "Taxes" {
		t.Errorf("left/right = %q/%q, want Work/Taxes", ui.swipeLeftCat, ui.swipeRightCat)
	}
}

func TestEllipsizeToWidthShortensLongTitles(t *testing.T) {
	style := fyne.TextStyle{Bold: true}
	size := float32(11)
	long := "very-long-invoice-filename-that-should-not-overflow.pdf"
	fullW := fyne.MeasureText(long, size, style).Width
	got := ellipsizeToWidth(long, fullW*0.4, size, style)
	if got == long {
		t.Error("expected ellipsized title, got full string")
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected trailing ellipsis, got %q", got)
	}
	if fyne.MeasureText(got, size, style).Width > fullW*0.4+1 {
		t.Errorf("ellipsized width exceeds budget: %q", got)
	}
	// Fits unchanged when there is room.
	if ellipsizeToWidth("a.pdf", 400, size, style) != "a.pdf" {
		t.Error("short title should pass through")
	}
}

func TestDarkPaletteIsWarmCharcoal(t *testing.T) {
	p := darkPalette
	bg, ok := p.background.(color.NRGBA)
	if !ok {
		t.Fatalf("background type %T", p.background)
	}
	if bg.R != 0x0F || bg.G != 0x0F || bg.B != 0x0F {
		t.Errorf("deep background = #%02X%02X%02X, want #0F0F0F", bg.R, bg.G, bg.B)
	}
	surf, ok := p.surface.(color.NRGBA)
	if !ok {
		t.Fatalf("surface type %T", p.surface)
	}
	if surf.R != 0x1C || surf.G != 0x1C || surf.B != 0x1A {
		t.Errorf("elevated surface = #%02X%02X%02X, want #1C1C1A", surf.R, surf.G, surf.B)
	}
	// No blue cast: B should not dominate R/G on neutrals.
	if bg.B > bg.R || surf.B > surf.R {
		t.Error("dark neutrals should not be blue-tinted")
	}
}

func TestSwipeAccentsMatchSelectors(t *testing.T) {
	if swipeLeftAccent.R != 0xB5 || swipeLeftAccent.G != 0x6F || swipeLeftAccent.B != 0x74 {
		t.Errorf("left accent = #%02X%02X%02X, want #B56F74", swipeLeftAccent.R, swipeLeftAccent.G, swipeLeftAccent.B)
	}
	if swipeRightAccent.R != 0x7A || swipeRightAccent.G != 0x9E || swipeRightAccent.B != 0x7E {
		t.Errorf("right accent = #%02X%02X%02X, want #7A9E7E", swipeRightAccent.R, swipeRightAccent.G, swipeRightAccent.B)
	}
}
