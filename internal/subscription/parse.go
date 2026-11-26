package subscription

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"qbox/internal/config"
	"qbox/internal/jsonutil"
)

// supportedTypes are the outbound types qbox can build from a subscription.
var supportedTypes = map[string]struct{}{
	"vless":       {},
	"vmess":       {},
	"trojan":      {},
	"shadowsocks": {},
	"hysteria2":   {},
	"tuic":        {},
	"socks":       {},
	"http":        {},
}

const (
	// MaxNodes bounds how many nodes one subscription can contribute.
	MaxNodes = 3000
	// maxTagRunes keeps tags readable and bounds config/UI growth.
	maxTagRunes = 128
)

type outboundMeta struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
}

type singBoxDoc struct {
	Outbounds []json.RawMessage `json:"outbounds"`
}

// Parse turns a subscription body into nodes. Warnings describe skipped
// lines; an error means nothing usable was found.
func Parse(data []byte) ([]config.Node, []string, error) {
	text := strings.TrimPrefix(string(data), "\ufeff")
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil, errors.New("пустое содержимое подписки")
	}
	nodes, warnings, err := parseContent(text, 0)
	if err != nil {
		return nil, warnings, err
	}
	if len(nodes) > MaxNodes {
		warnings = append(warnings, fmt.Sprintf("подписка обрезана до %d нод", MaxNodes))
		nodes = nodes[:MaxNodes]
	}
	return nodes, warnings, nil
}

func parseContent(text string, depth int) ([]config.Node, []string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil, errors.New("пустое содержимое подписки")
	}

	// Some providers base64-wrap even JSON documents.
	if depth < 2 {
		if decoded, ok := decodeBase64Text(text); ok {
			trimmed := strings.TrimSpace(string(decoded))
			if trimmed != text {
				return parseContent(trimmed, depth+1)
			}
		}
	}

	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		return parseSingBoxJSON([]byte(text))
	}
	if looksLikeClashYAML(text) {
		return nil, nil, errors.New("подписка в формате Clash YAML пока не поддерживается")
	}

	nodes, warnings := parseLines(text)
	if len(nodes) == 0 {
		if len(warnings) > 0 {
			return nil, warnings, fmt.Errorf("не удалось разобрать ни одной ноды: %s", warnings[0])
		}
		return nil, nil, errors.New("в подписке не найдено нод")
	}
	return uniqueTags(nodes), warnings, nil
}

func parseSingBoxJSON(data []byte) ([]config.Node, []string, error) {
	var doc singBoxDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		var outbounds []json.RawMessage
		if err2 := json.Unmarshal(data, &outbounds); err2 != nil {
			return nil, nil, fmt.Errorf("JSON подписки: %w", err)
		}
		doc.Outbounds = outbounds
	}
	if len(doc.Outbounds) == 0 {
		return nil, nil, errors.New("в JSON подписки нет outbounds")
	}

	nodes := make([]config.Node, 0, min(len(doc.Outbounds), MaxNodes+1))
	var warnings []string
	for i, raw := range doc.Outbounds {
		if len(nodes) >= MaxNodes {
			warnings = append(warnings, fmt.Sprintf("подписка обрезана до %d нод", MaxNodes))
			break
		}
		var meta outboundMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			warnings = append(warnings, fmt.Sprintf("outbound %d: %v", i+1, err))
			continue
		}
		if _, ok := supportedTypes[meta.Type]; !ok {
			warnings = append(warnings, fmt.Sprintf("outbound %d (%s): тип не поддерживается", i+1, meta.Type))
			continue
		}
		if meta.Server == "" || meta.ServerPort <= 0 || meta.ServerPort > 65535 {
			warnings = append(warnings, fmt.Sprintf("outbound %d (%s): нет адреса или порта", i+1, meta.Type))
			continue
		}
		server, err := cleanServer(meta.Server)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("outbound %d (%s): %v", i+1, meta.Type, err))
			continue
		}
		meta.Server = server
		tag := cleanTag(meta.Tag)
		if tag == "" {
			tag = defaultTag(meta.Type, meta.Server, meta.ServerPort)
		}
		raw, err := sanitizeJSONOutbound(raw, meta.Type)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("outbound %d: %v", i+1, err))
			continue
		}
		raw, err = setJSONTag(raw, tag)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("outbound %d: %v", i+1, err))
			continue
		}
		nodes = append(nodes, config.Node{
			Tag:      tag,
			Type:     meta.Type,
			Server:   meta.Server,
			Port:     meta.ServerPort,
			Outbound: raw,
		})
	}
	if len(nodes) == 0 {
		return nil, warnings, errors.New("в JSON подписки нет поддерживаемых нод")
	}
	return uniqueTags(nodes), warnings, nil
}

func parseLines(text string) ([]config.Node, []string) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	nodes := make([]config.Node, 0, min(len(lines), MaxNodes+1))
	var warnings []string
	for i, line := range lines {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if !strings.Contains(line, "://") {
			warnings = append(warnings, fmt.Sprintf("строка %d: не похожа на ссылку", i+1))
			continue
		}
		if len(nodes) >= MaxNodes {
			warnings = append(warnings, fmt.Sprintf("подписка обрезана до %d нод", MaxNodes))
			break
		}
		outbound, err := parseURI(line)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("строка %d: %v", i+1, err))
			continue
		}
		node, err := nodeFromMap(outbound)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("строка %d: %v", i+1, err))
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes, warnings
}

