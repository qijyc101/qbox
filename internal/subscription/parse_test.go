package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func parseOne(t *testing.T, line string) map[string]any {
	t.Helper()
	nodes, warnings, err := Parse([]byte(line))
	if err != nil {
		t.Fatalf("Parse(%q): %v (warnings %v)", line, err, warnings)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes = %d, warnings %v", len(nodes), warnings)
	}
	var obj map[string]any
	if err := json.Unmarshal(nodes[0].Outbound, &obj); err != nil {
		t.Fatal(err)
	}
	if obj["tag"] != nodes[0].Tag {
		t.Fatalf("tag mismatch: %v vs %q", obj["tag"], nodes[0].Tag)
	}
	return obj
}

func TestParseVLESS(t *testing.T) {
	obj := parseOne(t, "vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&sni=cdn.example.com&type=ws&path=%2Fws&host=cdn.example.com&fp=chrome#Test%20Node")
	if obj["type"] != "vless" || obj["server"] != "example.com" || obj["server_port"] != float64(443) {
		t.Fatalf("outbound = %#v", obj)
	}
	tlsObj, _ := obj["tls"].(map[string]any)
	if tlsObj["enabled"] != true || tlsObj["server_name"] != "cdn.example.com" {
		t.Fatalf("tls = %#v", obj["tls"])
	}
	utls, _ := tlsObj["utls"].(map[string]any)
	if utls["fingerprint"] != "chrome" {
		t.Fatalf("utls = %#v", tlsObj["utls"])
	}
	tr, _ := obj["transport"].(map[string]any)
	if tr["type"] != "ws" || tr["path"] != "/ws" {
		t.Fatalf("transport = %#v", obj["transport"])
	}
	if obj["tag"] != "Test Node" {
		t.Fatalf("tag = %v", obj["tag"])
	}
}

func TestParseVLESSReality(t *testing.T) {
	obj := parseOne(t, "vless://u@example.com:443?security=reality&pbk=KEY&sid=ab&sni=example.com&flow=xtls-rprx-vision&type=tcp#R")
	tlsObj, _ := obj["tls"].(map[string]any)
	reality, _ := tlsObj["reality"].(map[string]any)
	if reality["public_key"] != "KEY" || reality["short_id"] != "ab" {
		t.Fatalf("reality = %#v", tlsObj)
	}
	if obj["flow"] != "xtls-rprx-vision" {
		t.Fatalf("flow = %v", obj["flow"])
	}
}

func TestParseVMessBase64(t *testing.T) {
	doc := `{"v":"2","ps":"VM","add":"vm.example.com","port":"8443","id":"uuid","aid":"0","scy":"auto","net":"ws","host":"vm.example.com","path":"/ray","tls":"tls","sni":"vm.example.com"}`
	line := "vmess://" + base64.StdEncoding.EncodeToString([]byte(doc))
	obj := parseOne(t, line)
	if obj["type"] != "vmess" || obj["uuid"] != "uuid" || obj["server_port"] != float64(8443) {
		t.Fatalf("outbound = %#v", obj)
	}
	if obj["tag"] != "VM" {
		t.Fatalf("tag = %v", obj["tag"])
	}
	tr, _ := obj["transport"].(map[string]any)
	if tr["path"] != "/ray" {
		t.Fatalf("transport = %#v", obj["transport"])
	}
}

func TestParseTrojan(t *testing.T) {
	obj := parseOne(t, "trojan://secret@example.com:443?sni=example.com&type=grpc&serviceName=svc#T")
	if obj["type"] != "trojan" || obj["password"] != "secret" {
		t.Fatalf("outbound = %#v", obj)
	}
	tr, _ := obj["transport"].(map[string]any)
	if tr["type"] != "grpc" || tr["service_name"] != "svc" {
		t.Fatalf("transport = %#v", obj["transport"])
	}
}

func TestParseShadowsocks(t *testing.T) {
	userinfo := base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:pass"))
	obj := parseOne(t, "ss://"+userinfo+"@1.2.3.4:8388#SS")
	if obj["type"] != "shadowsocks" || obj["method"] != "aes-256-gcm" || obj["password"] != "pass" {
		t.Fatalf("outbound = %#v", obj)
	}
	if obj["server"] != "1.2.3.4" || obj["server_port"] != float64(8388) {
		t.Fatalf("address = %#v", obj)
	}

	legacy := base64.StdEncoding.EncodeToString([]byte("aes-128-gcm:pw@5.6.7.8:8389"))
	obj = parseOne(t, "ss://"+legacy+"#Legacy")
	if obj["method"] != "aes-128-gcm" || obj["server"] != "5.6.7.8" {
		t.Fatalf("legacy outbound = %#v", obj)
	}
}

