package subscription

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func parseURI(line string) (map[string]any, error) {
	if i := strings.Index(line, "://"); i > 0 {
		line = strings.ToLower(line[:i]) + line[i:]
	}
	lower := strings.ToLower(line)
	switch {
	case strings.HasPrefix(lower, "vless://"):
		return parseVLESS(line)
	case strings.HasPrefix(lower, "vmess://"):
		return parseVMess(line)
	case strings.HasPrefix(lower, "trojan://"):
		return parseTrojan(line)
	case strings.HasPrefix(lower, "ss://"):
		return parseShadowsocks(line)
	case strings.HasPrefix(lower, "hysteria2://"), strings.HasPrefix(lower, "hy2://"):
		return parseHysteria2(line)
	case strings.HasPrefix(lower, "tuic://"):
		return parseTUIC(line)
	case strings.HasPrefix(lower, "socks://"), strings.HasPrefix(lower, "socks5://"):
		return parseSOCKS(line)
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return parseHTTP(line)
	default:
		scheme := line
		if i := strings.Index(scheme, "://"); i > 0 {
			scheme = scheme[:i]
		}
		return nil, fmt.Errorf("схема %q не поддерживается", scheme)
	}
}

func parseVLESS(line string) (map[string]any, error) {
	u, err := parseURL(line, "vless")
	if err != nil {
		return nil, err
	}
	uuid := u.User.Username()
	if uuid == "" {
		return nil, errors.New("vless: нет UUID")
	}
	server, port, err := hostPort(u, 443)
	if err != nil {
		return nil, fmt.Errorf("vless: %w", err)
	}
	q := u.Query()
	out := map[string]any{
		"type":        "vless",
		"tag":         tagFromURL(u),
		"server":      server,
		"server_port": port,
		"uuid":        uuid,
	}
	if flow := q.Get("flow"); flow != "" {
		out["flow"] = flow
	}
	if pe := firstQuery(q, "packetEncoding", "packet_encoding", "packet-encoding"); pe != "" {
		out["packet_encoding"] = pe
	}

	security := strings.ToLower(q.Get("security"))
	if security == "" {
		security = "none"
	}
	switch security {
	case "none", "tls", "reality":
	default:
		return nil, fmt.Errorf("vless: security %q не поддерживается", security)
	}
	if security != "none" {
		out["tls"] = buildTLS(q, security)
	}

	transport, err := buildTransport(q.Get("type"), q.Get("host"), q.Get("path"), q.Get("serviceName"), q.Get("mode"), q)
	if err != nil {
		return nil, fmt.Errorf("vless: %w", err)
	}
	if transport != nil {
		out["transport"] = transport
	}
	return out, nil
}

