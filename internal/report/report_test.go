package report

import (
	"reflect"
	"testing"
	"time"

	"github.com/magiconair/beacon/internal/domain"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func datep(s string) *time.Time {
	t := date(s)
	return &t
}

func intp(n int) *int { return &n }

func TestBedNights(t *testing.T) {
	from, to := date("2025-03-01"), date("2025-04-01")
	tests := []struct {
		name     string
		arrived  string
		departed *time.Time
		want     int
	}{
		{"arrival before month, no departure", "2025-02-10", nil, 31},
		{"arrival before month, departure after month", "2025-02-10", datep("2025-04-05"), 31},
		{"arrival in month, departure after month", "2025-03-20", datep("2025-04-05"), 12},
		{"arrival before month, departure in month", "2025-02-10", datep("2025-03-10"), 9},
		{"same-day arrival and departure", "2025-03-15", datep("2025-03-15"), 0},
		{"departure on the 1st", "2025-02-20", datep("2025-03-01"), 0},
		{"arrival on the last day", "2025-03-31", nil, 1},
		{"departure on the 1st of next month", "2025-03-30", datep("2025-04-01"), 2},
		{"stay before month", "2025-01-01", datep("2025-02-01"), 0},
		{"arrival after month", "2025-04-02", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BedNights(date(tt.arrived), tt.departed, from, to); got != tt.want {
				t.Fatalf("got %d want %d", got, tt.want)
			}
		})
	}
}

func occ(nick, arrived string, departed *time.Time) domain.Occupant {
	return domain.Occupant{Nickname: nick, ArrivedAt: date(arrived), DepartedAt: departed, CountryOfOrigin: "NR", Gender: "NR"}
}

