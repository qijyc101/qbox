//go:build !windows

package platform

import (
	"errors"
	"os"
	"time"
)

// IsElevated reports whether the process is root on Unix-like systems.
func IsElevated() bool {
	return os.Geteuid() == 0
}

// RelaunchElevated is not implemented outside Windows in this MVP.
func RelaunchElevated(args ...string) error {
	return errors.New("автоматический перезапуск с правами доступен только в Windows-сборке")
}

// WaitForProcessExit is a no-op outside Windows.
func WaitForProcessExit(pid int, timeout time.Duration) error {
	return nil
}
