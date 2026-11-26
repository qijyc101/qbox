// Package model holds the small types shared by the core engine and the UI.
// It deliberately imports nothing from sing-box or mygo so that view tests
// stay fast.
package model

import "time"

// State is the lifecycle state of the sing-box engine.
type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateError    State = "error"
)

// Stats is a snapshot of the traffic counters kept by sing-box.
type Stats struct {
	Up          int64
	Down        int64
	Connections int
}

// PingResult is the latency of one node, measured through its proxy
// protocol.
type PingResult struct {
	Tag     string
	Latency time.Duration
	Err     string
}

// LogEntry is one line of the engine log shown by the UI.
type LogEntry struct {
	Time    time.Time
	Level   string
	Message string
}
