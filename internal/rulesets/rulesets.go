// Package rulesets downloads and caches the binary rule-sets used for
// ad blocking and country bypass.
package rulesets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MaxSize bounds one rule-set file.
const MaxSize = 64 << 20

// Item describes a rule-set qbox knows about.
type Item struct {
	Tag   string
	Title string
	URL   string
	File  string
}

// Items are the rule-sets used by the routing settings.
var Items = []Item{
	{
		Tag:   "geosite-ru",
		Title: "Домены РФ",
		URL:   "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ru.srs",
		File:  "geosite-category-ru.srs",
	},
	{
		Tag:   "geoip-ru",
		Title: "IP РФ",
		URL:   "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-ru.srs",
		File:  "geoip-ru.srs",
	},
	{
		Tag:   "geosite-ads",
		Title: "Реклама и трекеры",
		URL:   "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ads-all.srs",
		File:  "geosite-category-ads-all.srs",
	},
}

// Status reports whether a rule-set is installed.
type Status struct {
	Tag       string
	Title     string
	Installed bool
	Size      int64
	Updated   time.Time
}

// Manager owns the rule-set cache.
type Manager struct {
	dir   string
	HTTP  *http.Client
	Items []Item
	mu    sync.Mutex
}

// New creates a manager storing files under dir.
func New(dir string) *Manager {
	return &Manager{
		dir:   dir,
		HTTP:  &http.Client{Timeout: 90 * time.Second},
		Items: Items,
	}
}

// Dir returns the cache directory.
func (m *Manager) Dir() string { return m.dir }

func (m *Manager) items() []Item {
	if len(m.Items) == 0 {
		return Items
	}
	return m.Items
}

// Path returns the path of an installed rule-set.
func (m *Manager) Path(tag string) (string, bool) {
	item, ok := m.itemByTag(tag)
	if !ok {
		return "", false
	}
	path := filepath.Join(m.dir, item.File)
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return "", false
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	var magic [3]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil || string(magic[:]) != "SRS" {
		return "", false
	}
	return path, true
}

// Paths returns all installed rule-sets by tag.
func (m *Manager) Paths() map[string]string {
	out := make(map[string]string, len(m.items()))
	for _, item := range m.items() {
		if path, ok := m.Path(item.Tag); ok {
			out[item.Tag] = path
		}
	}
	return out
}

// Status returns the state of every known rule-set.
func (m *Manager) Status() []Status {
	out := make([]Status, 0, len(m.items()))
	for _, item := range m.items() {
		st := Status{Tag: item.Tag, Title: item.Title}
		path := filepath.Join(m.dir, item.File)
		if info, err := os.Stat(path); err == nil {
			st.Installed = info.Size() > 0
			st.Size = info.Size()
			st.Updated = info.ModTime()
		}
		if _, ok := m.Path(item.Tag); !ok {
			st.Installed = false
		}
		out = append(out, st)
	}
	return out
}

// Update downloads the given tags, or every rule-set when tags is empty.
// A 304 response keeps the existing file.
func (m *Manager) Update(ctx context.Context, tags ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.dir, 0o700); err != nil {
		return err
	}

	items := m.items()
	if len(tags) > 0 {
		items = make([]Item, 0, len(tags))
		for _, tag := range tags {
			item, ok := m.itemByTag(tag)
			if !ok {
				return fmt.Errorf("неизвестный rule-set %q", tag)
			}
			items = append(items, item)
		}
	}

	var errs []error
	for _, item := range items {
		if err := m.download(ctx, item); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", item.Tag, err))
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) download(ctx context.Context, item Item) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "qbox/0.1 (sing-box 1.14.4)")
	req.Header.Set("Accept", "application/octet-stream")

	target := filepath.Join(m.dir, item.File)
	etagPath := target + ".etag"
	if etag, err := os.ReadFile(etagPath); err == nil && len(bytes.TrimSpace(etag)) > 0 {
		req.Header.Set("If-None-Match", strings.TrimSpace(string(etag)))
	}

	client := m.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("скачивание: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("сервер вернул %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxSize+1))
	if err != nil {
		return fmt.Errorf("чтение: %w", err)
	}
	if len(data) > MaxSize {
		return fmt.Errorf("файл больше %d МБ", MaxSize>>20)
	}
	if len(data) < 4 || !bytes.HasPrefix(data, []byte("SRS")) {
		return errors.New("файл не похож на бинарный rule-set sing-box")
	}

	tmp, err := os.CreateTemp(m.dir, ".ruleset-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		return err
	}
	if etag := resp.Header.Get("ETag"); etag != "" {
		_ = os.WriteFile(etagPath, []byte(etag), 0o600)
	} else {
		_ = os.Remove(etagPath)
	}
	return nil
}

func (m *Manager) itemByTag(tag string) (Item, bool) {
	for _, item := range m.items() {
		if item.Tag == tag {
			return item, true
		}
	}
	return Item{}, false
}
