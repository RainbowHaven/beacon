package report

import (
	"fmt"
	"time"

	"github.com/RainbowHaven/beacon/internal/domain"
)

// DueDays is how many days after the end of a month its data are due. The
// due date ends at midnight UTC.
const DueDays = 7

// State is the reporting status of a safe house for one month.
type State string

const (
	StateNotYetDue State = "not_yet_due" // the month has not ended
	StateDue       State = "due"
	StateOverdue   State = "overdue"
	StateConfirmed State = "confirmed"
	StateChanged   State = "changed" // data changed after the latest confirmation
)

func (s State) Label() string {
	switch s {
	case StateNotYetDue:
		return "Not yet due"
	case StateDue:
		return "Due"
	case StateOverdue:
		return "Overdue"
	case StateConfirmed:
		return "Confirmed"
	case StateChanged:
		return "Changed after confirmation"
	}
	return string(s)
}

// Status is the reporting status of a house-month at a point in time.
type Status struct {
	Month        time.Time // first day of the month, UTC
	State        State
	Deadline     time.Time                 // first instant after the due date
	Confirmation *domain.MonthConfirmation // latest confirmation, even if data changed since
	DaysOverdue  int
}

func firstOfMonth(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// Deadline is the first instant after the due date of month.
func Deadline(month time.Time) time.Time {
	return firstOfMonth(month).AddDate(0, 1, DueDays)
}

// Ended reports whether month is over at now.
func Ended(month, now time.Time) bool {
	return !now.Before(firstOfMonth(month).AddDate(0, 1, 0))
}

// LastEndedMonth is the first day of the most recent month that has ended.
func LastEndedMonth(now time.Time) time.Time {
	return firstOfMonth(now).AddDate(0, -1, 0)
}

// EndedMonths lists the n most recent ended months, newest first.
func EndedMonths(now time.Time, n int) []time.Time {
	out := make([]time.Time, n)
	last := LastEndedMonth(now)
	for i := range out {
		out[i] = last.AddDate(0, -i, 0)
	}
	return out
}

// ComputeStatus derives the status of month at now from its latest
// confirmation (nil if none) and the current data fingerprint. A month is
// overdue when its current data are not confirmed by the deadline; days
// overdue count every started day after the deadline.
func ComputeStatus(month, now time.Time, latest *domain.MonthConfirmation, fingerprint string) Status {
	st := Status{Month: firstOfMonth(month), Deadline: Deadline(month), Confirmation: latest}
	if !Ended(month, now) {
		st.State = StateNotYetDue
		return st
	}
	if latest != nil && latest.Fingerprint == fingerprint {
		st.State = StateConfirmed
		return st
	}
	if !now.Before(st.Deadline) {
		st.DaysOverdue = int(now.Sub(st.Deadline)/day) + 1
	}
	switch {
	case latest != nil:
		st.State = StateChanged
	case st.DaysOverdue > 0:
		st.State = StateOverdue
	default:
		st.State = StateDue
	}
	return st
}

// Overdue reports whether the month's current data are past their deadline
// without a valid confirmation.
func (s Status) Overdue() bool { return s.DaysOverdue > 0 }

// DueDate is the last day on which the month's data are on time.
func (s Status) DueDate() time.Time { return s.Deadline.AddDate(0, 0, -1) }

// ConfirmedBy is the name of whoever made the latest confirmation.
func (s Status) ConfirmedBy() string {
	if s.Confirmation == nil {
		return ""
	}
	if s.Confirmation.ConfirmedByName == "" {
		return "a removed user"
	}
	return s.Confirmation.ConfirmedByName
}

// Summary is a one-line plain-English description of the status.
func (s Status) Summary() string {
	var out string
	switch s.State {
	case StateNotYetDue:
		return "Month in progress. Data entry can be confirmed complete after the month ends."
	case StateConfirmed:
		return "Confirmed complete by " + s.ConfirmedBy() + " on " + s.Confirmation.ConfirmedAt.UTC().Format("2006-01-02 15:04") + " UTC."
	case StateChanged:
		out = "Data changed after confirmation on " + s.Confirmation.ConfirmedAt.UTC().Format("2006-01-02 15:04") + " UTC by " + s.ConfirmedBy() + " — please confirm again."
	default:
		out = "Not confirmed."
	}
	if s.Overdue() {
		days := "days"
		if s.DaysOverdue == 1 {
			days = "day"
		}
		return fmt.Sprintf("%s Overdue by %d %s (due by %s).", out, s.DaysOverdue, days, s.DueDate().Format("2006-01-02"))
	}
	return out + " Due by " + s.DueDate().Format("2006-01-02") + "."
}

// MonthKey is the month as YYYY-MM.
func (s Status) MonthKey() string { return s.Month.Format("2006-01") }

// MonthLabel is the month as e.g. "February 2024".
func (s Status) MonthLabel() string { return s.Month.Format("January 2006") }
