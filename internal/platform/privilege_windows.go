//go:build windows

// Package platform wraps the few Windows-specific operations qbox needs:
// checking for administrator rights and relaunching elevated.
package platform

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// IsElevated reports whether the current process runs with an elevated token.
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// WaitForProcessExit waits until the process with pid exits, or the timeout
// expires. A process that is already gone counts as exited.
func WaitForProcessExit(pid int, timeout time.Duration) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(handle)
	event, err := windows.WaitForSingleObject(handle, uint32(timeout.Milliseconds()))
	if err != nil {
		return err
	}
	if event == windows.WAIT_OBJECT_0 {
		return nil
	}
	return errors.New("старый экземпляр qbox не завершился")
}

// RelaunchElevated starts the current executable with the "runas" verb.
// The caller is expected to quit after a successful call.
func RelaunchElevated(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	argsLine, _ := windows.UTF16PtrFromString(strings.Join(quoteArgs(args), " "))
	var cwdPtr *uint16
	if cwd != "" {
		cwdPtr, _ = windows.UTF16PtrFromString(cwd)
	}
	if err := windows.ShellExecute(0, verb, file, argsLine, cwdPtr, windows.SW_SHOWNORMAL); err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return errors.New("запуск с правами администратора отменён")
		}
		return fmt.Errorf("не удалось перезапустить с правами администратора: %w", err)
	}
	return nil
}

func quoteArgs(args []string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		if strings.ContainsAny(arg, " \t\"") {
			out[i] = `"` + strings.ReplaceAll(arg, `"`, `\"`) + `"`
		} else {
			out[i] = arg
		}
	}
	return out
}
