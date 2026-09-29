package demographics

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Special country-of-origin codes (not ISO 3166-1).
const (
	CountryNotReported = "NR"
	CountryUnknown     = "UNK"
	CountryStateless   = "XXA" // UNHCR-style convention for Stateless
)

// Gender codes per requirements §2.1.
const (
	GenderMale         = "M"
	GenderFemale       = "F"
	GenderX            = "X"
	GenderNotReported  = "NR"
)

// Country is a selectable country-of-origin option.
type Country struct {
	Code string
	Name string
}

// Countries returns the full selectable list: specials first, then ISO countries.
func Countries() []Country {
	out := make([]Country, 0, len(isoCountries)+3)
	out = append(out,
		Country{Code: CountryNotReported, Name: "Not reported"},
		Country{Code: CountryUnknown, Name: "Unknown"},
		Country{Code: CountryStateless, Name: "Stateless"},
	)
	out = append(out, isoCountries...)
	return out
}

// Genders returns selectable gender options.
func Genders() []struct{ Code, Name string } {
	return []struct{ Code, Name string }{
		{GenderNotReported, "Not reported"},
		{GenderMale, "M"},
		{GenderFemale, "F"},
		{GenderX, "X"},
	}
}

// NormalizeCountry returns a valid country code or an error.
func NormalizeCountry(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if code == "" {
		return CountryNotReported, nil
	}
	for _, c := range Countries() {
		if c.Code == code {
			return code, nil
		}
	}
	return "", fmt.Errorf("unknown country code %q", raw)
}

// NormalizeGender returns a valid gender code or an error.
func NormalizeGender(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if code == "" {
		return GenderNotReported, nil
	}
	switch code {
	case GenderMale, GenderFemale, GenderX, GenderNotReported:
		return code, nil
	default:
		return "", fmt.Errorf("unknown gender %q", raw)
	}
}

// ParseBirthYear parses an optional year of birth. Empty means not reported (nil).
func ParseBirthYear(raw string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	y, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid year of birth")
	}
	max := time.Now().UTC().Year()
	if y < 1900 || y > max {
		return nil, fmt.Errorf("year of birth must be between 1900 and %d", max)
	}
	return &y, nil
}

// CountryName looks up a display name for a stored code.
func CountryName(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, c := range Countries() {
		if c.Code == code {
			return c.Name
		}
	}
	if code == "" {
		return "Not reported"
	}
	return code
}

// GenderName looks up a display name for a stored code.
func GenderName(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, g := range Genders() {
		if g.Code == code {
			return g.Name
		}
	}
	if code == "" {
		return "Not reported"
	}
	return code
}
