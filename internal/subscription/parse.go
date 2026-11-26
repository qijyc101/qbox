package subscription

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"qbox/internal/config"
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
	return parseContent(text, 0)
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

	nodes := make([]config.Node, 0, len(doc.Outbounds))
	var warnings []string
	for i, raw := range doc.Outbounds {
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
		tag := cleanTag(meta.Tag)
		if tag == "" {
			tag = defaultTag(meta.Type, meta.Server, meta.ServerPort)
		}
		raw, err := setJSONTag(raw, tag)
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
	nodes := make([]config.Node, 0, len(lines))
	var warnings []string
	for i, line := range lines {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.Contains(line, "://") == false {
			warnings = append(warnings, fmt.Sprintf("строка %d: не похожа на ссылку", i+1))
			continue
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
	if server == "" {
		return config.Node{}, errors.New("пустой адрес сервера")
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
	for i := range nodes {
		base := cleanTag(nodes[i].Tag)
		if base == "" {
			base = defaultTag(nodes[i].Type, nodes[i].Server, nodes[i].Port)
		}
		if base == "auto" || base == "direct" {
			base += " (нода)"
		}
		tag := base
		for n := 2; ; n++ {
			if _, exists := used[tag]; !exists {
				break
			}
			tag = fmt.Sprintf("%s (%d)", base, n)
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
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(tag)
	if err != nil {
		return nil, err
	}
	obj["tag"] = encoded
	return json.Marshal(obj)
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
		if r < 0x20 {
			return -1
		}
		return r
	}, tag)
	return strings.TrimSpace(tag)
}

func defaultTag(typ, server string, port int) string {
	if server == "" {
		return typ
	}
	if port > 0 {
		return fmt.Sprintf("%s %s:%d", typ, server, port)
	}
	return fmt.Sprintf("%s %s", typ, server)
}
