package server

import (
	"testing"
	"time"
)

func TestReportMonthChoices(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	got := reportMonthChoices(now, "2026-10")
	if len(got) != 24 {
		t.Fatalf("len=%d want 24", len(got))
	}
	if got[0].Value != "2026-10" || got[0].Label != "October 2026" {
		t.Fatalf("newest = %+v", got[0])
	}
	if got[1].Value != "2026-09" || got[1].Label != "September 2026" {
		t.Fatalf("previous = %+v", got[1])
	}
	last := got[len(got)-1]
	if last.Value != "2024-11" || last.Label != "November 2024" {
		t.Fatalf("oldest = %+v", last)
	}

	got = reportMonthChoices(now, "2023-01")
	if got[0].Value != "2026-10" {
		t.Fatalf("still newest first: %+v", got[0])
	}
	found := false
	for _, c := range got {
		if c.Value == "2023-01" && c.Label == "January 2023" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("selected month outside the window missing")
	}
	if len(got) != 25 {
		t.Fatalf("len with extra selected=%d want 25", len(got))
	}
}
