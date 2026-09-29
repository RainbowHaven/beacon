package version_test

import (
	"testing"

	"github.com/magiconair/beacon/internal/version"
)

func TestLineDefaults(t *testing.T) {
	t.Setenv("RAILWAY_GIT_COMMIT_SHA", "")
	got := version.Line()
	if got != "v0.0.0 (unknown, unknown)" {
		t.Fatalf("got %q", got)
	}
}

func TestLineFormats(t *testing.T) {
	prevV, prevC, prevD := version.Version, version.Commit, version.Date
	t.Cleanup(func() {
		version.Version, version.Commit, version.Date = prevV, prevC, prevD
	})
	version.Version = "v1.2.3"
	version.Commit = "abcdef0123456789"
	version.Date = "2026-09-29"
	if got := version.Line(); got != "v1.2.3 (abcdef0, 2026-09-29)" {
		t.Fatalf("got %q", got)
	}
}
