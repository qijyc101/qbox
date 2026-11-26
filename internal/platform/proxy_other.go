//go:build !windows

package platform

// NewSystemProxy is a no-op outside Windows: qbox only runs there.
func NewSystemProxy(path string) SystemProxy {
	return NoopSystemProxy{}
}