func TestParseHysteria2(t *testing.T) {
	obj := parseOne(t, "hysteria2://pass@example.com:443?sni=example.com&insecure=1&obfs=salamander&obfs-password=op#H2")
	if obj["type"] != "hysteria2" || obj["server_port"] != float64(443) {
		t.Fatalf("outbound = %#v", obj)
	}
	tlsObj, _ := obj["tls"].(map[string]any)
	if tlsObj["insecure"] != true {
		t.Fatalf("tls = %#v", obj["tls"])
	}
	obfs, _ := obj["obfs"].(map[string]any)
	if obfs["type"] != "salamander" || obfs["password"] != "op" {
		t.Fatalf("obfs = %#v", obj["obfs"])
	}
}

func TestParseTUIC(t *testing.T) {
	obj := parseOne(t, "tuic://uuid:password@example.com:443?sni=example.com&congestion_control=bbr&udp_relay_mode=native&alpn=h3#TUIC")
	if obj["type"] != "tuic" || obj["uuid"] != "uuid" || obj["password"] != "password" {
		t.Fatalf("outbound = %#v", obj)
	}
	if obj["congestion_control"] != "bbr" || obj["udp_relay_mode"] != "native" {
		t.Fatalf("options = %#v", obj)
	}
}

func TestParseBase64List(t *testing.T) {
	list := "vless://u@a.example:443?security=tls#A\nvless://u@b.example:443?security=tls#B\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(list))
	nodes, warnings, err := Parse([]byte(encoded))
	if err != nil {
		t.Fatalf("Parse: %v (warnings %v)", err, warnings)
	}
	if len(nodes) != 2 || nodes[0].Tag != "A" || nodes[1].Tag != "B" {
		t.Fatalf("nodes = %+v", nodes)
	}
}

func TestParseSingBoxJSON(t *testing.T) {
	doc := `{"outbounds":[
		{"type":"vless","tag":"one","server":"a.example","server_port":443,"uuid":"u"},
		{"type":"direct","tag":"direct"},
		{"type":"trojan","server":"b.example","server_port":443,"password":"p"}
	]}`
	nodes, warnings, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("nodes = %+v warnings=%v", nodes, warnings)
	}
	if nodes[0].Tag != "one" || nodes[1].Tag == "" {
		t.Fatalf("tags = %q %q", nodes[0].Tag, nodes[1].Tag)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestDuplicateTagsAreMadeUnique(t *testing.T) {
	list := "vless://u@a.example:443?security=tls#Same\nvless://u@b.example:443?security=tls#Same\n"
	nodes, _, err := Parse([]byte(list))
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].Tag == nodes[1].Tag {
		t.Fatalf("tags collide: %q", nodes[0].Tag)
	}
}

func TestUnsupportedLinesAreReported(t *testing.T) {
	list := "vless://u@a.example:443?security=tls#A\nwireguard://nope\nclash://nope\n"
	nodes, warnings, err := Parse([]byte(list))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || len(warnings) != 2 {
		t.Fatalf("nodes=%d warnings=%v", len(nodes), warnings)
	}
	if !strings.Contains(warnings[0], "строка 2") {
		t.Fatalf("warning = %q", warnings[0])
	}
}

func TestUppercaseScheme(t *testing.T) {
	obj := parseOne(t, "VLESS://u@example.com:443?security=tls#Upper")
	if obj["type"] != "vless" || obj["server"] != "example.com" {
		t.Fatalf("outbound = %#v", obj)
	}
}

func TestShadowsocksPluginRejected(t *testing.T) {
	userinfo := base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:pass"))
	line := "ss://" + userinfo + "@1.2.3.4:8388?plugin=v2ray-plugin#SS"
	nodes, warnings, err := Parse([]byte(line))
	if err == nil || len(nodes) != 0 || len(warnings) != 1 {
		t.Fatalf("nodes=%d warnings=%v err=%v", len(nodes), warnings, err)
	}
	if !strings.Contains(warnings[0], "плагин") {
		t.Fatalf("warning = %q", warnings[0])
	}
}

