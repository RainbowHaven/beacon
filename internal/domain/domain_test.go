package domain

import (
	"strings"
	"testing"
)

func TestNormalizeRHLCode(t *testing.T) {
	cases := []struct {
		name, in, want string
		ok             bool
	}{
		{"empty", "", "", true},
		{"whitespace only", "   ", "", true},
		{"upper", "TOR", "TOR", true},
		{"lower to upper", "tor", "TOR", true},
		{"trim", "  van-1 ", "VAN-1", true},
		{"digits", "42", "42", true},
		{"min length", "AB", "AB", true},
		{"max length", "ABCDEFGHIJKL", "ABCDEFGHIJKL", true},
		{"too short", "a", "", false},
		{"too long", "ABCDEFGHIJKLM", "", false},
		{"inner space", "TO R", "", false},
		{"underscore", "TO_R", "", false},
		{"umlaut", "MÜN", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := NormalizeRHLCode(c.in)
			if got != c.want || ok != c.ok {
				t.Fatalf("NormalizeRHLCode(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestNormalizeIncidentID(t *testing.T) {
	cases := []struct {
		name, in, want string
		ok             bool
	}{
		{"document 37 format", "Incident-TOR-2026-1", "Incident-TOR-2026-1", true},
		{"trim", "  Incident-TOR-2026-2 ", "Incident-TOR-2026-2", true},
		{"free form", "TOR 2026 #3", "TOR 2026 #3", true},
		{"max length", strings.Repeat("x", 40), strings.Repeat("x", 40), true},
		{"multibyte counts runes", strings.Repeat("é", 40), strings.Repeat("é", 40), true},
		{"empty", "", "", false},
		{"whitespace only", "  \t ", "", false},
		{"too long", strings.Repeat("x", 41), "", false},
		{"newline", "Incident-TOR\n2026-1", "", false},
		{"carriage return", "Incident-TOR\r2026-1", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := NormalizeIncidentID(c.in)
			if got != c.want || ok != c.ok {
				t.Fatalf("NormalizeIncidentID(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
			}
		})
	}
	if got := IncidentID("TOR", 2026, 7); got != "Incident-TOR-2026-7" {
		t.Fatalf("IncidentID = %q", got)
	}
}

func TestSafeguardingStatusAndPrecision(t *testing.T) {
	for _, s := range SafeguardingStatuses {
		if !s.Valid() || s.Label() == "" {
			t.Fatalf("status %q", s)
		}
	}
	if SafeguardingStatus("deleted").Valid() || SafeguardingStatus("").Valid() {
		t.Fatal("unexpected valid status")
	}
	for _, p := range []DatePrecision{DateExact, DateApproximate, DateUnknown} {
		if !p.Valid() {
			t.Fatalf("precision %q", p)
		}
	}
	if DatePrecision("roughly").Valid() {
		t.Fatal("unexpected valid precision")
	}
}

func TestRHLLabel(t *testing.T) {
	if got := (RHL{Name: "Toronto"}).Label(); got != "Toronto" {
		t.Fatalf("got %q", got)
	}
	if got := (RHL{Name: "Toronto", Code: "TOR"}).Label(); got != "Toronto (TOR)" {
		t.Fatalf("got %q", got)
	}
}
