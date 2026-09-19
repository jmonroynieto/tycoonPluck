# tycoonPluck

A desktop PDF sorter for **Manjaro XFCE on X11**. Open a folder, preview each file, assign it to a category (or skip it), and the file is moved into that category’s subfolder. Categories are yours to define; nothing is preloaded.

Two sorting modes share the same queue and categories:

- **Review** — one file at a time, with a multi-page preview and category buttons
- **Swipe** — a card board for quicker left / right / down assigns

Wayland is not supported. Always build with the `x11` tag (or use `make`).

---

## Requirements

```bash
sudo pacman -S go gcc pkgconf poppler
```

| Package | Why |
|---------|-----|
| `go`, `gcc`, `pkgconf` | Compile Fyne (CGO + GLFW) |
| `poppler` | `pdftoppm` for first-page previews |
| X11 + OpenGL | Normal XFCE desktop already has these |
| `librsvg` (`rsvg-convert`) | Only for `make install` / `make icons` |

Confirm the session is X11:

```bash
echo "$XDG_SESSION_TYPE"   # should print: x11
```

If previews are missing and the app mentions `pdftoppm`:

```bash
sudo pacman -S poppler
which pdftoppm
```

Sorting still works without previews; you just will not see page thumbnails.

---

## Build and run

From this directory:

```bash
make run                 # preferred: go run -tags x11 .
# or
make build && ./tycoonPluck
```

Do not use plain `go build` / `go run` without `-tags x11` — GLFW may try to include Wayland and link the wrong backend. First build downloads modules (needs network) and compiles GLFW.

For editor / direct `go` commands that need the local module workspace:

```bash
make workspace
go run -tags x11 .
```

---

## Install (XFCE app menu)

```bash
sudo pacman -S librsvg   # if you don't already have rsvg-convert
make install
```

`make install` itself needs no `sudo`. It writes only under `~/.local`:

| Path | What |
|------|------|
| `~/.local/bin/tycoonPluck` | Binary |
| `~/.local/share/applications/com.local.tycoonpluck.desktop` | Launcher |
| `~/.local/share/icons/hicolor/**/apps/com.local.tycoonpluck.{png,svg}` | Icon set |

After install, the app appears in the XFCE menu (Whisker Menu, under *Office*). `~/.local/bin` must be on your `PATH`. To remove what the installer wrote:

```bash
make uninstall
```

| Target | Does |
|--------|------|
| `make icons` | Regenerates `packaging/icons/*.png` from the SVG |
| `make install` | Build + icons, then install binary / desktop entry / icons |
| `make uninstall` | Removes everything `make install` wrote |
| `make clean` | Removes local build artifacts and generated icon PNGs |

The scalable master icon lives at `packaging/icons/com.local.tycoonpluck.svg`. The checked-in `packaging/icons/icon.png` (256px) is embedded into the binary for the window / taskbar icon; other sizes are generated on demand by `make icons` / `make install`.

---

## How to use

1. **Open folder** — pick a directory (folders only, by design). Only **top-level** files are queued; subfolders are not scanned. Review and Swipe queue PDFs by default. Empty files and files without a PDF header are skipped and reported. Tick **Expanded formats** in the sidebar to also queue images, documents, tables, and text files (extension or MIME). Rescanning after the toggle keeps the current file in front.
2. **Categories** (left) — start empty. **Add category** creates one (saved under `~/.config/tycoonPluck/categories.json`). Hover a category to reveal **×** — that removes it from the list only; any folder already created for it on disk is left alone. Re-adding the same name reuses that folder.
3. **Assign** — click a category (or use a key) → file moves to `that-folder/CategoryName/` → next file. Queue order is **random** each time you open a folder.
4. **Preview** — Review shows up to the first **3** PDF pages; page buttons appear when more than one page is available.
5. **Open** (`O`) — `xdg-open` the current file in the system viewer, then return to triage.
6. **Skip** (`S`) — leave the file in place for this session. Skips are remembered in the session journal for that directory.
7. **Undo** (`Z`) — reverse the last **assign** (not skip). If undo fails (for example the file was deleted externally), fix disk state and try again; the undo entry is kept until it succeeds.
8. Name clashes become `file (1).pdf`, `file (2).pdf`, …

### Session journal

Assigns and skips for each opened folder are stored at:

`~/.cache/tycoonPluck/session.json` (or `$XDG_CACHE_HOME/tycoonPluck/session.json`)

Reopening the same directory restores undo for files still in their category folders and keeps previously skipped basenames out of the queue. Delete the file to reset session memory.

### Keyboard

Main window focused; not while the Add dialog is open.

| Key | Action |
|-----|--------|
| `1`–`9` | Assign to 1st–9th category |
| `O` | Open current file with system viewer |
| `S` | Skip |
| `Z` | Undo last assign |

### What this release does not do

- Full multi-page navigation or zoom (preview capped at first 3 pages)
- Recursive folder scan
- Auto-categorize / OCR
- Wayland
- Distro packages (`.pacman` / AUR / Flatpak) — install is a manual user-local `make install`

---

## For collaborators

### Tests

```bash
make test                 # go test -tags x11 ./...
# or pure packages only (no main binary link):
go test ./internal/...
```

Prefer `make test` or `go test -tags x11 ./...` so the root package links the X11 GLFW backend.

| Package | What it covers |
|---------|----------------|
| `internal/sorter` | Queue, assign, skip, undo, collisions, move edge cases |
| `internal/categories` | Sanitize, load/save, corrupt JSON |
| `internal/preview` | Real `pdftoppm` on a minimal PDF; skips if the tool is missing |
| `internal/ui` | Buttons, labels, shortcuts, modal key guard (Fyne software driver) |

### Layout

```
tycoonPluck/
  README.md
  Makefile
  main.go
  go.mod
  FyneApp.toml
  packaging/          # desktop entry, icon SVG, install assets
  internal/
    sorter/           # queue / move / undo
    categories/       # config
    preview/          # pdftoppm → image
    formats/          # PDF vs expanded-format matching
    session/          # per-directory journal
    ui/               # Fyne window, Review / Swipe
```

### Quick tester path

1. Put several PDFs in one folder (not inside category subfolders yet).
2. `make run`.
3. Open that folder; confirm count and previews.
4. Assign a few files; confirm they land under `CategoryName/`.
5. Skip one; confirm it stayed put and the queue advanced.
6. Undo an assign; confirm the file returned and is current again.
7. Add a category; quit and restart; confirm it is still listed.
8. Try keys `1`–`9`, `S`, `Z`.

When reporting a problem, include terminal / panic text, Manjaro version, `echo $XDG_SESSION_TYPE`, `go version`, whether `pdftoppm -v` works, and steps to reproduce.

Expected limitations for now: progress `n / total` is session progress (includes skips), not “files remaining on disk”; only top-level files are queued; look-and-feel is Fyne’s theme rather than native GTK/XFCE; the first preview of a large PDF can take a moment.

---
