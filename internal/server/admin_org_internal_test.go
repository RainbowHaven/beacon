package server

import "testing"

func TestParseSleepingPlaces(t *testing.T) {
	cases := []struct {
		in   string
		want int // 0 means nil
		ok   bool
	}{
		{"", 0, true},
		{"  ", 0, true},
		{"1", 1, true},
		{" 12 ", 12, true},
		{"100", 100, true},
		{"0", 0, false},
		{"-3", 0, false},
		{"2.5", 0, false},
		{"abc", 0, false},
		{"101", 0, false},
		{"99999999999", 0, false},
	}
	for _, c := range cases {
		got, ok := parseSleepingPlaces(c.in)
		if ok != c.ok {
			t.Fatalf("%q: ok=%v want %v", c.in, ok, c.ok)
		}
		switch {
		case c.want == 0 && got != nil:
			t.Fatalf("%q: got %d want nil", c.in, *got)
		case c.want != 0 && (got == nil || *got != c.want):
			t.Fatalf("%q: got %v want %d", c.in, got, c.want)
		}
	}
}
