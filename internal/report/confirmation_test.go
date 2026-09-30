package report

import (
	"testing"
	"time"

	"github.com/magiconair/beacon/internal/domain"
)

func TestComputeStatus(t *testing.T) {
	feb := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	at := func(s string) time.Time {
		v, err := time.Parse("2006-01-02 15:04", s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	confirmed := &domain.MonthConfirmation{Fingerprint: "aaa", ConfirmedAt: at("2026-03-02 10:00")}

	for _, tc := range []struct {
		name        string
		now         string
		latest      *domain.MonthConfirmation
		fingerprint string
		want        State
		daysOverdue int
	}{
		{"month in progress", "2026-02-28 23:59", nil, "aaa", StateNotYetDue, 0},
		{"due on the first day after the month", "2026-03-01 00:00", nil, "aaa", StateDue, 0},
		{"due until the end of day 7", "2026-03-07 23:59", nil, "aaa", StateDue, 0},
		{"overdue on day 8", "2026-03-08 00:00", nil, "aaa", StateOverdue, 1},
		{"overdue on day 10", "2026-03-10 12:00", nil, "aaa", StateOverdue, 3},
		{"confirmed", "2026-03-20 09:00", confirmed, "aaa", StateConfirmed, 0},
		{"changed after confirmation before the deadline", "2026-03-05 09:00", confirmed, "bbb", StateChanged, 0},
		{"changed after confirmation and overdue", "2026-03-09 09:00", confirmed, "bbb", StateChanged, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeStatus(feb, at(tc.now), tc.latest, tc.fingerprint)
			if got.State != tc.want || got.DaysOverdue != tc.daysOverdue {
				t.Fatalf("got %s with %d days overdue, want %s with %d", got.State, got.DaysOverdue, tc.want, tc.daysOverdue)
			}
			if got.Overdue() != (tc.daysOverdue > 0) {
				t.Fatalf("Overdue() = %v", got.Overdue())
			}
			if got.Confirmation != tc.latest {
				t.Fatal("latest confirmation not kept")
			}
		})
	}
}

func TestStatusSummary(t *testing.T) {
	feb := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	c := &domain.MonthConfirmation{Fingerprint: "aaa", ConfirmedByName: "Alex", ConfirmedAt: time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)}
	for _, tc := range []struct {
		now         time.Time
		latest      *domain.MonthConfirmation
		fingerprint string
		want        string
	}{
		{time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC), nil, "aaa", "Not confirmed. Due by 2026-03-07."},
		{time.Date(2026, 3, 8, 9, 0, 0, 0, time.UTC), nil, "aaa", "Not confirmed. Overdue by 1 day (due by 2026-03-07)."},
		{time.Date(2026, 3, 9, 9, 0, 0, 0, time.UTC), c, "aaa", "Confirmed complete by Alex on 2026-03-02 10:00 UTC."},
		{time.Date(2026, 3, 9, 9, 0, 0, 0, time.UTC), c, "bbb", "Data changed after confirmation on 2026-03-02 10:00 UTC by Alex — please confirm again. Overdue by 2 days (due by 2026-03-07)."},
	} {
		if got := ComputeStatus(feb, tc.now, tc.latest, tc.fingerprint).Summary(); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

func TestDeadlineAndEndedMonths(t *testing.T) {
	if got := Deadline(time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)); !got.Equal(time.Date(2027, 1, 8, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("deadline %v", got)
	}
	now := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	months := EndedMonths(now, 3)
	want := []string{"2025-12", "2025-11", "2025-10"}
	for i, m := range months {
		if m.Format("2006-01") != want[i] {
			t.Fatalf("months %v", months)
		}
	}
}
