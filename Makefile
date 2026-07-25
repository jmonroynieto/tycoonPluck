# tycoonPluck — always build/test for X11 (XFCE target; no Wayland).
TAGS ?= x11
BIN  ?= tycoonPluck
APP_ID ?= com.local.tycoonpluck

# User-local XDG install — no sudo, no system-wide changes.
BINDIR  ?= $(HOME)/.local/bin
DATADIR ?= $(HOME)/.local/share
ICON_SIZES := 16 24 32 48 64 128 256 512

.PHONY: run build test clean icons install uninstall

run:
	go run -tags $(TAGS) .

build:
	go build -tags $(TAGS) -o $(BIN) .

# Link GLFW with the X11 backend (plain `go test ./...` may pull Wayland).
test:
	go test -tags $(TAGS) ./...

clean:
	rm -f $(BIN) packaging/icons/*.png
	git checkout -- packaging/icons/icon.png 2>/dev/null || true

# Rasterize the hicolor icon set from the scalable master. icon.png (256, used
# by go:embed in main.go) is checked into git; the rest are install-time only.
icons:
	@command -v rsvg-convert >/dev/null || { echo "rsvg-convert not found — install librsvg to regenerate icons" >&2; exit 1; }
	for s in $(ICON_SIZES); do \
		rsvg-convert -w $$s -h $$s packaging/icons/$(APP_ID).svg -o packaging/icons/$$s.png; \
	done

# Installs the binary, .desktop launcher, and hicolor icon set under
# ~/.local — picked up by XFCE's app menu/panel without a logout.
install: build icons
	install -Dm755 $(BIN) $(BINDIR)/$(BIN)
	install -Dm644 packaging/$(APP_ID).desktop $(DATADIR)/applications/$(APP_ID).desktop
	install -Dm644 packaging/icons/$(APP_ID).svg $(DATADIR)/icons/hicolor/scalable/apps/$(APP_ID).svg
	for s in $(ICON_SIZES); do \
		install -Dm644 packaging/icons/$$s.png $(DATADIR)/icons/hicolor/$${s}x$${s}/apps/$(APP_ID).png; \
	done
	-update-desktop-database $(DATADIR)/applications
	-gtk-update-icon-cache -f -t $(DATADIR)/icons/hicolor
	@echo "Installed. Launch from the XFCE app menu, or: $(BINDIR)/$(BIN)"

uninstall:
	rm -f $(BINDIR)/$(BIN)
	rm -f $(DATADIR)/applications/$(APP_ID).desktop
	rm -f $(DATADIR)/icons/hicolor/scalable/apps/$(APP_ID).svg
	for s in $(ICON_SIZES); do rm -f $(DATADIR)/icons/hicolor/$${s}x$${s}/apps/$(APP_ID).png; done
	-update-desktop-database $(DATADIR)/applications
	-gtk-update-icon-cache -f -t $(DATADIR)/icons/hicolor