func nodeFromMap(outbound map[string]any) (config.Node, error) {
	typ, _ := outbound["type"].(string)
	if _, ok := supportedTypes[typ]; !ok {
		return config.Node{}, fmt.Errorf("тип %q не поддерживается", typ)
	}
	server, _ := outbound["server"].(string)
	port := toInt(outbound["server_port"])
	server, err := cleanServer(server)
	if err != nil {
		return config.Node{}, err
	}
	if port <= 0 || port > 65535 {
		return config.Node{}, fmt.Errorf("некорректный порт %d", port)
	}
	tag := cleanTag(stringValue(outbound["tag"]))
	if tag == "" {
		tag = defaultTag(typ, server, port)
	}
	outbound["tag"] = tag

	raw, err := json.Marshal(outbound)
	if err != nil {
		return config.Node{}, err
	}
	return config.Node{
		Tag:      tag,
		Type:     typ,
		Server:   server,
		Port:     port,
		Outbound: raw,
	}, nil
}

func uniqueTags(nodes []config.Node) []config.Node {
	used := make(map[string]struct{}, len(nodes))
	next := make(map[string]int, len(nodes))
	for i := range nodes {
		base := cleanTag(nodes[i].Tag)
		if base == "" {
			base = defaultTag(nodes[i].Type, nodes[i].Server, nodes[i].Port)
		}
		if base == "auto" || base == "direct" {
			base += " (нода)"
		}
		tag := base
		if _, exists := used[tag]; exists {
			n := next[base]
			if n < 2 {
				n = 2
			}
			for {
				tag = fmt.Sprintf("%s (%d)", base, n)
				if _, ok := used[tag]; !ok {
					break
				}
				n++
			}
			next[base] = n + 1
		}
		used[tag] = struct{}{}
		nodes[i].Tag = tag
		if raw, err := setJSONTag(nodes[i].Outbound, tag); err == nil {
			nodes[i].Outbound = raw
		}
	}
	return nodes
}

func setJSONTag(raw json.RawMessage, tag string) (json.RawMessage, error) {
	return jsonutil.SetObjectTag(raw, tag)
}

// sanitizeJSONOutbound rejects outbound fields that would execute local
// code or read local files. A subscription is untrusted input, while qbox
// runs elevated to create TUN.
func sanitizeJSONOutbound(raw json.RawMessage, typ string) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if typ == "shadowsocks" {
		if plugin, ok := obj["plugin"]; ok {
			var name string
			if err := json.Unmarshal(plugin, &name); err != nil {
				return nil, fmt.Errorf("plugin: %w", err)
			}
			if strings.TrimSpace(name) != "" {
				return nil, errors.New("плагин Shadowsocks не разрешён в подписке")
			}
		}
	}
	if tlsRaw, ok := obj["tls"]; ok {
		var tls map[string]json.RawMessage
		if err := json.Unmarshal(tlsRaw, &tls); err == nil {
			for _, field := range []string{"certificate_path", "client_certificate_path", "client_key_path"} {
				if nonEmptyRawString(tls[field]) {
					return nil, fmt.Errorf("tls.%s не разрешён в подписке", field)
				}
			}
			if echRaw, ok := tls["ech"]; ok {
				var ech map[string]json.RawMessage
				if json.Unmarshal(echRaw, &ech) == nil && nonEmptyRawString(ech["config_path"]) {
					return nil, errors.New("tls.ech.config_path не разрешён в подписке")
				}
			}
		}
	}
	return raw, nil
}

func nonEmptyRawString(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return strings.TrimSpace(value) != ""
}

func decodeBase64Text(text string) ([]byte, bool) {
	if strings.Contains(text, "://") {
		return nil, false
	}
	compact := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		}
		return r
	}, text)
	if compact == "" {
		return nil, false
	}
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, enc := range encodings {
		decoded, err := enc.DecodeString(compact)
		if err != nil || len(decoded) == 0 {
			continue
		}
		if !utf8.Valid(decoded) {
			continue
		}
		trimmed := bytes.TrimSpace(decoded)
		if bytes.Contains(trimmed, []byte("://")) ||
			bytes.HasPrefix(trimmed, []byte("{")) ||
			bytes.HasPrefix(trimmed, []byte("[")) ||
			bytes.Contains(trimmed, []byte("\n")) {
			return decoded, true
		}
	}
	return nil, false
}

func looksLikeClashYAML(text string) bool {
	return strings.Contains(text, "proxies:") && !strings.Contains(text, "://")
}

func cleanTag(tag string) string {
	tag = strings.TrimSpace(tag)
	tag = strings.Map(func(r rune) rune {
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
	}, tag)
	return strings.TrimSpace(truncateRunes(tag, maxTagRunes))
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func defaultTag(typ, server string, port int) string {
	if server == "" {
		return typ
	}
	if port > 0 {
		return truncateRunes(fmt.Sprintf("%s %s:%d", typ, server, port), maxTagRunes)
	}
	return truncateRunes(fmt.Sprintf("%s %s", typ, server), maxTagRunes)
}

func cleanServer(server string) (string, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return "", errors.New("пустой адрес сервера")
	}
	if len(server) > 253 {
		return "", errors.New("слишком длинный адрес сервера")
	}
	for _, r := range server {
		if unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune(`/\?#@`, r) {
			return "", errors.New("некорректный адрес сервера")
		}
	}
	return server, nil
}
