package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sagernet/sing-box/log"

	"qbox/internal/model"
)

const (
	maxLogEntries = 3000
	maxLogFile    = 5 << 20
)

// LogSink receives sing-box log messages, keeps a ring buffer for the UI
// and writes a rotating file.
type LogSink struct {
	mu      sync.Mutex
	entries []model.LogEntry
	rev     uint64
	file    *os.File
	path    string
	size    int64
}

// NewLogSink creates a sink writing to path (empty disables the file).
func NewLogSink(path string) *LogSink {
	s := &LogSink{path: path}
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
			if file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
				s.file = file
				if info, err := file.Stat(); err == nil {
					s.size = info.Size()
				}
			}
		}
	}
	return s
}

// WriteMessage implements log.PlatformWriter.
func (s *LogSink) WriteMessage(level log.Level, message string) {
	message = strings.TrimRight(message, "\r\n")
	entry := model.LogEntry{
		Time:    time.Now(),
		Level:   log.FormatLevel(level),
		Message: message,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = append(s.entries, entry)
	if len(s.entries) > maxLogEntries {
		copy(s.entries, s.entries[len(s.entries)-maxLogEntries:])
		s.entries = s.entries[:maxLogEntries]
	}
	s.rev++
	s.writeFileLocked(entry)
}

func (s *LogSink) writeFileLocked(entry model.LogEntry) {
	if s.file == nil {
		return
	}
	line := fmt.Sprintf("%s [%s] %s\n", entry.Time.Format("2006-01-02 15:04:05"), entry.Level, entry.Message)
	n, err := s.file.WriteString(line)
	if err != nil {
		return
	}
	s.size += int64(n)
	if s.size > maxLogFile {
		s.rotateLocked()
	}
}

func (s *LogSink) rotateLocked() {
	if s.file == nil {
		return
	}
	_ = s.file.Close()
	_ = os.Remove(s.path + ".1")
	_ = os.Rename(s.path, s.path+".1")
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		s.file = nil
		s.size = 0
		return
	}
	s.file = file
	s.size = 0
}

// Snapshot returns a copy of the buffered entries and the revision.
func (s *LogSink) Snapshot() ([]model.LogEntry, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.LogEntry, len(s.entries))
	copy(out, s.entries)
	return out, s.rev
}

// Clear empties the UI buffer but keeps the file.
func (s *LogSink) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
	s.rev++
}

// Close closes the log file.
func (s *LogSink) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
}
