package server

import "testing"

func TestParseMoneyToCents(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"12", 1200},
		{"12.5", 1250},
		{"12.50", 1250},
		{"$3.09", 309},
		{"0.01", 1},
		{"1,234.56", 123456},
	}
	for _, c := range cases {
		got, err := parseMoneyToCents(c.in)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("%q: got %d want %d", c.in, got, c.want)
		}
	}
	if _, err := parseMoneyToCents("12.999"); err == nil {
		t.Fatal("expected error for three decimal places")
	}
}

func TestFormatCents(t *testing.T) {
	if formatCents(1250) != "12.50" {
		t.Fatalf("got %q", formatCents(1250))
	}
	if formatCents(-5) != "-0.05" {
		t.Fatalf("got %q", formatCents(-5))
	}
	if formatCents(214000) != "2,140.00" {
		t.Fatalf("got %q", formatCents(214000))
	}
}

func TestParseUSDate(t *testing.T) {
	got, err := parseUSDate("09/10/2026")
	if err != nil {
		t.Fatal(err)
	}
	if formatUSDate(got) != "09/10/2026" {
		t.Fatalf("got %q", formatUSDate(got))
	}
	if _, err := parseUSDate("2026-09-10"); err != nil {
		t.Fatal(err)
	}
}
