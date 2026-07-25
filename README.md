# tycoonPluck

**v0 (tester candidate)** — PDF category sorter for **Manjaro XFCE on X11**.

Open a folder of PDFs → categories as buttons on the left → first-page preview → click a category (or press a key) → file is **moved** into that subfolder → next PDF.

| Doc | What it is |
|-----|------------|
| **This README** | Install, build, run, test, tester notes |
| **[plan.md](./plan.md)** | Full design, history, architecture, v0 scope |

Wayland is **not** supported in v0. Always build with the **`x11`** tag (or use `make`).

---

## Requirements

```bash
sudo pacman -S go gcc pkgconf poppler
```

| Package | Why |
|---------|-----|
| `go`, `gcc`, `pkgconf` | Compile Fyne (CGO + GLFW) |
| `poppler` | Provides `pdftoppm` for first-page previews |
| X11 + OpenGL | Normal XFCE desktop already has these |
| `librsvg` (`rsvg-convert`) | Only needed for `make install`/`make icons` — rasterizes the app icon |

Confirm session type:

```bash
echo "$XDG_SESSION_TYPE"   # should print: x11
```

---

## Build & run

From this directory (`tycoonPluck/`):

```bash
make run                 # preferred: go run -tags x11 .
# or
make build && ./tycoonPluck
# or
go run -tags x11 .
go build -tags x11 -o tycoonPluck .
```

**Do not** use plain `go build` / `go run` without `-tags x11` — GLFW will try to include Wayland and may fail or link the wrong backend.

First build downloads modules (needs network) and compiles GLFW.

### Preview dependency

If the app says it cannot preview and mentions `pdftoppm`:

```bash
sudo pacman -S poppler
which pdftoppm
```

Sorting (move/skip/undo) still works without previews; you just will not see page thumbnails.

---

## Install (XFCE app menu, icon, launcher)

```bash
sudo pacman -S librsvg   # if you don't already have rsvg-convert
make install
```

This is a **user-local** install — nothing is written outside `~/.local`, no `sudo` needed for `make install` itself:

| Installed to | What |
|---|---|
| `~/.local/bin/tycoonPluck` | The built binary |
| `~/.local/share/applications/com.local.tycoonpluck.desktop` | App launcher entry |
| `~/.local/share/icons/hicolor/**/apps/com.local.tycoonpluck.{png,svg}` | Icon, all standard sizes + scalable |

After `make install`, tycoonPluck shows up in the XFCE app menu (Whisker Menu, under *Office*) with its icon — no logout required; the install target refreshes the desktop and icon caches for you. `~/.local/bin` must be on your `PATH` (it is by default on most Manjaro XFCE installs).

To remove everything the installer wrote:

```bash
make uninstall
```

Other packaging targets:

| Target | Does |
|---|---|
| `make icons` | Regenerates `packaging/icons/*.png` from the scalable SVG (needs `rsvg-convert`) |
| `make install` | `build` + `icons`, then installs binary/desktop entry/icons and refreshes caches |
| `make uninstall` | Removes everything `make install` wrote |
| `make clean` | Removes the local build binary and generated icon PNGs |

The icon itself is generated from `packaging/gen-icon.py` (three chunky arrows spiraling out from center, in the app's own accent color) into `packaging/icons/com.local.tycoonpluck.svg`; only the 256px `icon.png` rasterized from it is checked into git (it's embedded into the binary via `go:embed` for the window/taskbar icon) — the rest of the size set is generated on demand by `make icons`/`make install`.

---

## How to use

1. **Open folder** — pick a **directory** (the picker only lists folders, not files — by design). PDFs must sit at the **top level** of that folder (subfolders are not scanned).
2. **Categories** (left) — start **empty**. Use **Add category** to create your own (saved under `~/.config/tycoonPluck/categories.json`). Nothing is preloaded. Hover a category to reveal a **×** — removes it from the list only; any folder already created for it (and files already sorted into it) is left untouched on disk. Re-adding the same name later picks the same folder back up.
3. **Assign** — click a category → file moves to `that-folder/CategoryName/` → next PDF. Queue order is **random** each time you open a folder (not A–Z).
4. **Preview pages** — up to the first **3 pages** render when the PDF has that many; page buttons appear under the preview only when there's more than one to show.
5. **Open PDF** (or key `O`) — `xdg-open` the current file in your system PDF viewer for a full inspection, then come back to triage.
6. **Skip** — leave the file in place; continue this session without it. Skips are remembered in the session journal for that directory.
7. **Undo** — reverse the last **assign** (not skip). If undo fails (e.g. file was deleted externally), fix disk state and try again; the undo entry is kept until it succeeds.
8. Name clashes in a category folder become `file (1).pdf`, `file (2).pdf`, …

