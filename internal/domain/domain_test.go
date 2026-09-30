package domain

import "testing"

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

func TestRHLLabel(t *testing.T) {
	if got := (RHL{Name: "Toronto"}).Label(); got != "Toronto" {
		t.Fatalf("got %q", got)
	}
	if got := (RHL{Name: "Toronto", Code: "TOR"}).Label(); got != "Toronto (TOR)" {
		t.Fatalf("got %q", got)
	}
}
