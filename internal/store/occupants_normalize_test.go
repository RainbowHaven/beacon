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
		{"t\u043em", "tom"},
		// Cyrillic а
		{"\u0430lfons", "alfons"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		if got := NicknameKey(tc.in); got != tc.want {
			t.Fatalf("NicknameKey(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
	if NicknameKey("Alfons") != NicknameKey("Alföns") {
		t.Fatal("Alfons and Alföns must share a key")
	}
	if NicknameKey("tom") != NicknameKey("t\u043em") {
		t.Fatal("Latin tom and Cyrillic-о tom must share a key")
	}
}
