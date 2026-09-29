package store

import "testing"

func TestNicknameKey(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"tom", "tom"},
		{"Tom", "tom"},
		{"  tom  ", "tom"},
		{"T O M", "tom"},
		{"t  o\tm", "tom"},
		{"Mary Jane", "maryjane"},
		{"Alfons", "alfons"},
		{"Alföns", "alfons"},
		{"ALFÖNS", "alfons"},
		{"José", "jose"},
		{"Müller", "muller"},
		{"straße", "strasse"},
		// Cyrillic look-alike о (U+043E) instead of Latin o
		{"tоm", "tom"},
		// Cyrillic а (U+0430)
		{"аlfons", "alfons"},
		{"", ""},
		{"   ", ""},

		// Punctuation is kept in the key (not stripped).
		{"tom-", "tom-"},
		{"tom–", "tom–"}, // en dash (U+2013)
		{"tom—", "tom—"}, // em dash (U+2014)
		{"t-o-m", "t-o-m"},
		{"tom.", "tom."},
		{"tom..", "tom.."},
		{"tom;", "tom;"},
		{"tom:", "tom:"},
		{"tom,", "tom,"},
		{"tom!", "tom!"},
		{"tom?", "tom?"},
		{"#tom", "#tom"},
		{"tom#1", "tom#1"},
		{"tom/1", "tom/1"},
		{`tom\1`, `tom\1`},
		{"tom|1", "tom|1"},
		{"tom_1", "tom_1"},
		{"tom'1", "tom'1"},
		{`tom"1`, `tom"1`},
		{"tom(1)", "tom(1)"},
		{"tom[1]", "tom[1]"},
		{"tom{1}", "tom{1}"},
		{"tom@house", "tom@house"},
		{"tom+1", "tom+1"},
		{"tom=1", "tom=1"},
		{"tom*1", "tom*1"},
		{"tom%1", "tom%1"},
		{"tom&1", "tom&1"},
		{"tom^1", "tom^1"},
		{"tom~1", "tom~1"},
		{"tom`1", "tom`1"},
		{"tom<1>", "tom<1>"},
		{"tom  -  1", "tom-1"}, // spaces removed around dash
		{"T.O.M", "t.o.m"},
		{"Alföns-2", "alfons-2"},
	}
	for _, tc := range cases {
		if got := NicknameKey(tc.in); got != tc.want {
			t.Fatalf("NicknameKey(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
	if NicknameKey("Alfons") != NicknameKey("Alföns") {
		t.Fatal("Alfons and Alföns must share a key")
	}
	// Cyrillic о (U+043E)
	if NicknameKey("tom") != NicknameKey("tоm") {
		t.Fatal("Latin tom and Cyrillic-о tom must share a key")
	}

	// Punctuation variants stay distinct from the plain nickname.
	for _, other := range []string{
		"tom-", "tom.", "tom;", "tom:", "tom,", "tom!", "tom?",
		"#tom", "tom#", "tom/1", `tom\1`, "tom_1", "tom'1",
		"t-o-m", "t.o.m", "tom(1)",
	} {
		if NicknameKey("tom") == NicknameKey(other) {
			t.Fatalf("plain tom must not collide with %q (key %q)", other, NicknameKey(other))
		}
	}
}