func TestBuild(t *testing.T) {
	type want struct {
		days, counted int
		inProgress    bool
		served        int
		bedNights     int
		admissions    int
		departures    int
		occupancy     string
		residents     []string
		nights        []int
	}
	tests := []struct {
		name      string
		month     string
		now       string
		places    *int
		occupants []domain.Occupant
		want      want
	}{
		{
			name:   "arrival before month and departure after month",
			month:  "2025-03-01",
			now:    "2025-06-01",
			places: intp(2),
			occupants: []domain.Occupant{
				occ("robin", "2025-02-10", datep("2025-04-05")),
			},
			want: want{days: 31, counted: 31, served: 1, bedNights: 31, occupancy: "50.0%",
				residents: []string{"robin"}, nights: []int{31}},
		},
		{
			name:   "no departure",
			month:  "2025-03-01",
			now:    "2025-06-01",
			places: intp(1),
			occupants: []domain.Occupant{
				occ("wren", "2025-03-11", nil),
			},
			want: want{days: 31, counted: 31, served: 1, bedNights: 21, admissions: 1, occupancy: "67.7%",
				residents: []string{"wren"}, nights: []int{21}},
		},
		{
			name:  "same-day arrival and departure is excluded but counted as admission and departure",
			month: "2025-03-01",
			now:   "2025-06-01",
			occupants: []domain.Occupant{
				occ("finch", "2025-03-15", datep("2025-03-15")),
			},
			want: want{days: 31, counted: 31, admissions: 1, departures: 1},
		},
		{
			name:  "departure on the 1st counts as departure only",
			month: "2025-03-01",
			now:   "2025-06-01",
			occupants: []domain.Occupant{
				occ("jay", "2025-02-20", datep("2025-03-01")),
			},
			want: want{days: 31, counted: 31, departures: 1},
		},
		{
			name:  "arrival on the last day is one night",
			month: "2025-03-01",
			now:   "2025-06-01",
			occupants: []domain.Occupant{
				occ("lark", "2025-03-31", nil),
			},
			want: want{days: 31, counted: 31, served: 1, bedNights: 1, admissions: 1,
				residents: []string{"lark"}, nights: []int{1}},
		},
		{
			name:   "february in a leap year",
			month:  "2024-02-01",
			now:    "2024-04-01",
			places: intp(1),
			occupants: []domain.Occupant{
				occ("kite", "2024-01-20", datep("2024-03-02")),
				occ("owl", "2024-02-28", datep("2024-03-01")),
			},
			want: want{days: 29, counted: 29, served: 2, bedNights: 31, admissions: 1, occupancy: "106.9%",
				residents: []string{"kite", "owl"}, nights: []int{29, 2}},
		},
		{
			name:   "month in progress counts nights before today",
			month:  "2025-03-01",
			now:    "2025-03-11T15:00:00Z",
			places: intp(2),
			occupants: []domain.Occupant{
				occ("rook", "2025-02-01", nil),
				occ("tern", "2025-03-09", datep("2025-03-11")),
				occ("gull", "2025-03-11", nil),
				occ("dove", "2025-03-12", nil),
			},
			want: want{days: 31, counted: 10, inProgress: true, served: 2, bedNights: 12, admissions: 2, departures: 1,
				occupancy: "60.0%", residents: []string{"rook", "tern"}, nights: []int{10, 2}},
		},
		{
			name:  "first day of the current month has no nights yet",
			month: "2025-03-01",
			now:   "2025-03-01",
			occupants: []domain.Occupant{
				occ("rook", "2025-02-01", nil),
			},
			want: want{days: 31, counted: 0, inProgress: true},
		},
		{
			name:  "occupancy not available without sleeping places",
			month: "2025-03-01",
			now:   "2025-06-01",
			occupants: []domain.Occupant{
				occ("rook", "2025-02-01", nil),
			},
			want: want{days: 31, counted: 31, served: 1, bedNights: 31,
				residents: []string{"rook"}, nights: []int{31}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tt.now)
			if err != nil {
				now = date(tt.now)
			}
			m := Build(date(tt.month), now, tt.places, tt.occupants)
			var names []string
			var nights []int
			for _, r := range m.Residents {
				names = append(names, r.Nickname)
				nights = append(nights, r.BedNights)
			}
			got := want{
				days: m.DaysInMonth, counted: m.NightsCounted, inProgress: m.InProgress,
				served: m.ResidentsServed, bedNights: m.BedNights,
				admissions: m.Admissions, departures: m.Departures,
				occupancy: m.OccupancyText(), residents: names, nights: nights,
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestBuildDemographics(t *testing.T) {
	by := func(y int) *int { return &y }
	occupants := []domain.Occupant{
		{Nickname: "robin", ArrivedAt: date("2025-03-02"), CountryOfOrigin: "UG", Gender: "M", BirthYear: by(1999)},
		{Nickname: "wren", ArrivedAt: date("2025-03-01"), CountryOfOrigin: "KE", Gender: "F"},
		{Nickname: "lark", ArrivedAt: date("2025-03-05"), CountryOfOrigin: "UG", Gender: "X"},
		{Nickname: "jay", ArrivedAt: date("2025-02-01"), DepartedAt: datep("2025-03-01"), CountryOfOrigin: "RW", Gender: "M"},
	}
	m := Build(date("2025-03-01"), date("2025-04-10"), nil, occupants)
	if m.Residents[0].Nickname != "wren" || m.Residents[0].Country != "Kenya" || m.Residents[0].Gender != "F" {
		t.Fatalf("first resident %+v", m.Residents[0])
	}
	if m.Residents[1].BirthYear == nil || *m.Residents[1].BirthYear != 1999 {
		t.Fatalf("birth year %+v", m.Residents[1])
	}
	wantGender := []Count{{"M", "M", 1}, {"F", "F", 1}, {"X", "X", 1}}
	if !reflect.DeepEqual(m.ByGender, wantGender) {
		t.Fatalf("by gender %+v", m.ByGender)
	}
	wantCountry := []Count{{"UG", "Uganda", 2}, {"KE", "Kenya", 1}}
	if !reflect.DeepEqual(m.ByCountry, wantCountry) {
		t.Fatalf("by country %+v", m.ByCountry)
	}
}
