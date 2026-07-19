package mcp

import (
	"math"
	"testing"
)

func TestFmtInt64Boundaries(t *testing.T) {
	cases := []struct {
		value int64
		want  string
	}{
		{math.MinInt64, "-9223372036854775808"},
		{-1, "-1"},
		{0, "0"},
		{1, "1"},
		{math.MaxInt64, "9223372036854775807"},
	}
	for _, tc := range cases {
		if got := string(fmtInt64(tc.value)); got != tc.want {
			t.Errorf("fmtInt64(%d) = %q, want %q", tc.value, got, tc.want)
		}
		request := newRequest(tc.value, "test", nil)
		if got := string(request.ID); got != tc.want {
			t.Errorf("newRequest(%d).ID = %q, want %q", tc.value, got, tc.want)
		}
	}
}
