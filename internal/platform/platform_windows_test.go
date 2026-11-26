//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreloadWintun(t *testing.T) {
	resources, err := filepath.Abs(filepath.Join("..", "..", "resources"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(resources, "windows-amd64", "wintun.dll")); err != nil {
		t.Skip("wintun.dll is not present in resources")
	}
	if err := PreloadWintun(resources); err != nil {
		t.Fatal(err)
	}
}

func TestElevationCheckDoesNotPanic(t *testing.T) {
	_ = IsElevated()
}