func parseVMess(line string) (map[string]any, error) {
	payload := strings.TrimPrefix(line, "vmess://")
	if strings.Contains(payload, "@") {
		return parseVMessURI(line)
	}
	if unescaped, err := url.PathUnescape(payload); err == nil {
		payload = unescaped
	}
	decoded, err := decodeBase64String(payload)
	if err != nil {
		return nil, fmt.Errorf("vmess: base64: %w", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(decoded, &doc); err != nil {
		return nil, fmt.Errorf("vmess: JSON: %w", err)
	}

	server := stringValue(doc["add"])
	port := toInt(doc["port"])
	if server == "" || port <= 0 {
		return nil, errors.New("vmess: нет адреса или порта")
	}
	uuid := stringValue(doc["id"])
	if uuid == "" {
		return nil, errors.New("vmess: нет UUID")
	}
	security := stringValue(doc["scy"])
	if security == "" {
		security = "auto"
	}
	out := map[string]any{
		"type":        "vmess",
		"tag":         cleanTag(stringValue(doc["ps"])),
		"server":      server,
		"server_port": port,
		"uuid":        uuid,
		"security":    security,
	}
	if alter := toInt(doc["aid"]); alter > 0 {
		out["alter_id"] = alter
	}

	tlsMode := strings.ToLower(stringValue(doc["tls"]))
	if tlsMode != "" && tlsMode != "none" {
		q := url.Values{}
		if sni := stringValue(doc["sni"]); sni != "" {
			q.Set("sni", sni)
		}
		if alpn := stringValue(doc["alpn"]); alpn != "" {
			q.Set("alpn", alpn)
		}
		if fp := stringValue(doc["fp"]); fp != "" {
			q.Set("fp", fp)
		}
		if host := stringValue(doc["host"]); host != "" {
			q.Set("host", host)
		}
		if v, ok := doc["allowInsecure"].(bool); ok && v {
			q.Set("allowInsecure", "1")
		}
		out["tls"] = buildTLS(q, "tls")
	}

	transport, err := buildTransport(
		stringValue(doc["net"]),
		stringValue(doc["host"]),
		stringValue(doc["path"]),
		stringValue(doc["serviceName"]),
		"",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("vmess: %w", err)
	}
	if transport != nil {
		out["transport"] = transport
	}
	return out, nil
}

func parseVMessURI(line string) (map[string]any, error) {
	u, err := parseURL(line, "vmess")
	if err != nil {
		return nil, err
	}
	uuid := u.User.Username()
	server, port, err := hostPort(u, 443)
	if err != nil {
		return nil, fmt.Errorf("vmess: %w", err)
	}
	if uuid == "" {
		return nil, errors.New("vmess: нет UUID")
	}
	q := u.Query()
	out := map[string]any{
		"type":        "vmess",
		"tag":         tagFromURL(u),
		"server":      server,
		"server_port": port,
		"uuid":        uuid,
		"security":    "auto",
	}
	if security := q.Get("scy"); security != "" {
		out["security"] = security
	}
	if tlsMode := strings.ToLower(q.Get("security")); tlsMode == "tls" || tlsMode == "reality" {
		out["tls"] = buildTLS(q, tlsMode)
	}
	transport, err := buildTransport(q.Get("type"), q.Get("host"), q.Get("path"), q.Get("serviceName"), q.Get("mode"), q)
	if err != nil {
		return nil, fmt.Errorf("vmess: %w", err)
	}
	if transport != nil {
		out["transport"] = transport
	}
	return out, nil
}

func parseTrojan(line string) (map[string]any, error) {
	u, err := parseURL(line, "trojan")
	if err != nil {
		return nil, err
	}
	password := u.User.Username()
	if pass, ok := u.User.Password(); ok {
		if password == "" {
			password = pass
		} else {
			password += ":" + pass
		}
	}
	if password == "" {
		password = u.User.String()
	}
	if password == "" {
		return nil, errors.New("trojan: нет пароля")
	}
	server, port, err := hostPort(u, 443)
	if err != nil {
		return nil, fmt.Errorf("trojan: %w", err)
	}
	q := u.Query()
	out := map[string]any{
		"type":        "trojan",
		"tag":         tagFromURL(u),
		"server":      server,
		"server_port": port,
		"password":    password,
	}
	security := strings.ToLower(q.Get("security"))
	if security != "none" {
		out["tls"] = buildTLS(q, "tls")
	}
	transport, err := buildTransport(q.Get("type"), q.Get("host"), q.Get("path"), q.Get("serviceName"), q.Get("mode"), q)
	if err != nil {
		return nil, fmt.Errorf("trojan: %w", err)
	}
	if transport != nil {
		out["transport"] = transport
	}
	return out, nil
}

func parseShadowsocks(line string) (map[string]any, error) {
	raw := strings.TrimPrefix(line, "ss://")
	fragment := ""
	if i := strings.Index(raw, "#"); i >= 0 {
		fragment = raw[i+1:]
		raw = raw[:i]
	}
	if i := strings.Index(raw, "?"); i >= 0 {
		query, _ := url.ParseQuery(raw[i+1:])
		if query.Get("plugin") != "" {
			return nil, errors.New("ss: плагины (plugin=) не поддерживаются")
		}
		raw = raw[:i]
	}

	var method, password, hostport string
	if i := strings.LastIndex(raw, "@"); i >= 0 {
		userinfo := raw[:i]
		hostport = raw[i+1:]
		if decoded, err := decodeBase64String(userinfo); err == nil && strings.Contains(string(decoded), ":") {
			userinfo = string(decoded)
		} else if unescaped, err := url.PathUnescape(userinfo); err == nil {
			userinfo = unescaped
		}
		parts := strings.SplitN(userinfo, ":", 2)
		if len(parts) != 2 {
			return nil, errors.New("ss: нет метода или пароля")
		}
		method, password = parts[0], parts[1]
	} else {
		decoded, err := decodeBase64String(raw)
		if err != nil {
			return nil, fmt.Errorf("ss: base64: %w", err)
		}
		legacy := string(decoded)
		at := strings.LastIndex(legacy, "@")
		if at < 0 {
			return nil, errors.New("ss: неожиданный формат ссылки")
		}
		userinfo, host := legacy[:at], legacy[at+1:]
		parts := strings.SplitN(userinfo, ":", 2)
		if len(parts) != 2 {
			return nil, errors.New("ss: нет метода или пароля")
		}
		method, password, hostport = parts[0], parts[1], host
	}

	host, port, err := splitHostPort(hostport, 8388)
	if err != nil {
		return nil, fmt.Errorf("ss: %w", err)
	}
	if tag, err := url.PathUnescape(fragment); err == nil {
		fragment = tag
	}
	return map[string]any{
		"type":        "shadowsocks",
		"tag":         cleanTag(fragment),
		"server":      host,
		"server_port": port,
		"method":      strings.ToLower(method),
		"password":    password,
	}, nil
}

func parseHysteria2(line string) (map[string]any, error) {
	u, err := parseURL(line, "hysteria2")
	if err != nil {
		return nil, err
	}
	auth := u.User.Username()
	if pass, ok := u.User.Password(); ok {
		if auth == "" {
			auth = pass
		} else {
			auth += ":" + pass
		}
	}
	if auth == "" {
		auth = u.User.String()
	}
	server, port, err := hostPort(u, 443)
	if err != nil {
		return nil, fmt.Errorf("hysteria2: %w", err)
	}
	q := u.Query()
	out := map[string]any{
		"type":        "hysteria2",
		"tag":         tagFromURL(u),
		"server":      server,
		"server_port": port,
		"tls":         buildTLS(q, "tls"),
	}
	if auth != "" {
		out["password"] = auth
	}
	if obfs := strings.ToLower(q.Get("obfs")); obfs != "" {
		if obfs != "salamander" && obfs != "gecko" {
			return nil, fmt.Errorf("hysteria2: obfs %q не поддерживается", obfs)
		}
		obfsOptions := map[string]any{"type": obfs}
		if pass := firstQuery(q, "obfs-password", "obfs_password", "obfsPassword"); pass != "" {
			obfsOptions["password"] = pass
		}
		out["obfs"] = obfsOptions
	}
	if up := toInt(q.Get("up")); up > 0 {
		out["up_mbps"] = up
	}
	if down := toInt(q.Get("down")); down > 0 {
		out["down_mbps"] = down
	}
	return out, nil
}

func parseTUIC(line string) (map[string]any, error) {
	u, err := parseURL(line, "tuic")
	if err != nil {
		return nil, err
	}
	uuid := u.User.Username()
	password, _ := u.User.Password()
	if uuid == "" {
		return nil, errors.New("tuic: нет UUID")
	}
	server, port, err := hostPort(u, 443)
	if err != nil {
		return nil, fmt.Errorf("tuic: %w", err)
	}
	q := u.Query()
	out := map[string]any{
		"type":        "tuic",
		"tag":         tagFromURL(u),
		"server":      server,
		"server_port": port,
		"uuid":        uuid,
		"tls":         buildTLS(q, "tls"),
	}
	if password != "" {
		out["password"] = password
	}
	if cc := firstQuery(q, "congestion_control", "congestion-control", "congestion"); cc != "" {
		out["congestion_control"] = cc
	}
	if mode := firstQuery(q, "udp_relay_mode", "udp-relay-mode"); mode != "" {
		out["udp_relay_mode"] = mode
	}
	if qBool(q, "zero_rtt_handshake", "zero-rtt-handshake", "zero_rtt") {
		out["zero_rtt_handshake"] = true
	}
	return out, nil
}

func parseSOCKS(line string) (map[string]any, error) {
	u, err := parseURL(line, "socks")
	if err != nil {
		return nil, err
	}
	server, port, err := hostPort(u, 1080)
	if err != nil {
		return nil, fmt.Errorf("socks: %w", err)
	}
	out := map[string]any{
		"type":        "socks",
		"tag":         tagFromURL(u),
		"server":      server,
		"server_port": port,
		"version":     "5",
	}
	if user := u.User.Username(); user != "" {
		out["username"] = user
		if pass, ok := u.User.Password(); ok {
			out["password"] = pass
		}
	}
	return out, nil
}

func parseHTTP(line string) (map[string]any, error) {
	u, err := parseURL(line, "http")
	if err != nil {
		return nil, err
	}
	server, port, err := hostPort(u, 8080)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	out := map[string]any{
		"type":        "http",
		"tag":         tagFromURL(u),
		"server":      server,
		"server_port": port,
	}
	if user := u.User.Username(); user != "" {
		out["username"] = user
		if pass, ok := u.User.Password(); ok {
			out["password"] = pass
		}
	}
	if strings.EqualFold(u.Scheme, "https") {
		out["tls"] = map[string]any{
			"enabled":     true,
			"server_name": u.Hostname(),
		}
	}
	return out, nil
}

func buildTLS(q url.Values, security string) map[string]any {
	tls := map[string]any{"enabled": true}
	serverName := firstQuery(q, "sni", "peer", "servername", "server_name")
	if serverName == "" {
		serverName = q.Get("host")
	}
	if serverName != "" {
		tls["server_name"] = serverName
	}
	if alpn := q.Get("alpn"); alpn != "" {
		tls["alpn"] = splitList(alpn)
	}
	if qBool(q, "allowInsecure", "allow_insecure", "insecure", "skip-cert-verify") {
		tls["insecure"] = true
	}
	if fp := firstQuery(q, "fp", "fingerprint", "client-fingerprint", "client_fingerprint"); fp != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
	}
	if security == "reality" {
		reality := map[string]any{"enabled": true}
		if pbk := firstQuery(q, "pbk", "public-key", "public_key"); pbk != "" {
			reality["public_key"] = pbk
		}
		if sid := firstQuery(q, "sid", "short-id", "short_id"); sid != "" {
			reality["short_id"] = sid
		}
		tls["reality"] = reality
	}
	return tls
}

func buildTransport(kind, host, path, serviceName, mode string, q url.Values) (map[string]any, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "", "tcp", "none":
		return nil, nil
	case "ws", "websocket":
		transport := map[string]any{"type": "ws", "path": defaultString(path, "/")}
		if host != "" {
			transport["headers"] = map[string]any{"Host": host}
		}
		if q != nil {
			if ed := firstQuery(q, "ed", "max_early_data"); ed != "" {
				transport["max_early_data"] = toInt(ed)
			}
			if eh := firstQuery(q, "eh", "early_data_header_name"); eh != "" {
				transport["early_data_header_name"] = eh
			}
		}
		return transport, nil
	case "grpc":
		transport := map[string]any{"type": "grpc"}
		if serviceName != "" {
			transport["service_name"] = serviceName
		}
		return transport, nil
	case "httpupgrade":
		transport := map[string]any{"type": "httpupgrade", "path": defaultString(path, "/")}
		if host != "" {
			transport["host"] = host
		}
		return transport, nil
	case "http", "h2":
		transport := map[string]any{"type": "http", "path": defaultString(path, "/")}
		if host != "" {
			transport["host"] = splitList(host)
		}
		return transport, nil
	case "quic":
		return map[string]any{"type": "quic"}, nil
	default:
		return nil, fmt.Errorf("транспорт %q не поддерживается", kind)
	}
}

