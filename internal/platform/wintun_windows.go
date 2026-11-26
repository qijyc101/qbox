//go:build windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/windows"
)

// PreloadWintun loads wintun.dll from the app's resources before sing-tun
// asks the system for it. sing-tun's loader searches the application
// directory and System32; preloading by absolute path makes the DLL
// available there without copying it next to the executable.
func PreloadWintun(resourcesDir string) error {
	if resourcesDir == "" {
		return fmt.Errorf("каталог ресурсов не определён")
	}
	arch := "amd64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	candidates := []string{
		filepath.Join(resourcesDir, "wintun.dll"),
		filepath.Join(resourcesDir, "windows-"+arch, "wintun.dll"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		handle, err := windows.LoadLibraryEx(path, 0,
			windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_DEFAULT_DIRS)
		if err != nil {
			return fmt.Errorf("загрузка %s: %w", path, err)
		}
		_ = handle // keep the module loaded for the process lifetime
		return nil
	}
	return fmt.Errorf("wintun.dll не найден в %s (запустите scripts\\fetch-wintun.ps1)", resourcesDir)
}
