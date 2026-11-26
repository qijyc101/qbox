//go:build windows

package platform

import (
	"errors"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const internetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

const (
	internetOptionRefresh         = 37
	internetOptionSettingsChanged = 39
)

var procInternetSetOptionW = windows.NewLazySystemDLL("wininet.dll").NewProc("InternetSetOptionW")

// NewSystemProxy returns the per-user Windows proxy manager. The backup
// file is stored at path.
func NewSystemProxy(path string) SystemProxy {
	return newProxyManager(path, registryProxyBackend{})
}

type registryProxyBackend struct{}

func (registryProxyBackend) Read() (ProxySettings, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.QUERY_VALUE)
	if err != nil {
		return ProxySettings{}, err
	}
	defer key.Close()

	settings := ProxySettings{}
	settings.ProxyEnable = readDWORD(key, "ProxyEnable")
	settings.ProxyServer = readString(key, "ProxyServer")
	settings.ProxyOverride = readString(key, "ProxyOverride")
	settings.AutoConfigURL = readString(key, "AutoConfigURL")
	settings.AutoDetect = readDWORD(key, "AutoDetect")
	return settings, nil
}

func (registryProxyBackend) Write(settings ProxySettings) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	if err := key.SetDWordValue("ProxyEnable", settings.ProxyEnable); err != nil {
		return err
	}
	if err := setStringOrDelete(key, "ProxyServer", settings.ProxyServer); err != nil {
		return err
	}
	if err := setStringOrDelete(key, "ProxyOverride", settings.ProxyOverride); err != nil {
		return err
	}
	if err := setStringOrDelete(key, "AutoConfigURL", settings.AutoConfigURL); err != nil {
		return err
	}
	return key.SetDWordValue("AutoDetect", settings.AutoDetect)
}

func (registryProxyBackend) Notify() error {
	// Tell WinINET that the settings changed. Without this, already running
	// applications may keep using the old proxy until they restart.
	r1, _, err := procInternetSetOptionW.Call(0, internetOptionSettingsChanged, 0, 0)
	if r1 == 0 {
		return err
	}
	r1, _, err = procInternetSetOptionW.Call(0, internetOptionRefresh, 0, 0)
	if r1 == 0 {
		return err
	}
	return nil
}

func readDWORD(key registry.Key, name string) uint32 {
	value, _, err := key.GetIntegerValue(name)
	if err != nil {
		return 0
	}
	return uint32(value)
}

func readString(key registry.Key, name string) string {
	value, _, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	return value
}

func setStringOrDelete(key registry.Key, name, value string) error {
	if value == "" {
		err := key.DeleteValue(name)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	return key.SetStringValue(name, value)
}