func parseURL(line, scheme string) (*url.URL, error) {
	u, err := url.Parse(line)
	if err != nil {
		return nil, fmt.Errorf("%s: некорректный URL", scheme)
	}
	return u, nil
}

func hostPort(u *url.URL, defaultPort int) (string, int, error) {
	host := u.Hostname()
	if host == "" {
		return "", 0, errors.New("нет адреса сервера")
	}
	port := defaultPort
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			return "", 0, fmt.Errorf("некорректный порт %q", p)
		}
		port = n
	}
	return host, port, nil
}

func splitHostPort(hostport string, defaultPort int) (string, int, error) {
	hostport = strings.TrimSpace(strings.TrimSuffix(hostport, "/"))
	if host, port, err := net.SplitHostPort(hostport); err == nil {
		n, convErr := strconv.Atoi(port)
		if convErr != nil || n <= 0 || n > 65535 {
			return "", 0, fmt.Errorf("некорректный порт %q", port)
		}
		return strings.Trim(host, "[]"), n, nil
	}
	trimmed := strings.Trim(hostport, "[]")
	if trimmed == "" {
		return "", 0, errors.New("нет адреса сервера")
	}
	return trimmed, defaultPort, nil
}

func tagFromURL(u *url.URL) string {
	if u.Fragment == "" {
		return ""
	}
	tag, err := url.PathUnescape(u.Fragment)
	if err != nil {
		tag = u.Fragment
	}
	return cleanTag(tag)
}

func firstQuery(q url.Values, names ...string) string {
	for _, name := range names {
		if value := q.Get(name); value != "" {
			return value
		}
	}
	return ""
}

func qBool(q url.Values, names ...string) bool {
	for _, name := range names {
		switch strings.ToLower(q.Get(name)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func decodeBase64String(value string) ([]byte, error) {
	compact := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		}
		return r
	}, value)
	var lastErr error
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		decoded, err := enc.DecodeString(compact)
		if err == nil {
			return decoded, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func toInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	default:
		return 0
	}
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
