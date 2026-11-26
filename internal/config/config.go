// Package config defines qbox's on-disk configuration and its store.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Version is the current configuration schema version.
const Version = 2

// MaxRoutingProfiles bounds the number of routing profiles.
const MaxRoutingProfiles = 32

// maxRoutingProfileName bounds a profile name shown in the UI.
const maxRoutingProfileName = 64

// maxConfigSize bounds the config file read from disk.
const maxConfigSize = 32 << 20

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
	// RoutingProfiles stores every routing profile. Routing above is the
	// active profile and stays authoritative for the engine and old builds.
	RoutingProfiles        []RoutingProfile `json:"routing_profiles,omitempty"`
	ActiveRoutingProfileID string           `json:"active_routing_profile,omitempty"`
	TUN                    TUN              `json:"tun"`
	DNS                    DNS              `json:"dns"`
	LogLevel               string           `json:"log_level,omitempty"`
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

// RoutingProfile is a named set of routing rules.
type RoutingProfile struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Routing Routing `json:"routing"`
}

// TUN holds the inbound settings.
type TUN struct {
	InterfaceName string `json:"interface_name"`
	Stack         string `json:"stack"`
	MTU           int    `json:"mtu,omitempty"`
	StrictRoute   bool   `json:"strict_route,omitempty"`
	// DisableIPv6 keeps the IPv6 address off the TUN interface. Some
	// Windows installations with IPv6 disabled cannot assign one.
	DisableIPv6 bool `json:"disable_ipv6,omitempty"`
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
	routing := Routing{
		Mode:      ModeRules,
		BypassLAN: true,
		BypassRU:  true,
		BlockAds:  true,
	}
	return Config{
		Version:      Version,
		SelectedNode: "auto",
		Routing:      routing,
		RoutingProfiles: []RoutingProfile{{
			ID:      "profile-1",
			Name:    "Основной",
			Routing: routing.Clone(),
		}},
		ActiveRoutingProfileID: "profile-1",
		TUN: TUN{
			InterfaceName: "qbox",
			Stack:         StackMixed,
			MTU:           9000,
			StrictRoute:   true,
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
	if c.RoutingProfiles != nil {
		out.RoutingProfiles = make([]RoutingProfile, len(c.RoutingProfiles))
		for i, profile := range c.RoutingProfiles {
			out.RoutingProfiles[i] = profile.Clone()
		}
	}
	if c.Subscriptions != nil {
		out.Subscriptions = make([]Subscription, len(c.Subscriptions))
		for i, sub := range c.Subscriptions {
			out.Subscriptions[i] = sub.Clone()
		}
	}
	return out
}

// Clone returns a deep copy of a routing profile.
func (p RoutingProfile) Clone() RoutingProfile {
	out := p
	out.Routing = p.Routing.Clone()
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

// Equal reports whether two routing configurations have the same effect.
func (r Routing) Equal(other Routing) bool {
	return r.Mode == other.Mode &&
		r.BypassLAN == other.BypassLAN &&
		r.BypassRU == other.BypassRU &&
		r.BlockAds == other.BlockAds &&
		slices.Equal(r.DirectDomains, other.DirectDomains) &&
		slices.Equal(r.DirectCIDRs, other.DirectCIDRs) &&
		slices.Equal(r.ProxyDomains, other.ProxyDomains) &&
		slices.Equal(r.ProxyCIDRs, other.ProxyCIDRs)
}

func (r Routing) configured() bool {
	return r.Mode != "" || r.BypassLAN || r.BypassRU || r.BlockAds ||
		len(r.DirectDomains) > 0 || len(r.DirectCIDRs) > 0 ||
		len(r.ProxyDomains) > 0 || len(r.ProxyCIDRs) > 0
}

// Normalize fills zero values with defaults and repairs invalid choices.
func (c *Config) Normalize() {
	if c.Version < Version {
		c.Version = Version
	}
	def := Defaults()

	flatRoutingSet := c.Routing.configured()
	normalizeRouting(&c.Routing, def.Routing)
	c.normalizeRoutingProfiles(def.Routing, flatRoutingSet)

	c.TUN.InterfaceName = strings.TrimSpace(c.TUN.InterfaceName)
	if !validInterfaceName(c.TUN.InterfaceName) {
		c.TUN.InterfaceName = def.TUN.InterfaceName
	}
	switch c.TUN.Stack {
	case StackSystem, StackMixed, StackGVisor:
	default:
		c.TUN.Stack = def.TUN.Stack
	}
	minMTU := 576
	if !c.TUN.DisableIPv6 {
		minMTU = 1280
	}
	if c.TUN.MTU < minMTU || c.TUN.MTU > 65535 {
		c.TUN.MTU = def.TUN.MTU
	}
	if c.DNS.Remote == "" {
		c.DNS.Remote = def.DNS.Remote
	}
	if c.LogLevel == "warning" {
		c.LogLevel = "warn"
	}
	if c.LogLevel == "" {
		c.LogLevel = def.LogLevel
	}
	switch c.LogLevel {
	case "trace", "debug", "info", "warn", "error":
	default:
		c.LogLevel = def.LogLevel
	}
	if c.SelectedNode == "" {
		c.SelectedNode = "auto"
	}
	usedIDs := make(map[string]struct{}, len(c.Subscriptions))
	for i := range c.Subscriptions {
		id := c.Subscriptions[i].ID
		if id == "" {
			id = fmt.Sprintf("sub-%d", i+1)
		}
		base := id
		for n := 2; ; n++ {
			if _, exists := usedIDs[id]; !exists {
				break
			}
			id = fmt.Sprintf("%s-%d", base, n)
		}
		usedIDs[id] = struct{}{}
		c.Subscriptions[i].ID = id
	}
	if c.ActiveSubscription != "" && c.Subscription(c.ActiveSubscription) == nil {
		c.ActiveSubscription = ""
	}
	if c.ActiveSubscription == "" || len(c.ActiveNodes()) == 0 {
		c.ActiveSubscription = ""
		for i := range c.Subscriptions {
			if len(c.Subscriptions[i].Nodes) > 0 {
				c.ActiveSubscription = c.Subscriptions[i].ID
				break
			}
		}
	}
	if c.ActiveSubscription == "" && len(c.Subscriptions) > 0 {
		c.ActiveSubscription = c.Subscriptions[0].ID
	}
}

func normalizeRouting(r *Routing, def Routing) {
	switch r.Mode {
	case ModeGlobal, ModeRules, ModeDirect:
	default:
		r.Mode = def.Mode
	}
}

func (c *Config) normalizeRoutingProfiles(def Routing, flatRoutingSet bool) {
	if len(c.RoutingProfiles) == 0 {
		c.RoutingProfiles = []RoutingProfile{{
			ID:      "profile-1",
			Name:    "Основной",
			Routing: c.Routing.Clone(),
		}}
		c.ActiveRoutingProfileID = c.RoutingProfiles[0].ID
		return
	}

	usedIDs := make(map[string]struct{}, len(c.RoutingProfiles))
	usedNames := make(map[string]struct{}, len(c.RoutingProfiles))
	activeID := strings.TrimSpace(c.ActiveRoutingProfileID)
	c.ActiveRoutingProfileID = activeID
	activeAssigned := false
	for i := range c.RoutingProfiles {
		profile := &c.RoutingProfiles[i]
		oldID := strings.TrimSpace(profile.ID)
		if oldID == "" {
			oldID = fmt.Sprintf("profile-%d", i+1)
		}
		id := oldID
		for n := 2; ; n++ {
			if _, exists := usedIDs[id]; !exists {
				break
			}
			id = fmt.Sprintf("%s-%d", oldID, n)
		}
		usedIDs[id] = struct{}{}
		profile.ID = id
		if !activeAssigned && activeID != "" && activeID == oldID {
			c.ActiveRoutingProfileID = id
			activeAssigned = true
		}

		profile.Name = sanitizeRoutingProfileName(profile.Name)
		if profile.Name == "" {
			profile.Name = fmt.Sprintf("Профиль %d", i+1)
		}
		profile.Name = uniqueRoutingProfileName(profile.Name, usedNames)
		normalizeRouting(&profile.Routing, def)
	}

	active := c.RoutingProfile(c.ActiveRoutingProfileID)
	if active == nil {
		active = &c.RoutingProfiles[0]
		c.ActiveRoutingProfileID = active.ID
	}
	if flatRoutingSet {
		// The flat routing block is what the UI edits and what older builds
		// read, so it wins when both copies are present.
		active.Routing = c.Routing.Clone()
	} else {
		c.Routing = active.Routing.Clone()
	}
}

func sanitizeRoutingProfileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n', '\t':
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		if r == '\u200b' || r == '\u200c' || r == '\u200d' || r == '\u200e' || r == '\u200f' ||
			(r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069') || r == '\ufeff' {
			return -1
		}
		return r
	}, name)
	return strings.TrimSpace(truncateRunes(name, maxRoutingProfileName))
}

