// Package report builds the monthly operations report (formerly Document 50)
// from resident records. It does no I/O.
package report

import (
	"fmt"
	"sort"
	"time"

	"github.com/RainbowHaven/beacon/internal/demographics"
	"github.com/RainbowHaven/beacon/internal/domain"
)

const day = 24 * time.Hour

// Resident is one listed row: someone who stayed at least one night in the month.
type Resident struct {
	Nickname    string
	ArrivedAt   time.Time
	DepartedAt  *time.Time
	BedNights   int
	CountryCode string
	Country     string
	GenderCode  string
	Gender      string
	BirthYear   *int
}

// Count is one line of a summary breakdown.
type Count struct {
	Code  string
	Label string
	N     int
}

// Monthly is the report for one safe house and one calendar month.
type Monthly struct {
	Month          time.Time // first day of the month, UTC
	DaysInMonth    int
	NightsCounted  int  // nights up to today for a month in progress, else DaysInMonth
	InProgress     bool // the month has not ended yet
	SleepingPlaces *int

	Residents       []Resident
	ResidentsServed int
	BedNights       int
	Admissions      int
	Departures      int
	ByGender        []Count
	ByCountry       []Count
}

// MonthStart parses YYYY-MM into the first day of that month (UTC).
func MonthStart(s string) (time.Time, error) {
	return time.Parse("2006-01", s)
}

// BedNights counts the nights of a stay inside [from, to). The night of the
// departure date is not counted.
func BedNights(arrived time.Time, departed *time.Time, from, to time.Time) int {
	start := arrived.UTC().Truncate(day)
	if from.After(start) {
		start = from
	}
	end := to
	if departed != nil {
		if d := departed.UTC().Truncate(day); d.Before(end) {
			end = d
		}
	}
	if !end.After(start) {
		return 0
	}
	return int(end.Sub(start) / day)
}

// Build computes the report for month (any time within it) as of now.
// occupants must all belong to the same safe house.
func Build(month, now time.Time, sleepingPlaces *int, occupants []domain.Occupant) Monthly {
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	next := start.AddDate(0, 1, 0)
	today := now.UTC().Truncate(day)

	// Nights are counted before cutoff; admissions and departures up to lastDay.
	cutoff, lastDay := next, next.Add(-day)
	switch {
	case today.Before(start):
		cutoff, lastDay = start, start.Add(-day)
	case today.Before(next):
		cutoff, lastDay = today, today
	}

	m := Monthly{
		Month:          start,
		DaysInMonth:    int(next.Sub(start) / day),
		NightsCounted:  int(cutoff.Sub(start) / day),
		InProgress:     today.Before(next),
		SleepingPlaces: sleepingPlaces,
	}

	inMonth := func(t time.Time) bool {
		t = t.UTC().Truncate(day)
		return !t.Before(start) && !t.After(lastDay)
	}

	genders := map[string]int{}
	countries := map[string]int{}
	for _, o := range occupants {
		if inMonth(o.ArrivedAt) {
			m.Admissions++
		}
		if o.DepartedAt != nil && inMonth(*o.DepartedAt) {
			m.Departures++
		}
		n := BedNights(o.ArrivedAt, o.DepartedAt, start, cutoff)
		if n == 0 {
			continue
		}
		m.Residents = append(m.Residents, Resident{
			Nickname:    o.Nickname,
			ArrivedAt:   o.ArrivedAt,
			DepartedAt:  o.DepartedAt,
			BedNights:   n,
			CountryCode: o.CountryOfOrigin,
			Country:     demographics.CountryName(o.CountryOfOrigin),
			GenderCode:  o.Gender,
			Gender:      demographics.GenderName(o.Gender),
			BirthYear:   o.BirthYear,
		})
		m.BedNights += n
		genders[o.Gender]++
		countries[o.CountryOfOrigin]++
	}
	m.ResidentsServed = len(m.Residents)
	sort.Slice(m.Residents, func(i, j int) bool {
		a, b := m.Residents[i], m.Residents[j]
		if !a.ArrivedAt.Equal(b.ArrivedAt) {
			return a.ArrivedAt.Before(b.ArrivedAt)
		}
		return a.Nickname < b.Nickname
	})

	for _, g := range []string{demographics.GenderMale, demographics.GenderFemale, demographics.GenderX, demographics.GenderNotReported} {
		if n := genders[g]; n > 0 {
			m.ByGender = append(m.ByGender, Count{Code: g, Label: demographics.GenderName(g), N: n})
			delete(genders, g)
		}
	}
	for g, n := range genders {
		m.ByGender = append(m.ByGender, Count{Code: g, Label: demographics.GenderName(g), N: n})
	}
	for c, n := range countries {
		m.ByCountry = append(m.ByCountry, Count{Code: c, Label: demographics.CountryName(c), N: n})
	}
	sort.Slice(m.ByCountry, func(i, j int) bool {
		a, b := m.ByCountry[i], m.ByCountry[j]
		if a.N != b.N {
			return a.N > b.N
		}
		return a.Label < b.Label
	})
	return m
}

// Occupancy is bed-nights divided by approved sleeping places times the nights
// counted so far. ok is false when sleeping places are not set.
func (m Monthly) Occupancy() (ratio float64, ok bool) {
	if m.SleepingPlaces == nil || *m.SleepingPlaces <= 0 || m.NightsCounted == 0 {
		return 0, false
	}
	return float64(m.BedNights) / float64(*m.SleepingPlaces*m.NightsCounted), true
}

// OccupancyText formats Occupancy as a percentage with one decimal, or ""
// when it is not available.
func (m Monthly) OccupancyText() string {
	r, ok := m.Occupancy()
	if !ok {
		return ""
	}
	return fmt.Sprintf("%.1f%%", r*100)
}

// MonthKey is the month as YYYY-MM.
func (m Monthly) MonthKey() string { return m.Month.Format("2006-01") }

// MonthLabel is the month as e.g. "February 2024".
func (m Monthly) MonthLabel() string { return m.Month.Format("January 2006") }

// LastCountedDay is the last date whose night is included.
func (m Monthly) LastCountedDay() time.Time {
	return m.Month.AddDate(0, 0, m.NightsCounted-1)
}
