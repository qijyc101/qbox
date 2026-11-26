// Package config defines qbox's on-disk configuration and its store.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Version is the current configuration schema version.
const Version = 1

// Routing modes.
const (
	ModeGlobal = "global"
	ModeRules  = "rules"
	ModeDirect = "direct"
)

// TUN stacks supported by sing-box.
const (
	StackSystem = "system"
	StackMixed  = "mixed"
	StackGVisor = "gvisor"
)

// Config is the whole persisted state of qbox.
type Config struct {
	Version       int            `json:"version"`
	Subscriptions []Subscription `json:"subscriptions,omitempty"`
	// ActiveSubscription is the ID of the subscription whose nodes are used.
	ActiveSubscription string  `json:"active_subscription,omitempty"`
	SelectedNode       string  `json:"selected_node,omitempty"`
	Routing            Routing `json:"routing"`
	TUN                TUN     `json:"tun"`
	DNS                DNS     `json:"dns"`
	LogLevel           string  `json:"log_level,omitempty"`
}

// Subscription is a fetched proxy list.
type Subscription struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	Nodes     []Node    `json:"nodes,omitempty"`
}

// Node is one proxy outbound stored as sing-box JSON.
type Node struct {
	Tag      string          `json:"tag"`
	Type     string          `json:"type"`
	Server   string          `json:"server,omitempty"`
	Port     int             `json:"port,omitempty"`
	Outbound json.RawMessage `json:"outbound"`
}

// Routing is the rule configuration.
type Routing struct {
	Mode          string   `json:"mode"`
	BypassLAN     bool     `json:"bypass_lan"`
	BypassRU      bool     `json:"bypass_ru"`
	BlockAds      bool     `json:"block_ads"`
	DirectDomains []string `json:"direct_domains,omitempty"`
	DirectCIDRs   []string `json:"direct_cidrs,omitempty"`
	ProxyDomains  []string `json:"proxy_domains,omitempty"`
	ProxyCIDRs    []string `json:"proxy_cidrs,omitempty"`
}

// TUN holds the inbound settings.
type TUN struct {
	InterfaceName string `json:"interface_name"`
	Stack         string `json:"stack"`
	MTU           int    `json:"mtu,omitempty"`
	StrictRoute   bool   `json:"strict_route,omitempty"`
}

// DNS holds the resolver settings.
type DNS struct {
	// Remote is a DoH/DoT URL used while the tunnel is up, for example
	// https://1.1.1.1/dns-query.
	Remote   string `json:"remote,omitempty"`
	UseProxy bool   `json:"use_proxy"`
}

// Defaults returns a usable configuration for a fresh installation.
func Defaults() Config {
	return Config{
		Version:      Version,
		SelectedNode: "auto",
		Routing: Routing{
			Mode:      ModeRules,
			BypassLAN: true,
			BypassRU:  true,
			BlockAds:  true,
		},
		TUN: TUN{
			InterfaceName: "qbox",
			Stack:         StackMixed,
			MTU:           9000,
		},
		DNS: DNS{
			Remote:   "https://1.1.1.1/dns-query",
			UseProxy: true,
		},
		LogLevel: "info",
	}
}

// Clone returns a deep copy safe to hand to another goroutine.
func (c Config) Clone() Config {
	out := c
	out.Routing = c.Routing.Clone()
	if c.Subscriptions != nil {
		out.Subscriptions = make([]Subscription, len(c.Subscriptions))
		for i, sub := range c.Subscriptions {
			out.Subscriptions[i] = sub.Clone()
		}
	}
	return out
}

// Clone returns a deep copy of a subscription.
func (s Subscription) Clone() Subscription {
	out := s
	if s.Nodes != nil {
		out.Nodes = make([]Node, len(s.Nodes))
		for i, n := range s.Nodes {
			out.Nodes[i] = n.Clone()
		}
	}
	return out
}

// Clone returns a deep copy of a node.
func (n Node) Clone() Node {
	out := n
	if n.Outbound != nil {
		out.Outbound = append(json.RawMessage(nil), n.Outbound...)
	}
	return out
}

// Clone returns a deep copy of the routing settings.
func (r Routing) Clone() Routing {
	out := r
	out.DirectDomains = slices.Clone(r.DirectDomains)
	out.DirectCIDRs = slices.Clone(r.DirectCIDRs)
	out.ProxyDomains = slices.Clone(r.ProxyDomains)
	out.ProxyCIDRs = slices.Clone(r.ProxyCIDRs)
	return out
}

