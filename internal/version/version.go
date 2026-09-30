package version

import (
	"fmt"
	"os"
	"strings"
)

// Set at link time via -ldflags, e.g.
//
//	-X github.com/RainbowHaven/beacon/internal/version.Version=1.2.3
//	-X github.com/RainbowHaven/beacon/internal/version.Commit=abc1234
//	-X github.com/RainbowHaven/beacon/internal/version.Date=2026-09-29T14:32Z
var (
	Version = "0.0.0"
	Commit  = "unknown"
	Date    = "unknown"
)

// Line returns a short build label like "v1.2.3 (abc1234, 2026-09-29T14:32Z)".
func Line() string {
	ver := strings.TrimPrefix(Version, "v")
	commit := Commit
	if commit == "" || commit == "unknown" {
		if sha := os.Getenv("RAILWAY_GIT_COMMIT_SHA"); sha != "" {
			commit = sha
		}
	}
	if len(commit) > 7 {
		commit = commit[:7]
	}
	date := Date
	if date == "" {
		date = "unknown"
	}
	return fmt.Sprintf("v%s (%s, %s)", ver, commit, date)
}
