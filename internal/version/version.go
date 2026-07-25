// Package version holds tycoonPluck's version string — the single source
// used both for app metadata (main.go) and the small version label in the
// UI topbar, so the two can't drift apart.
//
// Keep in sync with the Version field in FyneApp.toml, which is separate:
// it's only read by the `fyne` packaging CLI, not by a plain go build/run,
// so it can't be the runtime source of truth (see main.go's SetMetadata
// comment for why).
package version

const Version = "0.1.0"
