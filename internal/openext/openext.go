// Package openext opens files with the desktop default application (xdg-open on Linux).
package openext

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// Open starts the platform default handler for path without waiting for it to exit.
func Open(path string) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}
	if _, err := os.Stat(path); err != nil {
		return err
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return fmt.Errorf("xdg-open not found — install xdg-utils (Manjaro: sudo pacman -S xdg-utils)")
		}
		cmd = exec.Command("xdg-open", path)
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		return fmt.Errorf("open not supported on %s", runtime.GOOS)
	}
	// Detach: don't wait for the viewer to close.
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the process in the background so we don't leave zombies.
	go func() { _ = cmd.Wait() }()
	return nil
}
