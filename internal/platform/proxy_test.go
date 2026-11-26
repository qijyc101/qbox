package platform

import (
	"errors"
	"path/filepath"
	"testing"
)

type fakeProxyBackend struct {
	settings ProxySettings
	reads    int
	writes   int
	notifies int
	readErr  error
	writeErr error
}

func (f *fakeProxyBackend) Read() (ProxySettings, error) {
	f.reads++
	if f.readErr != nil {
		return ProxySettings{}, f.readErr
	}
	return f.settings, nil
}

func (f *fakeProxyBackend) Write(settings ProxySettings) error {
	f.writes++
	if f.writeErr != nil {
		return f.writeErr
	}
	f.settings = settings
	return nil
}

func (f *fakeProxyBackend) Notify() error {
	f.notifies++
	return nil
}

func TestProxyManagerEnableRestore(t *testing.T) {
	backend := &fakeProxyBackend{settings: ProxySettings{
		ProxyEnable:   1,
		ProxyServer:   "corp-proxy:8080",
		ProxyOverride: "*.corp.example",
		AutoConfigURL: "http://pac.corp/proxy.pac",
		AutoDetect:    1,
	}}
	manager := newProxyManager(filepath.Join(t.TempDir(), "proxy.json"), backend)

	if err := manager.Enable("127.0.0.1:2080"); err != nil {
		t.Fatal(err)
	}
	if !manager.Pending() {
		t.Fatal("no backup after enable")
	}
	if backend.settings.ProxyServer != "127.0.0.1:2080" || backend.settings.ProxyEnable != 1 {
		t.Fatalf("settings after enable = %+v", backend.settings)
	}
	if backend.settings.AutoConfigURL != "" || backend.settings.AutoDetect != 0 {
		t.Fatalf("PAC was not cleared: %+v", backend.settings)
	}

	if err := manager.Restore(); err != nil {
		t.Fatal(err)
	}
	if manager.Pending() {
		t.Fatal("backup was not removed")
	}
	if backend.settings.ProxyServer != "corp-proxy:8080" ||
		backend.settings.ProxyOverride != "*.corp.example" ||
		backend.settings.AutoConfigURL != "http://pac.corp/proxy.pac" ||
		backend.settings.AutoDetect != 1 {
		t.Fatalf("settings after restore = %+v", backend.settings)
	}
}

func TestProxyManagerKeepsForeignProxy(t *testing.T) {
	backend := &fakeProxyBackend{settings: ProxySettings{ProxyEnable: 1, ProxyServer: "original:8080"}}
	manager := newProxyManager(filepath.Join(t.TempDir(), "proxy.json"), backend)
	if err := manager.Enable("127.0.0.1:2080"); err != nil {
		t.Fatal(err)
	}

	// The user (or another app) switched to a different proxy after qbox.
	backend.settings = ProxySettings{ProxyEnable: 1, ProxyServer: "other:3128"}
	if err := manager.Restore(); err != nil {
		t.Fatal(err)
	}
	if backend.settings.ProxyServer != "other:3128" {
		t.Fatalf("foreign proxy was overwritten: %+v", backend.settings)
	}
	if manager.Pending() {
		t.Fatal("backup was not removed")
	}
}

func TestProxyManagerRollsBackOnWriteError(t *testing.T) {
	backend := &fakeProxyBackend{settings: ProxySettings{ProxyEnable: 1, ProxyServer: "original:8080"}}
	manager := newProxyManager(filepath.Join(t.TempDir(), "proxy.json"), backend)
	backend.writeErr = errors.New("access denied")
	if err := manager.Enable("127.0.0.1:2080"); err == nil {
		t.Fatal("enable succeeded despite the write error")
	}
	if manager.Pending() {
		t.Fatal("a failed enable left a backup")
	}
}

func TestProxyManagerRestoreAfterCrash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxy.json")
	first := &fakeProxyBackend{settings: ProxySettings{ProxyEnable: 1, ProxyServer: "original:8080"}}
	manager := newProxyManager(path, first)
	if err := manager.Enable("127.0.0.1:2080"); err != nil {
		t.Fatal(err)
	}

	// Simulate a new process after a crash: the backup is on disk and the
	// system proxy still points at qbox.
	secondBackend := &fakeProxyBackend{settings: first.settings}
	second := newProxyManager(path, secondBackend)
	if !second.Pending() {
		t.Fatal("backup was not detected")
	}
	if err := second.Restore(); err != nil {
		t.Fatal(err)
	}
	if secondBackend.settings.ProxyServer != "original:8080" {
		t.Fatalf("settings = %+v", secondBackend.settings)
	}
}
