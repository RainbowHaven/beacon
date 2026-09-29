package store

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

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

// PrepareNickname normalizes and validates a nickname for create/rename.
// Nicknames must be non-empty, start with a letter, and yield a non-empty uniqueness key.
func PrepareNickname(s string) (normalized, key string, err error) {
	normalized = NormalizeNickname(s)
	if normalized == "" {
		return "", "", ErrNicknameInvalid
	}
	r, _ := utf8.DecodeRuneInString(normalized)
	if !unicode.IsLetter(r) {
		return "", "", ErrNicknameInvalid
	}
	key = NicknameKey(normalized)
	if key == "" {
		return "", "", ErrNicknameInvalid
	}
	return normalized, key, nil
}

// NicknameSuggestionStem is the kebab stem used for alternatives (trailing digits stripped).
func NicknameSuggestionStem(s string) string {
	stem := strings.TrimRightFunc(NicknameKey(s), unicode.IsDigit)
	if stem == "" {
		return "resident"
	}
	r, _ := utf8.DecodeRuneInString(stem)
	if !unicode.IsLetter(r) {
		return "resident"
	}
	return stem
}

// KebabSuggestion formats stem or stem-N (N >= 2) for copyable alternatives.
func KebabSuggestion(stem string, n int) string {
	if n <= 1 {
		return stem
	}
	return fmt.Sprintf("%s-%d", stem, n)
}