// Normalize fills zero values with defaults and repairs invalid choices.
func (c *Config) Normalize() {
	if c.Version == 0 {
		c.Version = Version
	}
	def := Defaults()

	switch c.Routing.Mode {
	case ModeGlobal, ModeRules, ModeDirect:
	default:
		c.Routing.Mode = def.Routing.Mode
	}

	if c.TUN.InterfaceName == "" {
		c.TUN.InterfaceName = def.TUN.InterfaceName
	}
	switch c.TUN.Stack {
	case StackSystem, StackMixed, StackGVisor:
	default:
		c.TUN.Stack = def.TUN.Stack
	}
	if c.TUN.MTU <= 0 {
		c.TUN.MTU = def.TUN.MTU
	}
	if c.DNS.Remote == "" {
		c.DNS.Remote = def.DNS.Remote
	}
	if c.LogLevel == "" {
		c.LogLevel = def.LogLevel
	}
	switch c.LogLevel {
	case "trace", "debug", "info", "warn", "warning", "error":
	default:
		c.LogLevel = def.LogLevel
	}
	if c.SelectedNode == "" {
		c.SelectedNode = "auto"
	}
	if c.ActiveSubscription != "" && c.Subscription(c.ActiveSubscription) == nil {
		c.ActiveSubscription = ""
	}
	if c.ActiveSubscription == "" && len(c.Subscriptions) > 0 {
		c.ActiveSubscription = c.Subscriptions[0].ID
	}
	for i := range c.Subscriptions {
		if c.Subscriptions[i].ID == "" {
			c.Subscriptions[i].ID = fmt.Sprintf("sub-%d", i+1)
		}
	}
}

// Subscription returns the subscription with the given ID, if any.
func (c *Config) Subscription(id string) *Subscription {
	for i := range c.Subscriptions {
		if c.Subscriptions[i].ID == id {
			return &c.Subscriptions[i]
		}
	}
	return nil
}

// Active returns the subscription whose nodes are in use.
func (c *Config) Active() *Subscription {
	if sub := c.Subscription(c.ActiveSubscription); sub != nil {
		return sub
	}
	if len(c.Subscriptions) > 0 {
		return &c.Subscriptions[0]
	}
	return nil
}

// ActiveNodes returns the nodes of the active subscription.
func (c *Config) ActiveNodes() []Node {
	sub := c.Active()
	if sub == nil {
		return nil
	}
	return sub.Nodes
}

// NodeByTag finds a node of the active subscription.
func (c *Config) NodeByTag(tag string) *Node {
	for _, n := range c.ActiveNodes() {
		if n.Tag == tag {
			node := n
			return &node
		}
	}
	return nil
}

// Store reads and writes the configuration file.
type Store struct {
	dir  string
	path string
	mu   sync.Mutex
}

// NewStore creates a store rooted at dir. The zero dir means the default
// per-user configuration directory.
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		var err error
		dir, err = DefaultDir()
		if err != nil {
			return nil, err
		}
	}
	return &Store{dir: dir, path: filepath.Join(dir, "config.json")}, nil
}

// Path returns the configuration file path.
func (s *Store) Path() string { return s.path }

// Dir returns the configuration directory.
func (s *Store) Dir() string { return s.dir }

// Load reads the configuration. A missing file is not an error: it yields
// defaults. A damaged file falls back to the .bak copy and reports the
// problem so the UI can show it.
func (s *Store) Load() (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Defaults()
			cfg.Normalize()
			return cfg, nil
		}
		return Defaults(), err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		backup, backupErr := os.ReadFile(s.path + ".bak")
		if backupErr == nil {
			var restored Config
			if json.Unmarshal(backup, &restored) == nil {
				restored.Normalize()
				return restored, fmt.Errorf("config.json повреждён (%v); загружена резервная копия", err)
			}
		}
		fallback := Defaults()
		fallback.Normalize()
		return fallback, fmt.Errorf("config.json повреждён: %w", err)
	}
	cfg.Normalize()
	return cfg, nil
}

// Save writes the configuration atomically, keeping one backup copy.
func (s *Store) Save(cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg = cfg.Clone()
	cfg.Normalize()

	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}

	if old, err := os.ReadFile(s.path); err == nil && len(old) > 0 {
		_ = os.WriteFile(s.path+".bak", old, 0o600)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(s.dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
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
	return os.Rename(tmpName, s.path)
}

// CleanList trims lines, drops empty ones and comments, and removes
// duplicates while preserving order.
func CleanList(lines []string) []string {
	out := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		out = append(out, line)
	}
	return out
}

// SplitList turns a text area into a clean list.
func SplitList(text string) []string {
	return CleanList(strings.Split(text, "\n"))
}

// JoinList formats a list for a text area.
func JoinList(lines []string) string {
	return strings.Join(lines, "\n")
}
