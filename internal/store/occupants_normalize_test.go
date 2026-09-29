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
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		if got := NicknameKey(tc.in); got != tc.want {
			t.Fatalf("NicknameKey(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}