func uniqueRoutingProfileName(base string, used map[string]struct{}) string {
	name := base
	for n := 2; ; n++ {
		if _, exists := used[name]; !exists {
			break
		}
		suffix := fmt.Sprintf(" (%d)", n)
		name = truncateRunes(base, maxRoutingProfileName-len([]rune(suffix))) + suffix
	}
	used[name] = struct{}{}
	return name
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

// RoutingProfile returns the profile with the given ID, if any.
func (c *Config) RoutingProfile(id string) *RoutingProfile {
	for i := range c.RoutingProfiles {
		if c.RoutingProfiles[i].ID == id {
			return &c.RoutingProfiles[i]
		}
	}
	return nil
}

// ActiveRoutingProfile returns the profile currently applied.
func (c *Config) ActiveRoutingProfile() *RoutingProfile {
	return c.RoutingProfile(c.ActiveRoutingProfileID)
}

// SyncActiveRoutingProfile copies the flat routing settings into the active
// profile, so an in-memory profile never lags behind the UI edits.
func (c *Config) SyncActiveRoutingProfile() {
	if profile := c.ActiveRoutingProfile(); profile != nil {
		profile.Routing = c.Routing.Clone()
	}
}

// SelectRoutingProfile saves the active routing settings into the current
// profile and switches to another one.
func (c *Config) SelectRoutingProfile(id string) bool {
	target := c.RoutingProfile(id)
	if target == nil {
		return false
	}
	if current := c.ActiveRoutingProfile(); current != nil {
		current.Routing = c.Routing.Clone()
	}
	c.ActiveRoutingProfileID = target.ID
	c.Routing = target.Routing.Clone()
	return true
}

// AddRoutingProfile creates a profile copying the current routing settings.
func (c *Config) AddRoutingProfile(name string) (string, error) {
	if len(c.RoutingProfiles) >= MaxRoutingProfiles {
		return "", fmt.Errorf("не больше %d профилей маршрутизации", MaxRoutingProfiles)
	}
	name = sanitizeRoutingProfileName(name)
	if name == "" {
		name = "Новый профиль"
	}
	usedNames := make(map[string]struct{}, len(c.RoutingProfiles))
	for i := range c.RoutingProfiles {
		usedNames[c.RoutingProfiles[i].Name] = struct{}{}
	}
	profile := RoutingProfile{
		ID:      c.nextRoutingProfileID(),
		Name:    uniqueRoutingProfileName(name, usedNames),
		Routing: c.Routing.Clone(),
	}
	c.RoutingProfiles = append(c.RoutingProfiles, profile)
	return profile.ID, nil
}

// RenameRoutingProfile changes a profile name and returns the final unique
// name. The boolean is false for an unknown profile or an empty name.
func (c *Config) RenameRoutingProfile(id, name string) (string, bool) {
	profile := c.RoutingProfile(id)
	if profile == nil {
		return "", false
	}
	name = sanitizeRoutingProfileName(name)
	if name == "" {
		return "", false
	}
	usedNames := make(map[string]struct{}, len(c.RoutingProfiles))
	for i := range c.RoutingProfiles {
		if c.RoutingProfiles[i].ID != id {
			usedNames[c.RoutingProfiles[i].Name] = struct{}{}
		}
	}
	profile.Name = uniqueRoutingProfileName(name, usedNames)
	return profile.Name, true
}

// DeleteRoutingProfile removes a profile. The last profile cannot be
// deleted. Deleting the active profile selects its neighbour.
func (c *Config) DeleteRoutingProfile(id string) bool {
	if len(c.RoutingProfiles) <= 1 {
		return false
	}
	index := -1
	for i := range c.RoutingProfiles {
		if c.RoutingProfiles[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}
	wasActive := c.ActiveRoutingProfileID == id
	c.RoutingProfiles = append(c.RoutingProfiles[:index], c.RoutingProfiles[index+1:]...)
	if !wasActive {
		return true
	}
	next := index
	if next >= len(c.RoutingProfiles) {
		next = 0
	}
	c.ActiveRoutingProfileID = c.RoutingProfiles[next].ID
	c.Routing = c.RoutingProfiles[next].Routing.Clone()
	return true
}

func (c *Config) nextRoutingProfileID() string {
	for n := 1; ; n++ {
		id := fmt.Sprintf("profile-%d", n)
		if c.RoutingProfile(id) == nil {
			return id
		}
	}
}

func validInterfaceName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	if strings.ContainsAny(name, `\/:*?"<>|`) {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
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
	sub := c.Active()
	if sub == nil {
		return nil
	}
	for i := range sub.Nodes {
		if sub.Nodes[i].Tag == tag {
			return &sub.Nodes[i]
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

	data, err := readLimited(s.path, maxConfigSize)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Defaults()
			cfg.Normalize()
			return cfg, nil
		}
		if backup, backupErr := readConfigFile(s.path + ".bak"); backupErr == nil {
			return backup, fmt.Errorf("config.json недоступен (%v); загружена резервная копия", err)
		}
		fallback := Defaults()
		fallback.Normalize()
		return fallback, fmt.Errorf("config.json недоступен: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		if backup, backupErr := readConfigFile(s.path + ".bak"); backupErr == nil {
			return backup, fmt.Errorf("config.json повреждён (%v); загружена резервная копия", err)
		}
		fallback := Defaults()
		fallback.Normalize()
		return fallback, fmt.Errorf("config.json повреждён: %w", err)
	}
	cfg.Normalize()
	return cfg, nil
}

func readConfigFile(path string) (Config, error) {
	data, err := readLimited(path, maxConfigSize)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	cfg.Normalize()
	return cfg, nil
}

func readLimited(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("файл %s больше %d МБ", path, limit>>20)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("файл %s вырос больше %d МБ", path, limit>>20)
	}
	return data, nil
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

	// Only preserve a valid, writable-sized primary as the backup; a corrupt
	// or oversized file must not replace a good .bak.
	if old, err := readLimited(s.path, maxConfigSize); err == nil && len(old) > 0 && json.Valid(old) {
		_ = writeAtomic(s.path+".bak", old, 0o600)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if int64(len(data)) > maxConfigSize {
		return fmt.Errorf("конфигурация больше %d МБ", maxConfigSize>>20)
	}
	return writeAtomic(s.path, data, 0o600)
}

func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(perm); err != nil {
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
	return os.Rename(tmpName, path)
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
