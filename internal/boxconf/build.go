// Package boxconf turns qbox settings into a sing-box configuration.
//
// It builds JSON rather than sing-box option structs so that the result can
// be inspected, golden-tested, and validated by the pinned sing-box version
// itself before the engine starts.
package boxconf

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"qbox/internal/config"
	"qbox/internal/jsonutil"
)

// Input is everything the builder needs.
type Input struct {
	Config       config.Config
	RuleSetPaths map[string]string // tag -> absolute .srs path
	ExeName      string
	CacheFile    string
	// DisableIPv6 forces an IPv4-only TUN interface. Used for the
	// automatic retry when Windows refuses the IPv6 address.
	DisableIPv6 bool
}

type document struct {
	Log          logOptions          `json:"log"`
	DNS          dnsOptions          `json:"dns"`
	Inbounds     []inbound           `json:"inbounds"`
	Outbounds    []json.RawMessage   `json:"outbounds"`
	Route        routeOptions        `json:"route"`
	Experimental experimentalOptions `json:"experimental"`
}

type logOptions struct {
	Level     string `json:"level,omitempty"`
	Timestamp bool   `json:"timestamp,omitempty"`
}

type dnsOptions struct {
	Servers  []dnsServer `json:"servers"`
	Final    string      `json:"final,omitempty"`
	Strategy string      `json:"strategy,omitempty"`
}

type dnsServer struct {
	Type       string      `json:"type"`
	Tag        string      `json:"tag"`
	Server     string      `json:"server,omitempty"`
	ServerPort uint16      `json:"server_port,omitempty"`
	Path       string      `json:"path,omitempty"`
	Detour     string      `json:"detour,omitempty"`
	TLS        *tlsOptions `json:"tls,omitempty"`
}

type tlsOptions struct {
	Enabled    bool            `json:"enabled"`
	ServerName string          `json:"server_name,omitempty"`
	Insecure   bool            `json:"insecure,omitempty"`
	ALPN       []string        `json:"alpn,omitempty"`
	UTLS       *utlsOptions    `json:"utls,omitempty"`
	Reality    *realityOptions `json:"reality,omitempty"`
}

type utlsOptions struct {
	Enabled     bool   `json:"enabled"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type realityOptions struct {
	Enabled   bool   `json:"enabled"`
	PublicKey string `json:"public_key,omitempty"`
	ShortID   string `json:"short_id,omitempty"`
}

type inbound struct {
	Type          string   `json:"type"`
	Tag           string   `json:"tag"`
	InterfaceName string   `json:"interface_name,omitempty"`
	Address       []string `json:"address,omitempty"`
	MTU           int      `json:"mtu,omitempty"`
	DNSMode       string   `json:"dns_mode,omitempty"`
	AutoRoute     bool     `json:"auto_route,omitempty"`
	StrictRoute   bool     `json:"strict_route,omitempty"`
	Stack         string   `json:"stack,omitempty"`
}

type routeOptions struct {
	Rules                 []rule                `json:"rules,omitempty"`
	RuleSet               []ruleSet             `json:"rule_set,omitempty"`
	Final                 string                `json:"final,omitempty"`
	AutoDetectInterface   bool                  `json:"auto_detect_interface,omitempty"`
	DefaultDomainResolver *domainResolveOptions `json:"default_domain_resolver,omitempty"`
}

type domainResolveOptions struct {
	Server string `json:"server"`
}

type rule struct {
	Action       string   `json:"action,omitempty"`
	Outbound     string   `json:"outbound,omitempty"`
	Protocol     []string `json:"protocol,omitempty"`
	ProcessName  []string `json:"process_name,omitempty"`
	DomainSuffix []string `json:"domain_suffix,omitempty"`
	IPCIDR       []string `json:"ip_cidr,omitempty"`
	IPIsPrivate  bool     `json:"ip_is_private,omitempty"`
	RuleSet      []string `json:"rule_set,omitempty"`
}

type ruleSet struct {
	Type   string `json:"type"`
	Tag    string `json:"tag"`
	Format string `json:"format,omitempty"`
	Path   string `json:"path,omitempty"`
}

type experimentalOptions struct {
	CacheFile *cacheFile `json:"cache_file,omitempty"`
}

type cacheFile struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path,omitempty"`
}

