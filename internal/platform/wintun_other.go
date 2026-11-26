//go:build !windows

package platform

// PreloadWintun is a no-op outside Windows.
func PreloadWintun(resourcesDir string) error {
	return nil
}