### Session journal (per directory)

Assigns and skips for each opened folder are recorded in a small cache file:

`~/.cache/tycoonPluck/session.json` (or `$XDG_CACHE_HOME/tycoonPluck/session.json`)

Reopening the same directory restores undo for files still in their category folders and keeps previously skipped basenames out of the queue. Safe to delete the file to reset session memory.

### Keyboard (main window focused; not while Add dialog is open)

| Key | Action |
|-----|--------|
| `1`–`9` | Assign to 1st–9th category |
| `O` | Open current PDF with system viewer (`xdg-open`) |
| `S` | Skip |
| `Z` | Undo |

### What v0 does *not* do

- Full multi-page navigation / zoom — preview is capped at the first 3 pages, no zoom
- Recursive folder scan  
- Auto-categorize / OCR  
- Wayland  
- Distro packages (`.pacman`/AUR/Flatpak) — install target is a manual, user-local `make install` (see [Install](#install-xfce-app-menu-icon-launcher) above)

---

## Tests

```bash
cd tycoonPluck
make test                 # go test -tags x11 ./...
# or only pure packages (no main binary link):
go test ./internal/...
```

| Package | Coverage |
|---------|----------|
| `internal/sorter` | Queue, assign, skip, undo, collisions, move edge cases |
| `internal/categories` | Sanitize, load/save, corrupt JSON, non-string entries |
| `internal/preview` | Real `pdftoppm` on a minimal PDF; skips if tool missing |
| `internal/ui` | Buttons, labels, shortcuts, modal key guard (Fyne software driver) |

Always prefer **`make test`** or **`go test -tags x11 ./...`** so the root package links the X11 GLFW backend.

---

## For testers (v0)

### Happy path

1. Put several PDFs in one folder (not inside category subfolders yet).
2. `make run`.
3. Open that folder; confirm count and first-page previews.
4. Assign a few files; confirm they land under `CategoryName/`.
5. Skip one; confirm it stayed put and queue advanced.
6. Undo an assign; confirm file returned and is current again.
7. Add a category; quit and restart; confirm it is still listed.
8. Try keys `1`–`9`, `S`, `Z`.

### Please report

- Full terminal output / panic text  
- Manjaro version, `echo $XDG_SESSION_TYPE`, `go version`  
- Whether `pdftoppm` works: `pdftoppm -v`  
- Steps to reproduce (folder layout if relevant)

### Known limitations (expected in v0)

- Progress `n / total` is **session progress** (includes skips), not “files remaining on disk.”  
- Only top-level PDFs are queued.  
- Look-and-feel is Fyne’s theme, not GTK/XFCE native widgets.  
- First preview of a large PDF can take a moment (render is async).  

### Changelog (tester-relevant)

- **v0.1.0+** — Fixed crash on **Open folder** (Fyne 2.8 `FileDialog.Resize` must run after `Show`). Enabled Fyne `fyneDo` migration (preview already uses `fyne.Do`) via `app.SetMetadata` in `main.go` — `FyneApp.toml` alone only gets read next to the executable / `go run` cwd, so it silently didn't apply once installed to `~/.local/bin`; metadata is now baked into the binary so the threading warning stays gone everywhere.

---

## Layout

```
tycoonPluck/
  plan.md
  README.md
  Makefile
  main.go
  go.mod
  FyneApp.toml
  packaging/
    gen-icon.py                     # regenerates the icon SVG
    com.local.tycoonpluck.desktop   # app launcher entry
    icons/
      com.local.tycoonpluck.svg     # scalable master icon
      icon.png                      # 256px, checked in, go:embed'd for window icon
  internal/
    sorter/       # queue / move / undo
    categories/   # config
    preview/      # pdftoppm → image
    ui/           # Fyne window
```

---

## Related trees

| Path | Role |
|------|------|
| **`tycoonPluck/`** | **Current product (this app)** |
| `../pdf-sorter/` | Earlier Python/GTK experiment — not for testers |
| `../plan.md` | Repo index pointing here |
