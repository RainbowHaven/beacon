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

func TestParseExpenseReportRange(t *testing.T) {
	check := func(fromRaw, toRaw string, now time.Time, wantFrom, wantTo, wantErr string) {
		t.Helper()
		from, to, fromIn, toIn, errMsg := parseExpenseReportRange(fromRaw, toRaw, now)
		if errMsg != wantErr {
			t.Fatalf("parse %q %q: error %q, want %q", fromRaw, toRaw, errMsg, wantErr)
		}
		gotFrom, gotTo := "", ""
		if !from.IsZero() {
			gotFrom = from.Format("2006-01-02")
		}
		if !to.IsZero() {
			gotTo = to.Format("2006-01-02")
		}
		if wantErr == "" {
			if gotFrom != wantFrom || gotTo != wantTo {
				t.Fatalf("parse %q %q: range %s..%s, want %s..%s", fromRaw, toRaw, gotFrom, gotTo, wantFrom, wantTo)
			}
			if fromIn != formatUSDate(from) || toIn != formatUSDate(to) {
				t.Fatalf("inputs %q %q, want %q %q", fromIn, toIn, formatUSDate(from), formatUSDate(to))
			}
			return
		}
		if wantFrom != "" && fromIn != wantFrom {
			t.Fatalf("from input %q, want %q", fromIn, wantFrom)
		}
		if wantTo != "" && toIn != wantTo {
			t.Fatalf("to input %q, want %q", toIn, wantTo)
		}
	}

	october := time.Date(2026, 10, 26, 21, 0, 0, 0, time.UTC)
	check("", "", october, "2026-09-01", "2026-09-30", "")
	check("  ", " ", october, "2026-09-01", "2026-09-30", "")

	march := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	check("", "", march, "2024-02-01", "2024-02-29", "")

	january := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
	check("", "", january, "2025-12-01", "2025-12-31", "")

	check("01/15/2026", "01/15/2026", october, "2026-01-15", "2026-01-15", "")
	check("2026-03-01", "03/31/2026", october, "2026-03-01", "2026-03-31", "")
	check("03/15/2026", "03/01/2026", october, "03/15/2026", "03/01/2026", expenseDateOrderMsg)
	check("03/01/2026", "", october, "03/01/2026", "", expenseDateBothMsg)
	check("", "03/31/2026", october, "", "03/31/2026", expenseDateBothMsg)
	check("02/31/2026", "03/01/2026", october, "02/31/2026", "03/01/2026", expenseDateFormatMsg)
	check("13/01/2026", "03/01/2026", october, "13/01/2026", "03/01/2026", expenseDateFormatMsg)
}
