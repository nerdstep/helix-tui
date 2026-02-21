package version

import (
	"fmt"
	"runtime/debug"
	"strings"
)

var (
	// These values can be overridden at build time via:
	// -ldflags "-X helix-tui/internal/version.Version=v1.2.3 -X helix-tui/internal/version.Commit=abc123 -X helix-tui/internal/version.Date=2026-02-21T00:00:00Z"
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func String() string {
	module := "helix-tui"
	goVersion := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		if strings.TrimSpace(info.Main.Path) != "" {
			module = strings.TrimSpace(info.Main.Path)
		}
		if strings.TrimSpace(info.GoVersion) != "" {
			goVersion = strings.TrimSpace(info.GoVersion)
		}
	}
	return fmt.Sprintf("%s version=%s commit=%s date=%s go=%s", module, Version, Commit, Date, goVersion)
}
