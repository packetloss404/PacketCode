package app

import "testing"

func TestActiveMentionToken(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		wantStart int
		wantQuery string
		wantOK    bool
	}{
		{name: "empty", text: "", wantOK: false},
		{name: "bare @", text: "@", wantStart: 0, wantQuery: "", wantOK: true},
		{name: "mid-sentence mention", text: "see @ma", wantStart: 4, wantQuery: "ma", wantOK: true},
		{name: "@ glued to word is not a trigger", text: "foo@bar", wantOK: false},
		{name: "second token", text: "@a @b", wantStart: 3, wantQuery: "b", wantOK: true},
		{name: "trailing space ends the token", text: "@a ", wantOK: false},
		{name: "home-relative path", text: "~/x", wantOK: false},
		{name: "home-relative mention", text: "@~/x", wantStart: 0, wantQuery: "~/x", wantOK: true},
		{name: "path mention", text: "open @internal/app/app.go", wantStart: 5, wantQuery: "internal/app/app.go", wantOK: true},
		{name: "illegal char closes popup", text: "@a$b", wantOK: false},
		{name: "space in query closes", text: "@a b", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, query, ok := activeMentionToken(tc.text)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (text %q)", ok, tc.wantOK, tc.text)
			}
			if !ok {
				return
			}
			if start != tc.wantStart {
				t.Fatalf("start = %d, want %d (text %q)", start, tc.wantStart, tc.text)
			}
			if query != tc.wantQuery {
				t.Fatalf("query = %q, want %q (text %q)", query, tc.wantQuery, tc.text)
			}
			// start must point at the "@".
			if tc.text[start] != '@' {
				t.Fatalf("text[start] = %q, want '@'", tc.text[start])
			}
		})
	}
}
