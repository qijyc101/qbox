package config

import (
	"os"
	"path/filepath"
)

// AppName is the directory under the user's configuration and cache roots.
const AppName = "qbox"

// DefaultDir returns the per-user configuration directory:
// %AppData%\qbox on Windows, ~/.config/qbox elsewhere.
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AppName), nil
}

// CacheDir returns the per-user cache directory.
func CacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AppName), nil
}

// LogDir returns the directory for qbox's own log files.
func LogDir() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "logs"), nil
}
