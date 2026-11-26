//go:build windows

// Package platform wraps the few Windows-specific operations qbox needs:
// checking for administrator rights, relaunching elevated, and preloading
// the Wintun driver DLL before sing-box opens a TUN device.
package platform

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// IsElevated reports whether the current process runs with an elevated token.
func IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
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
