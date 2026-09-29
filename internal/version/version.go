package version

import (
	"fmt"
	"strings"
)

// Set at link time via -ldflags, e.g.
//
//	-X github.com/magiconair/beacon/internal/version.Version=1.2.3
//	-X github.com/magiconair/beacon/internal/version.Commit=abc1234
//	-X github.com/magiconair/beacon/internal/version.Date=2026-09-29
var (
	Version = "0.0.0"
	Commit  = "unknown"
	Date    = "unknown"
)

// Line returns a footer string like "v1.2.3 (abc1234, 2026-09-29)".
func Line() string {
	ver := strings.TrimPrefix(Version, "v")
	commit := Commit
	if len(commit) > 7 {
		commit = commit[:7]
	}
	return fmt.Sprintf("v%s (%s, %s)", ver, commit, Date)
}
