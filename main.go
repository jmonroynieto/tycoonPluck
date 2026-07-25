// tycoonPluck — PDF category sorter (Fyne).
//
// Build / run / test on Manjaro XFCE (X11) — always use the x11 tag:
//
//	sudo pacman -S go gcc pkgconf poppler
//	cd tycoonPluck && make test && make run
//	# or: go run -tags x11 .

package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"tycoonPluck/internal/ui"
	"tycoonPluck/internal/version"
)

//go:embed packaging/icons/icon.png
var iconPNG []byte

func main() {
	icon := fyne.NewStaticResource("icon.png", iconPNG)

	// Fyne's FyneApp.toml auto-loading only checks the directory next to the
	// running executable (or the cwd, under "go run") — a dev-mode
	// convenience that finds nothing once installed to ~/.local/bin (see
	// `make install`). Set metadata explicitly so it's baked into the binary
	// and correct everywhere, including the fyneDo migration flag that
	// silences Fyne's "not migrated to the fyne.Do threading model" warning.
	// Keep in sync with FyneApp.toml.
	app.SetMetadata(fyne.AppMetadata{
		ID:      "com.local.tycoonpluck",
		Name:    "tycoonPluck",
		Version: version.Version,
		Build:   1,
		Icon:    icon,
		Migrations: map[string]bool{
			"fyneDo": true,
		},
	})

	a := app.NewWithID("com.local.tycoonpluck")
	a.SetIcon(icon)
	ui.New(a).ShowAndRun()
}
