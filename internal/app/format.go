package app

import (
	"fmt"
	"time"
)

const (
	secondsPerMinute = 60
	minutesPerHour   = 60
	bytesPerUnit     = 1024
)

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "<1 с"
	}
	seconds := int(d.Seconds())
	if seconds < secondsPerMinute {
		return fmt.Sprintf("%d с", seconds)
	}
	minutes := seconds / secondsPerMinute
	seconds %= secondsPerMinute
	if minutes < minutesPerHour {
		return fmt.Sprintf("%dм %dс", minutes, seconds)
	}
	hours := minutes / minutesPerHour
	minutes %= minutesPerHour
	return fmt.Sprintf("%dч %dм", hours, minutes)
}

func formatBytes(n int64) string {
	if n < bytesPerUnit {
		return fmt.Sprintf("%d Б", n)
	}
	units := []string{"КБ", "МБ", "ГБ", "ТБ"}
	div, exp := int64(bytesPerUnit), 0
	for m := n / bytesPerUnit; m >= bytesPerUnit && exp < len(units)-1; m /= bytesPerUnit {
		div *= bytesPerUnit
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), units[exp])
}
