package app

import "testing"

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 Б"},
		{1023, "1023 Б"},
		{1024, "1.0 КБ"},
		{1024 * 1024, "1.0 МБ"},
		{1024 * 1024 * 1024, "1.0 ГБ"},
		{1024 * 1024 * 1024 * 1024, "1.0 ТБ"},
	}
	for _, tt := range cases {
		if got := formatBytes(tt.in); got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
