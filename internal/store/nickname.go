package store

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var multiSpace = regexp.MustCompile(`\s+`)

// NormalizeNickname trims ends and collapses internal whitespace for storage/display.
func NormalizeNickname(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return multiSpace.ReplaceAllString(s, " ")
}

// confusableLatin maps lowercase look-alike letters (Cyrillic, Greek, etc.) to Latin.
// Display is unchanged; this is only for NicknameKey uniqueness.
var confusableLatin = map[rune]rune{
	// Cyrillic
	'а': 'a', 'е': 'e', 'о': 'o', 'р': 'p', 'с': 'c', 'у': 'y', 'х': 'x',
	'і': 'i', 'ј': 'j', 'ѕ': 's', 'һ': 'h', 'ԛ': 'q', 'ԝ': 'w',
	'ԁ': 'd', 'ɡ': 'g',
	// Greek
	'α': 'a', 'ο': 'o', 'ν': 'v', 'ι': 'i', 'η': 'n', 'ρ': 'p', 'τ': 't', 'χ': 'x',
	'κ': 'k', 'μ': 'm', 'γ': 'y',
}

// NicknameKey is the uniqueness key for a house:
// letters/digits only after folding — whitespace and punctuation ignored,
// diacritics stripped, common look-alikes mapped to Latin, lowercased.
// Examples: "tom!", "T-o-M", "T O M", "tоm" → "tom"; "Alföns" → "alfons".
func NicknameKey(s string) string {
	s = NormalizeNickname(s)
	if s == "" {
		return ""
	}
	s = multiSpace.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "ß", "ss")
	s = strings.ReplaceAll(s, "ẞ", "ss")

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFKD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		r = unicode.ToLower(r)
		if m, ok := confusableLatin[r]; ok {
			r = m
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
