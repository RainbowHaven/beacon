package server

import (
	"slices"
	"testing"
)

func TestCSVSafeRow(t *testing.T) {
	in := []string{"", "Roof leak", "=HYPERLINK(\"x\")", "+1+1", "-3", "-2.5", "-cmd", "@SUM(A1)", "\tTab", "2026-09-01"}
	want := []string{"", "Roof leak", "'=HYPERLINK(\"x\")", "'+1+1", "-3", "-2.5", "'-cmd", "'@SUM(A1)", "'\tTab", "2026-09-01"}
	if got := csvSafeRow(in); !slices.Equal(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}
