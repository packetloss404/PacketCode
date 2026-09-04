package tools

import "testing"

func TestIsSecretFilePath(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{".env", true},
		{"./.env", true},
		{"sub/dir/.env", true},
		{`sub\dir\.env`, true},
		{".env.local", true},
		{".env.production", true},
		{".ENV", true},
		{".env.example", false},
		{".env.sample", false},
		{".env.template", false},
		{".env.dist", false},
		{"env", false},
		{"main.env", false},
		{"config/.envrc", false},
		{"", false},
	} {
		if got := IsSecretFilePath(tc.name); got != tc.want {
			t.Errorf("IsSecretFilePath(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
