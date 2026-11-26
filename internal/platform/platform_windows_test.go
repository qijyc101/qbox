//go:build windows

package platform

import (
	"testing"
	"time"
)

func TestElevationCheckDoesNotPanic(t *testing.T) {
	_ = IsElevated()
}

func TestWaitForMissingProcess(t *testing.T) {
	if err := WaitForProcessExit(999999, time.Second); err != nil {
		t.Fatal(err)
	}
}