// Build returns the sing-box configuration and non-fatal warnings.
func Build(in Input) ([]byte, []string, error) {
	cfg := in.Config.Clone()
	cfg.Normalize()

	var warnings []string
	nodes := cfg.ActiveNodes()
	tags := make(map[string]struct{}, len(nodes))
	nodeTags := make([]string, 0, len(nodes))
	outbounds := make([]json.RawMessage, 0, len(nodes)+2)

	if cfg.Routing.Mode != config.ModeDirect {
		for _, node := range nodes {
			if node.Server == "" {
				return nil, warnings, fmt.Errorf("нода %q: пустой адрес", node.Tag)
			}
			if _, exists := tags[node.Tag]; exists {
				return nil, warnings, fmt.Errorf("дублирующийся тег ноды %q", node.Tag)
			}
			tags[node.Tag] = struct{}{}
			nodeTags = append(nodeTags, node.Tag)

			raw, err := withTag(node.Outbound, node.Tag)
			if err != nil {
				return nil, warnings, fmt.Errorf("нода %q: %w", node.Tag, err)
			}
			outbounds = append(outbounds, raw)
		}
	}

	final := "direct"
	if cfg.Routing.Mode != config.ModeDirect {
		if len(nodes) == 0 {
			return nil, warnings, errors.New("нет нод: добавьте подписку или выберите режим Direct")
		}
		final = "auto"
		if cfg.SelectedNode != "" && cfg.SelectedNode != "auto" {
			if _, ok := tags[cfg.SelectedNode]; ok {
				final = cfg.SelectedNode
			} else {
				warnings = append(warnings, fmt.Sprintf("нода %q не найдена, используется auto", cfg.SelectedNode))
			}
		}
		if final == "auto" {
			auto, err := json.Marshal(map[string]any{
				"type":      "urltest",
				"tag":       "auto",
				"outbounds": nodeTags,
				"url":       "https://www.gstatic.com/generate_204",
				"interval":  "3m",
				"tolerance": 50,
			})
			if err != nil {
				return nil, warnings, err
			}
			outbounds = append(outbounds, auto)
		}
	}
	direct, _ := json.Marshal(map[string]any{"type": "direct", "tag": "direct"})
	outbounds = append(outbounds, direct)

	remoteDNS, remoteWarning := buildRemoteDNS(cfg.DNS, final)
	if remoteWarning != "" {
		warnings = append(warnings, remoteWarning)
	}

	rules, usedRuleSets, ruleWarnings := buildRules(cfg, final, in.ExeName, in.RuleSetPaths)
	warnings = append(warnings, ruleWarnings...)

	rulesets := make([]ruleSet, 0, len(usedRuleSets))
	usedTags := make([]string, 0, len(usedRuleSets))
	for tag := range usedRuleSets {
		usedTags = append(usedTags, tag)
	}
	sort.Strings(usedTags)
	for _, tag := range usedTags {
		rulesets = append(rulesets, ruleSet{
			Type:   "local",
			Tag:    tag,
			Format: "binary",
			Path:   usedRuleSets[tag],
		})
	}

	dnsServers := []dnsServer{{Type: "local", Tag: "local"}}
	dnsFinal := "local"
	if remoteDNS != nil {
		dnsServers = append(dnsServers, *remoteDNS)
		dnsFinal = "remote"
	}

	addresses := []string{"172.19.0.1/30"}
	disableIPv6 := cfg.TUN.DisableIPv6 || in.DisableIPv6
	if !disableIPv6 {
		addresses = append(addresses, "fdfe:dcba:9876::1/126")
	}
	strictRoute := cfg.TUN.StrictRoute
	dnsStrategy := "prefer_ipv4"
	if disableIPv6 {
		// Without an IPv6 address sing-tun installs no IPv6 route. On
		// Windows strict_route also blocks IPv6 leaks through the physical
		// interface, so force it for IPv4-only TUNs.
		if !strictRoute {
			warnings = append(warnings, "IPv6 отключён: strict_route включён для блокировки IPv6-утечек")
		}
		strictRoute = true
		dnsStrategy = "ipv4_only"
	}

	doc := document{
		Log: logOptions{
			Level:     cfg.LogLevel,
			Timestamp: true,
		},
		DNS: dnsOptions{
			Servers:  dnsServers,
			Final:    dnsFinal,
			Strategy: dnsStrategy,
		},
		Inbounds: []inbound{{
			Type:          "tun",
			Tag:           "tun-in",
			InterfaceName: cfg.TUN.InterfaceName,
			Address:       addresses,
			MTU:           cfg.TUN.MTU,
			DNSMode:       "hijack",
			AutoRoute:     true,
			StrictRoute:   strictRoute,
			Stack:         cfg.TUN.Stack,
		}},
		Outbounds: outbounds,
		Route: routeOptions{
			Rules:                 rules,
			RuleSet:               rulesets,
			Final:                 final,
			AutoDetectInterface:   true,
			DefaultDomainResolver: &domainResolveOptions{Server: "local"},
		},
		Experimental: experimentalOptions{
			CacheFile: &cacheFile{Enabled: true, Path: in.CacheFile},
		},
	}

	// guard against an accidental empty cache path: sing-box would write
	// cache.db into the working directory.
	if in.CacheFile == "" {
		doc.Experimental.CacheFile = nil
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, warnings, err
	}
	return data, warnings, nil
}

