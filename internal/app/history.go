package app

import (
	"time"

	"qbox/internal/model"
)

const trafficPoints = 60

type trafficPoint struct {
	Up   int64
	Down int64
}

// sampleTraffic appends a bytes-per-second sample once a second.
func (a *App) sampleTraffic(now time.Time, stats model.Stats) bool {
	if a.lastSample.IsZero() {
		a.lastStats = stats
		a.lastSample = now
		return false
	}
	elapsed := now.Sub(a.lastSample)
	if elapsed < 900*time.Millisecond {
		return false
	}
	up := stats.Up - a.lastStats.Up
	down := stats.Down - a.lastStats.Down
	if up < 0 {
		up = 0
	}
	if down < 0 {
		down = 0
	}
	seconds := elapsed.Seconds()
	if seconds <= 0 {
		return false
	}
	a.traffic = append(a.traffic, trafficPoint{
		Up:   int64(float64(up) / seconds),
		Down: int64(float64(down) / seconds),
	})
	if len(a.traffic) > trafficPoints {
		a.traffic = append([]trafficPoint(nil), a.traffic[len(a.traffic)-trafficPoints:]...)
	}
	a.lastStats = stats
	a.lastSample = now
	return true
}

// resetTraffic starts a fresh chart, keeping the counter baseline.
func (a *App) resetTraffic(stats model.Stats) {
	a.traffic = nil
	a.lastStats = stats
	a.lastSample = time.Now()
}

func (a *App) trafficPeak() int64 {
	var peak int64
	for _, point := range a.traffic {
		if point.Down > peak {
			peak = point.Down
		}
		if point.Up > peak {
			peak = point.Up
		}
	}
	return peak
}

func sameConnections(a, b []model.Connection) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Upload != b[i].Upload || a[i].Download != b[i].Download {
			return false
		}
	}
	return true
}
