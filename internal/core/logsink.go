package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/sagernet/sing-box/log"

	"qbox/internal/model"
)

const (
	maxLogEntries = 3000
	maxLogFile    = 5 << 20
	maxLogMessage = 8 << 10
)

// LogSink receives sing-box log messages, keeps a ring buffer for the UI
// and writes a rotating file.
type LogSink struct {
	mu      sync.Mutex
	entries []model.LogEntry
	start   int
	count   int
	rev     uint64
	file    *os.File
	path    string
	size    int64
}

// NewLogSink creates a sink writing to path (empty disables the file).
func NewLogSink(path string) *LogSink {
	s := &LogSink{path: path, entries: make([]model.LogEntry, maxLogEntries)}
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
	message = sanitizeLogMessage(message)
	entry := model.LogEntry{
		Time:    time.Now(),
		Level:   log.FormatLevel(level),
		Message: message,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.count < maxLogEntries {
		s.entries[(s.start+s.count)%maxLogEntries] = entry
		s.count++
	} else {
		s.entries[s.start] = entry
		s.start = (s.start + 1) % maxLogEntries
	}
	s.rev++
	s.writeFileLocked(entry)
}

func sanitizeLogMessage(message string) string {
	message = strings.TrimRight(message, "\r\n")
	if len(message) > maxLogMessage {
		message = message[:maxLogMessage]
		for len(message) > 0 && !utf8.ValidString(message) {
			message = message[:len(message)-1]
		}
		message += "…"
	}
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\t' || (r < 0x20) || r == 0x7f {
			return ' '
		}
		// Drop zero-width and bidirectional control characters so a remote
		// server name cannot visually rewrite the log.
		if r == '\u200b' || r == '\u200c' || r == '\u200d' || r == '\u200e' || r == '\u200f' ||
			(r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069') || r == '\ufeff' {
			return -1
		}
		return r
	}, message)
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
	oldSize := s.size
	_ = s.file.Close()
	_ = os.Remove(s.path + ".1")
	if err := os.Rename(s.path, s.path+".1"); err != nil {
		// Keep appending to the old file rather than silently losing the
		// rotation state; retry on a later write.
		file, openErr := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if openErr != nil {
			s.file = nil
			s.size = 0
			return
		}
		s.file = file
		s.size = oldSize
		return
	}
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
	out := make([]model.LogEntry, s.count)
	for i := 0; i < s.count; i++ {
		out[i] = s.entries[(s.start+i)%maxLogEntries]
	}
	return out, s.rev
}

// Revision returns the current revision without copying entries.
func (s *LogSink) Revision() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rev
}

// Clear empties the UI buffer but keeps the file.
func (s *LogSink) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.start = 0
	s.count = 0
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