func buildRemoteDNS(dns config.DNS, final string) (*dnsServer, string) {
	if !dns.UseProxy || final == "direct" || strings.TrimSpace(dns.Remote) == "" {
		return nil, ""
	}
	raw := strings.TrimSpace(dns.Remote)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, "DNS-сервер не разобран, используется системный DNS"
	}
	server := &dnsServer{
		Tag:    "remote",
		Server: u.Hostname(),
		Detour: final,
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n <= 0 || n > 65535 {
			return nil, "DNS-сервер: некорректный порт, используется системный DNS"
		}
		server.ServerPort = uint16(n)
	}
	host := u.Hostname()
	ip := net.ParseIP(host) != nil
	switch strings.ToLower(u.Scheme) {
	case "https":
		server.Type = "https"
		server.Path = u.Path
		if server.Path == "" {
			server.Path = "/dns-query"
		}
		server.TLS = &tlsOptions{Enabled: true}
		if !ip {
			server.TLS.ServerName = host
		}
	case "tls":
		server.Type = "tls"
		server.TLS = &tlsOptions{Enabled: true}
		if !ip {
			server.TLS.ServerName = host
		}
	case "quic":
		server.Type = "quic"
		server.TLS = &tlsOptions{Enabled: true}
		if !ip {
			server.TLS.ServerName = host
		}
	case "udp":
		server.Type = "udp"
		if server.ServerPort == 0 {
			server.ServerPort = 53
		}
	default:
		return nil, fmt.Sprintf("DNS: схема %q не поддерживается, используется системный DNS", u.Scheme)
	}
	return server, ""
}

func buildRules(cfg config.Config, final, exeName string, ruleSetPaths map[string]string) ([]rule, map[string]string, []string) {
	var warnings []string
	used := make(map[string]string)
	rules := make([]rule, 0, 8)

	rules = append(rules, rule{Action: "sniff"})
	if exeName != "" {
		rules = append(rules, rule{
			Action:      "route",
			Outbound:    "direct",
			ProcessName: []string{exeName},
		})
	}

	if cfg.Routing.BlockAds {
		if path, ok := ruleSetPaths["geosite-ads"]; ok && path != "" {
			used["geosite-ads"] = path
			rules = append(rules, rule{Action: "reject", RuleSet: []string{"geosite-ads"}})
		} else {
			warnings = append(warnings, "база рекламы не установлена: блокировка рекламы пропущена")
		}
	}

	if cfg.Routing.BypassLAN {
		rules = append(rules, rule{Action: "route", Outbound: "direct", IPIsPrivate: true})
	}

	rulesMode := cfg.Routing.Mode == config.ModeRules
	if rulesMode && final != "direct" {
		if domains := cleanDomains(cfg.Routing.ProxyDomains, &warnings, "proxy"); len(domains) > 0 {
			rules = append(rules, rule{Action: "route", Outbound: final, DomainSuffix: domains})
		}
		if cidrs := cleanCIDRs(cfg.Routing.ProxyCIDRs, &warnings, "proxy"); len(cidrs) > 0 {
			rules = append(rules, rule{Action: "route", Outbound: final, IPCIDR: cidrs})
		}
	}
	if rulesMode {
		if domains := cleanDomains(cfg.Routing.DirectDomains, &warnings, "direct"); len(domains) > 0 {
			rules = append(rules, rule{Action: "route", Outbound: "direct", DomainSuffix: domains})
		}
		if cidrs := cleanCIDRs(cfg.Routing.DirectCIDRs, &warnings, "direct"); len(cidrs) > 0 {
			rules = append(rules, rule{Action: "route", Outbound: "direct", IPCIDR: cidrs})
		}
	}

	if cfg.Routing.BypassRU && rulesMode && final != "direct" {
		var tags []string
		for _, tag := range []string{"geosite-ru", "geoip-ru"} {
			if path, ok := ruleSetPaths[tag]; ok && path != "" {
				tags = append(tags, tag)
				used[tag] = path
			}
		}
		if len(tags) > 0 {
			rules = append(rules, rule{Action: "route", Outbound: "direct", RuleSet: tags})
		} else {
			warnings = append(warnings, "гео-базы РФ не установлены: обход РФ пропущен")
		}
	}

	return rules, used, warnings
}

func cleanDomains(lines []string, warnings *[]string, listName string) []string {
	seen := make(map[string]struct{}, len(lines))
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		value := strings.TrimPrefix(line, ".")
		if value == "" || strings.Contains(value, "://") || strings.ContainsAny(value, "/*\\ \t") {
			*warnings = append(*warnings, fmt.Sprintf("список %s: %q не похоже на домен", listName, line))
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func cleanCIDRs(lines []string, warnings *[]string, listName string) []string {
	seen := make(map[string]struct{}, len(lines))
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		value := line
		if addr, err := netip.ParseAddr(line); err == nil {
			if addr.Is4() {
				value = addr.String() + "/32"
			} else {
				value = addr.String() + "/128"
			}
		} else if prefix, err := netip.ParsePrefix(line); err == nil {
			value = prefix.Masked().String()
		} else {
			*warnings = append(*warnings, fmt.Sprintf("список %s: %q не является IP или CIDR", listName, line))
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func withTag(raw json.RawMessage, tag string) (json.RawMessage, error) {
	return jsonutil.SetObjectTag(raw, tag)
}

// CachePath returns the cache database path for a cache directory.
func CachePath(cacheDir string) string {
	return filepath.Join(cacheDir, "cache.db")
}
