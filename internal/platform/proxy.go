package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ProxySettings is the subset of the Windows internet settings that qbox
// changes while the system proxy mode is on.
type ProxySettings struct {
	ProxyEnable   uint32 `json:"proxy_enable"`
	ProxyServer   string `json:"proxy_server,omitempty"`
	ProxyOverride string `json:"proxy_override,omitempty"`
	AutoConfigURL string `json:"auto_config_url,omitempty"`
	AutoDetect    uint32 `json:"auto_detect,omitempty"`
}

type proxyBackup struct {
	Saved   ProxySettings `json:"saved"`
	Address string        `json:"address"`
	SavedAt time.Time     `json:"saved_at"`
}

type proxyBackend interface {
	Read() (ProxySettings, error)
	Write(ProxySettings) error
	Notify() error
}

// SystemProxy switches the per-user Windows proxy on and off, keeping the
// previous settings so they can be restored.
type SystemProxy interface {
	Enable(address string) error
	Restore() error
	Pending() bool
}

// NoopSystemProxy is the inactive proxy manager used by tests and before
// the app is wired to the platform.
type NoopSystemProxy struct{}

func (NoopSystemProxy) Enable(string) error { return nil }
func (NoopSystemProxy) Restore() error      { return nil }
func (NoopSystemProxy) Pending() bool       { return false }

// ProxyManager implements SystemProxy over a backend and a backup file.
type ProxyManager struct {
	path    string
	backend proxyBackend

	mu      sync.Mutex
	enabled bool
	address string
}

const maxProxyBackupSize = 64 << 10

func newProxyManager(path string, backend proxyBackend) *ProxyManager {
	return &ProxyManager{path: path, backend: backend}
}

// Enable points the system proxy at address. The previous settings are
// written to the backup file first, so a crash can be recovered.
func (m *ProxyManager) Enable(address string) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return errors.New("пустой адрес прокси")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.enabled && m.address == address {
		return nil
	}
	if m.enabled {
		if err := m.restoreLocked(); err != nil {
			return err
		}
	}
	if _, err := os.Stat(m.path); err == nil {
		// A previous run left a backup. Put the old settings back before
		// saving a new snapshot.
		if err := m.restoreLocked(); err != nil {
			return fmt.Errorf("не удалось восстановить прошлые настройки прокси: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	current, err := m.backend.Read()
	if err != nil {
		return fmt.Errorf("чтение настроек прокси: %w", err)
	}
	backup := proxyBackup{Saved: current, Address: address, SavedAt: time.Now().UTC()}
	if err := m.writeBackup(backup); err != nil {
		return fmt.Errorf("сохранение настроек прокси: %w", err)
	}
	next := ProxySettings{
		ProxyEnable:   1,
		ProxyServer:   address,
		ProxyOverride: "localhost;127.0.0.1;::1",
		AutoConfigURL: "",
		AutoDetect:    0,
	}
	if err := m.backend.Write(next); err != nil {
		_ = m.backend.Write(current)
		_ = m.backend.Notify()
		_ = os.Remove(m.path)
		return fmt.Errorf("включение системного прокси: %w", err)
	}
	_ = m.backend.Notify()
	m.enabled = true
	m.address = address
	return nil
}

// Restore returns the settings saved by Enable. If another program changed
// the proxy meanwhile, the backup is dropped instead of overwriting it.
func (m *ProxyManager) Restore() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.restoreLocked()
}

func (m *ProxyManager) restoreLocked() error {
	backup, err := m.readBackup()
	if err != nil {
		if os.IsNotExist(err) {
			m.enabled = false
			m.address = ""
			return nil
		}
		return err
	}
	current, err := m.backend.Read()
	if err != nil {
		return fmt.Errorf("чтение настроек прокси: %w", err)
	}
	ours := current.ProxyEnable == 1 && current.ProxyServer == backup.Address
	if ours {
		if err := m.backend.Write(backup.Saved); err != nil {
			return fmt.Errorf("восстановление системного прокси: %w", err)
		}
		_ = m.backend.Notify()
	}
	if err := os.Remove(m.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	m.enabled = false
	m.address = ""
	return nil
}

// Pending reports whether a proxy backup from an earlier run exists.
func (m *ProxyManager) Pending() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := os.Stat(m.path)
	return err == nil
}

func (m *ProxyManager) writeBackup(backup proxyBackup) error {
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Remove(m.path); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, m.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (m *ProxyManager) readBackup() (proxyBackup, error) {
	file, err := os.Open(m.path)
	if err != nil {
		return proxyBackup{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return proxyBackup{}, err
	}
	if info.Size() > maxProxyBackupSize {
		return proxyBackup{}, fmt.Errorf("файл настроек прокси больше %d КБ", maxProxyBackupSize>>10)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxProxyBackupSize+1))
	if err != nil {
		return proxyBackup{}, err
	}
	if len(data) > maxProxyBackupSize {
		return proxyBackup{}, fmt.Errorf("файл настроек прокси больше %d КБ", maxProxyBackupSize>>10)
	}
	var backup proxyBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		return proxyBackup{}, err
	}
	return backup, nil
}