func TestVMessPercentEncodedPayload(t *testing.T) {
	doc := `{"v":"2","ps":"VM","add":"vm.example.com","port":"443","id":"uuid","net":"ws","path":"/","tls":"tls"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(doc))
	line := "vmess://" + strings.ReplaceAll(encoded, "=", "%3D")
	obj := parseOne(t, line)
	if obj["tag"] != "VM" || obj["server"] != "vm.example.com" {
		t.Fatalf("outbound = %#v", obj)
	}
}

func TestReservedTagIsRenamed(t *testing.T) {
	nodes, _, err := Parse([]byte("vless://u@example.com:443?security=tls#auto"))
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].Tag == "auto" {
		t.Fatalf("tag = %q", nodes[0].Tag)
	}
}

func TestTagIsTruncated(t *testing.T) {
	long := strings.Repeat("я", maxTagRunes+50)
	nodes, _, err := Parse([]byte("vless://u@example.com:443?security=tls#" + url.QueryEscape(long)))
	if err != nil {
		t.Fatal(err)
	}
	if got := len([]rune(nodes[0].Tag)); got > maxTagRunes {
		t.Fatalf("tag length = %d", got)
	}
}

func TestNodeLimit(t *testing.T) {
	var builder strings.Builder
	for i := 0; i < MaxNodes+5; i++ {
		fmt.Fprintf(&builder, "vless://u@example.com:443?security=tls#n%d\n", i)
	}
	nodes, warnings, err := Parse([]byte(builder.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != MaxNodes {
		t.Fatalf("nodes = %d", len(nodes))
	}
	if len(warnings) == 0 {
		t.Fatal("no truncation warning")
	}
}

func TestJSONOutboundWithoutServerIsSkipped(t *testing.T) {
	doc := `{"outbounds":[{"type":"vless","tag":"bad","server_port":443,"uuid":"u"},{"type":"vless","tag":"ok","server":"example.com","server_port":443,"uuid":"u"}]}`
	nodes, warnings, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Tag != "ok" || len(warnings) != 1 {
		t.Fatalf("nodes=%+v warnings=%v", nodes, warnings)
	}
}

func TestInvalidServerIsRejected(t *testing.T) {
	nodes, warnings, err := Parse([]byte("vless://u@exa%20mple.com:443?security=tls#bad"))
	if err == nil && len(nodes) != 0 {
		t.Fatalf("nodes=%+v warnings=%v", nodes, warnings)
	}
}

func TestJSONShadowsocksPluginRejected(t *testing.T) {
	doc := `{"outbounds":[
		{"type":"shadowsocks","tag":"bad","server":"example.com","server_port":8388,"method":"aes-256-gcm","password":"p","plugin":"evil"},
		{"type":"shadowsocks","tag":"ok","server":"example.com","server_port":8388,"method":"aes-256-gcm","password":"p"}
	]}`
	nodes, warnings, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Tag != "ok" {
		t.Fatalf("nodes=%+v", nodes)
	}
	if len(warnings) != 1 || !strings.Contains(strings.ToLower(warnings[0]), "плагин") {
		t.Fatalf("warnings=%v", warnings)
	}
}

func TestJSONTLSFilePathsRejected(t *testing.T) {
	doc := `{"outbounds":[
		{"type":"vless","tag":"bad","server":"example.com","server_port":443,"uuid":"u","tls":{"enabled":true,"certificate_path":"C:\\secret.pem"}},
		{"type":"vless","tag":"ok","server":"example.com","server_port":443,"uuid":"u","tls":{"enabled":true}}
	]}`
	nodes, warnings, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Tag != "ok" {
		t.Fatalf("nodes=%+v", nodes)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "certificate_path") {
		t.Fatalf("warnings=%v", warnings)
	}
}

func TestCleanTagDropsBidiControls(t *testing.T) {
	if got := cleanTag("a\u202eb\u200b"); got != "ab" {
		t.Fatalf("cleanTag = %q", got)
	}
}

func TestMalformedURLErrorDoesNotLeakPassword(t *testing.T) {
	nodes, warnings, err := Parse([]byte("trojan://SECRET@exa mple.com:443#x"))
	if err == nil && len(nodes) > 0 {
		t.Fatalf("nodes=%+v", nodes)
	}
	for _, warning := range warnings {
		if strings.Contains(warning, "SECRET") {
			t.Fatalf("password leaked: %q", warning)
		}
	}
}

func TestEmptyUsernameKeepsPassword(t *testing.T) {
	obj := parseOne(t, "hysteria2://:pass@example.com:443#h")
	if obj["password"] != "pass" {
		t.Fatalf("password = %v", obj["password"])
	}
	obj = parseOne(t, "trojan://:pass@example.com:443#t")
	if obj["password"] != "pass" {
		t.Fatalf("password = %v", obj["password"])
	}
}

func TestClashYAMLIsRejected(t *testing.T) {
	_, _, err := Parse([]byte("proxies:\n  - name: x\n    type: ss\n"))
	if err == nil || !strings.Contains(err.Error(), "Clash YAML") {
		t.Fatalf("err = %v", err)
	}
}
