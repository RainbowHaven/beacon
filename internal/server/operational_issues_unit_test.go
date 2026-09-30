package server

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RainbowHaven/beacon/internal/domain"
	"github.com/RainbowHaven/beacon/internal/store"
)

func TestOperationalIssueFormFields(t *testing.T) {
	categories := domain.Categories{
		{Key: "food_supplies", Label: "Food or supplies", SortOrder: 10, Active: true},
		{Key: "staffing_agent", Label: "Staffing or Agent change", SortOrder: 20},
	}
	today := time.Date(2026, 3, 15, 22, 0, 0, 0, time.UTC)
	base := operationalIssueForm{
		IdentifiedOn: "2026-03-10",
		Category:     "food_supplies",
		Description:  "Rice delivery missed",
		Status:       "open",
		ClosedOn:     "2026-03-12",
		ClosureNotes: "ignored while open",
	}
	f, err := base.fields(today, categories)
	if err != nil {
		t.Fatal(err)
	}
	if f.ClosedOn != nil || f.ClosureNotes != "" {
		t.Fatalf("open form kept closure: %+v", f)
	}

	tomorrow := base
	tomorrow.IdentifiedOn = "2026-03-16"
	if _, err := tomorrow.fields(today, categories); err != nil {
		t.Fatalf("one day of time zone slack: %v", err)
	}

	cases := []struct {
		name string
		mod  func(f *operationalIssueForm)
		want string
	}{
		{"missing date", func(f *operationalIssueForm) { f.IdentifiedOn = "" }, "required"},
		{"bad date", func(f *operationalIssueForm) { f.IdentifiedOn = "03/10/2026" }, "Invalid date identified"},
		{"future date", func(f *operationalIssueForm) { f.IdentifiedOn = "2026-03-20" }, "future"},
		{"bad closure date", func(f *operationalIssueForm) {
			f.Status = "resolved"
			f.ClosedOn = "soon"
		}, "Invalid resolution"},
		{"future closure", func(f *operationalIssueForm) {
			f.Status = "closed"
			f.ClosedOn = "2026-04-01"
		}, "future"},
		{"resolved without date", func(f *operationalIssueForm) {
			f.Status = "resolved"
			f.ClosedOn = ""
		}, "date is required"},
		{"no category", func(f *operationalIssueForm) { f.Category = "" }, "Choose a category"},
		{"unknown category", func(f *operationalIssueForm) { f.Category = "residents" }, "Choose a category"},
		{"retired category", func(f *operationalIssueForm) { f.Category = "staffing_agent" }, "Choose a category"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := base
			tc.mod(&f)
			_, err := f.fields(today, categories)
			var ve store.ValidationError
			if !errors.As(err, &ve) || !strings.Contains(ve.Error(), tc.want) {
				t.Fatalf("err=%v want %q", err, tc.want)
			}
		})
	}
}

func TestOperationalIssueChanges(t *testing.T) {
	closed := time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC)
	old := domain.OperationalIssue{
		IdentifiedOn: time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC),
		Category:     "utilities",
		Description:  "Water off",
		Status:       "resolved",
		ClosedOn:     &closed,
		ClosureNotes: "Fixed",
	}
	same := store.OperationalIssueFields{
		IdentifiedOn: old.IdentifiedOn,
		Category:     old.Category,
		Description:  old.Description,
		Status:       old.Status,
		ClosedOn:     &closed,
		ClosureNotes: old.ClosureNotes,
	}
	if got := operationalIssueChanges(old, same); len(got) != 0 {
		t.Fatalf("unchanged=%v", got)
	}
	reopen := same
	reopen.Status = "open"
	reopen.ClosedOn = nil
	reopen.ClosureNotes = ""
	reopen.Effect = "No showers"
	got := operationalIssueChanges(old, reopen)
	want := []string{"effect", "status", "closed_on", "closure_notes"}
	if !slices.Equal(got, want) {
		t.Fatalf("changes=%v want %v", got, want)
	}
}

func TestOperationalIssueFilterLinks(t *testing.T) {
	links := operationalIssueFilterLinks("resolved", 7)
	if len(links) != 4 {
		t.Fatalf("links=%+v", links)
	}
	if links[0].Href != "/operations?house=7" || links[0].Current {
		t.Fatalf("open link=%+v", links[0])
	}
	if links[1].Href != "/operations?house=7&status=resolved" || !links[1].Current {
		t.Fatalf("resolved link=%+v", links[1])
	}
	if links[3].Label != "All" || links[3].Href != "/operations?house=7&status=all" {
		t.Fatalf("all link=%+v", links[3])
	}
	if got := operationalIssueFilterLinks("open", 0)[0].Href; got != "/operations" {
		t.Fatalf("plain open href=%q", got)
	}
}
