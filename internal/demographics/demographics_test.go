package demographics_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/RainbowHaven/beacon/internal/demographics"
)

func TestNormalizeCountry(t *testing.T) {
	got, err := demographics.NormalizeCountry("")
	if err != nil || got != demographics.CountryNotReported {
		t.Fatalf("empty: %q %v", got, err)
	}
	got, err = demographics.NormalizeCountry("de")
	if err != nil || got != "DE" {
		t.Fatalf("de: %q %v", got, err)
	}
	got, err = demographics.NormalizeCountry("XXA")
	if err != nil || got != demographics.CountryStateless {
		t.Fatalf("stateless: %q %v", got, err)
	}
	if _, err := demographics.NormalizeCountry("ZZZ"); err == nil {
		t.Fatal("expected error for ZZZ")
	}
	if n := len(demographics.Countries()); n < 250 {
		t.Fatalf("countries=%d", n)
	}
}

func TestNormalizeGender(t *testing.T) {
	got, err := demographics.NormalizeGender("")
	if err != nil || got != demographics.GenderNotReported {
		t.Fatalf("empty: %q %v", got, err)
	}
	got, err = demographics.NormalizeGender("x")
	if err != nil || got != demographics.GenderX {
		t.Fatalf("x: %q %v", got, err)
	}
	if _, err := demographics.NormalizeGender("Z"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseBirthYear(t *testing.T) {
	y, err := demographics.ParseBirthYear("")
	if err != nil || y != nil {
		t.Fatalf("empty: %v %v", y, err)
	}
	y, err = demographics.ParseBirthYear("1990")
	if err != nil || y == nil || *y != 1990 {
		t.Fatalf("1990: %v %v", y, err)
	}
	if _, err := demographics.ParseBirthYear("1800"); err == nil {
		t.Fatal("expected low year error")
	}
	future := time.Now().UTC().Year() + 1
	if _, err := demographics.ParseBirthYear(strconv.Itoa(future)); err == nil {
		t.Fatal("expected future year error")
	}
}
