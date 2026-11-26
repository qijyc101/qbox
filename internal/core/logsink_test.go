package core

import (
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/sagernet/sing-box/log"
)

func TestLogSinkRingAndClear(t *testing.T) {
	sink := NewLogSink("")
	defer sink.Close()
	for i := 0; i < maxLogEntries+10; i++ {
		sink.WriteMessage(log.LevelInfo, "line")
	}
	entries, rev := sink.Snapshot()
	if len(entries) != maxLogEntries {
		t.Fatalf("entries = %d", len(entries))
	}
	if rev == 0 || sink.Revision() != rev {
		t.Fatalf("revision = %d, sink = %d", rev, sink.Revision())
	}
	sink.Clear()
	entries, _ = sink.Snapshot()
	if len(entries) != 0 {
		t.Fatalf("entries after clear = %d", len(entries))
	}
}

func TestSanitizeLogMessage(t *testing.T) {
	long := strings.Repeat("я", maxLogMessage)
	got := sanitizeLogMessage(long)
	if !utf8.ValidString(got) {
		t.Fatal("truncated message is not valid UTF-8")
	}
	if len(got) > maxLogMessage+len("…") {
		t.Fatalf("message is too long: %d bytes", len(got))
	}
	spoof := sanitizeLogMessage("a\x00b\nc\u202ed\u200b")
	if spoof != "a b cd" {
		t.Fatalf("spoof = %q", spoof)
	}
}

func TestLogSinkConcurrentAccess(t *testing.T) {
	sink := NewLogSink(t.TempDir() + "/qbox.log")
	defer sink.Close()

	var wg sync.WaitGroup
	wg.Add(9)
	for writer := 0; writer < 8; writer++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				sink.WriteMessage(log.LevelInfo, "concurrent")
			}
		}()
	}
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			entries, _ := sink.Snapshot()
			if len(entries) > maxLogEntries {
				t.Errorf("entries = %d", len(entries))
				return
			}
			_ = sink.Revision()
		}
	}()
	wg.Wait()
	entries, _ := sink.Snapshot()
	if len(entries) != maxLogEntries {
		t.Fatalf("entries = %d", len(entries))
	}
}
