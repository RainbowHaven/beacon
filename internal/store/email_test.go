package store

import (
	"errors"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	for in, want := range map[string]string{
		"a@example.org":             "a@example.org",
		"  Mixed.Case@Ex.ORG  ":     "mixed.case@ex.org",
		"first+tag@sub.example.org": "first+tag@sub.example.org",
	} {
		got, err := NormalizeEmail(in)
		if err != nil || got != want {
			t.Fatalf("NormalizeEmail(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "plain", "@example.org", "a@localhost", "Name <a@example.org>", "a@b@example.org", "a b@example.org"} {
		if _, err := NormalizeEmail(in); !errors.Is(err, ErrEmailInvalid) {
			t.Fatalf("NormalizeEmail(%q) err = %v; want ErrEmailInvalid", in, err)
		}
	}
}
