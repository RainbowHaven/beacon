package store

import "testing"

func TestNicknameKey(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		// Empty / whitespace
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"trim ends", "  tom  ", "tom"},
		{"collapse internal spaces", "Mary Jane", "maryjane"},
		{"strip all spaces", "T O M", "tom"},
		{"strip tabs between letters", "t  o\tm", "tom"},

		// Case
		{"lowercase", "tom", "tom"},
		{"mixed case", "Tom", "tom"},
		{"uppercase", "TOM", "tom"},

		// Diacritics / umlauts (stripped via NFKD)
		{"umlaut o U+00F6", "Alf\u00f6ns", "alfons"},
		{"umlaut O U+00D6", "ALF\u00d6NS", "alfons"},
		{"acute e U+00E9", "Jos\u00e9", "jose"},
		{"umlaut u U+00FC", "M\u00fcller", "muller"},
		{"plain matches umlaut fold", "Alfons", "alfons"},

		// Sharp s
		{"eszett U+00DF to ss", "stra\u00dfe", "strasse"},
		{"capital eszett U+1E9E to ss", "STRA\u1e9eE", "strasse"},

		// Cyrillic look-alikes (every mapped rune)
		{"cyrillic a U+0430", "\u0430", "a"},
		{"cyrillic e U+0435", "\u0435", "e"},
		{"cyrillic o U+043E", "\u043e", "o"},
		{"cyrillic er U+0440", "\u0440", "p"},
		{"cyrillic es U+0441", "\u0441", "c"},
		{"cyrillic u U+0443", "\u0443", "y"},
		{"cyrillic ha U+0445", "\u0445", "x"},
		{"cyrillic i U+0456", "\u0456", "i"},
		{"cyrillic je U+0458", "\u0458", "j"},
		{"cyrillic dze U+0455", "\u0455", "s"},
		{"cyrillic shha U+04BB", "\u04bb", "h"},
		{"cyrillic qa U+051B", "\u051b", "q"},
		{"cyrillic we U+051D", "\u051d", "w"},
		{"cyrillic komi de U+0501", "\u0501", "d"},
		{"latin alpha g U+0261", "\u0261", "g"},
		{"cyrillic o inside word", "t\u043em", "tom"},
		{"cyrillic a prefix", "\u0430lfons", "alfons"},
		{"all cyrillic look-alikes string", "\u0430\u0435\u043e\u0440\u0441\u0443\u0445\u0456\u0458\u0455\u04bb\u051b\u051d\u0501\u0261", "aeopcyxijshqwdg"},

		// Greek look-alikes (every mapped rune)
		{"greek alpha U+03B1", "\u03b1", "a"},
		{"greek omicron U+03BF", "\u03bf", "o"},
		{"greek nu U+03BD", "\u03bd", "v"},
		{"greek iota U+03B9", "\u03b9", "i"},
		{"greek eta U+03B7", "\u03b7", "n"},
		{"greek rho U+03C1", "\u03c1", "p"},
		{"greek tau U+03C4", "\u03c4", "t"},
		{"greek chi U+03C7", "\u03c7", "x"},
		{"greek kappa U+03BA", "\u03ba", "k"},
		{"greek mu U+03BC", "\u03bc", "m"},
		{"greek gamma U+03B3", "\u03b3", "y"},
		{"greek omicron inside word", "t\u03bfm", "tom"},
		{"all greek look-alikes string", "\u03b1\u03bf\u03bd\u03b9\u03b7\u03c1\u03c4\u03c7\u03ba\u03bc\u03b3", "aovinptxkmy"},

		// Punctuation kept in the key
		{"hyphen suffix", "tom-", "tom-"},
		{"en dash U+2013 suffix", "tom\u2013", "tom\u2013"},
		{"em dash U+2014 suffix", "tom\u2014", "tom\u2014"},
		{"hyphens between letters", "t-o-m", "t-o-m"},
		{"dot suffix", "tom.", "tom."},
		{"double dot suffix", "tom..", "tom.."},
		{"semicolon suffix", "tom;", "tom;"},
		{"colon suffix", "tom:", "tom:"},
		{"comma suffix", "tom,", "tom,"},
		{"exclamation suffix", "tom!", "tom!"},
		{"question suffix", "tom?", "tom?"},
		{"hash prefix", "#tom", "#tom"},
		{"hash infix", "tom#1", "tom#1"},
		{"slash infix", "tom/1", "tom/1"},
		{"backslash infix", `tom\1`, `tom\1`},
		{"pipe infix", "tom|1", "tom|1"},
		{"underscore infix", "tom_1", "tom_1"},
		{"apostrophe infix", "tom'1", "tom'1"},
		{"double quote infix", "tom\"1", "tom\"1"},
		{"parens", "tom(1)", "tom(1)"},
		{"brackets", "tom[1]", "tom[1]"},
		{"braces", "tom{1}", "tom{1}"},
		{"at sign", "tom@house", "tom@house"},
		{"plus", "tom+1", "tom+1"},
		{"equals", "tom=1", "tom=1"},
		{"asterisk", "tom*1", "tom*1"},
		{"percent", "tom%1", "tom%1"},
		{"ampersand", "tom&1", "tom&1"},
		{"caret", "tom^1", "tom^1"},
		{"tilde", "tom~1", "tom~1"},
		{"backtick", "tom`1", "tom`1"},
		{"angle brackets", "tom<1>", "tom<1>"},
		{"spaces around hyphen removed", "tom  -  1", "tom-1"},
		{"dotted initials", "T.O.M", "t.o.m"},
		{"umlaut plus hyphen", "Alf\u00f6ns-2", "alfons-2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NicknameKey(tc.in); got != tc.want {
				t.Fatalf("NicknameKey(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}

	t.Run("umlaut folds to plain", func(t *testing.T) {
		if NicknameKey("Alfons") != NicknameKey("Alf\u00f6ns") {
			t.Fatal("Alfons and Alföns must share a key")
		}
	})
	t.Run("cyrillic o folds to latin o", func(t *testing.T) {
		if NicknameKey("tom") != NicknameKey("t\u043em") {
			t.Fatal("Latin tom and Cyrillic-о tom must share a key")
		}
	})
	t.Run("greek o folds to latin o", func(t *testing.T) {
		if NicknameKey("tom") != NicknameKey("t\u03bfm") {
			t.Fatal("Latin tom and Greek-ο tom must share a key")
		}
	})

	t.Run("punctuation stays distinct from plain", func(t *testing.T) {
		for _, other := range []string{
			"tom-", "tom.", "tom;", "tom:", "tom,", "tom!", "tom?",
			"#tom", "tom#", "tom/1", `tom\1`, "tom_1", "tom'1",
			"t-o-m", "t.o.m", "tom(1)",
		} {
			if NicknameKey("tom") == NicknameKey(other) {
				t.Fatalf("plain tom must not collide with %q (key %q)", other, NicknameKey(other))
			}
		}
	})
}
