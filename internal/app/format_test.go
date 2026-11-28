package app

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "<1 с"},
		{999 * time.Millisecond, "<1 с"},
		{time.Second, "1 с"},
		{59 * time.Second, "59 с"},
		{time.Minute, "1м 0с"},
		{time.Hour, "1ч 0м"},
		{2*time.Hour + 3*time.Minute, "2ч 3м"},
	}
	for _, tt := range cases {
		if got := formatDuration(tt.in); got != tt.want {
			t.Errorf("formatDuration(%s) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

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
