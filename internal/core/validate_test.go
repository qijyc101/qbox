package core

import (
	"encoding/json"
	"strings"
	"testing"

	"qbox/internal/config"
)

func nodeConfig(t *testing.T, node config.Node) config.Config {
	t.Helper()
	cfg := config.Defaults()
	cfg.Subscriptions = []config.Subscription{{ID: "sub", Name: "test", Nodes: []config.Node{node}}}
	cfg.ActiveSubscription = "sub"
	return cfg
}

func TestCanonicalizeRejectsTor(t *testing.T) {
	cfg := nodeConfig(t, config.Node{
		Tag:      "bad",
		Type:     "vless",
		Server:   "example.com",
		Port:     443,
		Outbound: json.RawMessage(`{"type":"tor","tag":"bad","executable_path":"evil.exe"}`),
	})
	err := canonicalizeNodes(&cfg)
	if err == nil || !strings.Contains(err.Error(), "запрещён") {
		t.Fatalf("err = %v", err)
	}
}

func TestCanonicalizeRejectsCaseFoldedType(t *testing.T) {
	cfg := nodeConfig(t, config.Node{
		Tag:      "bad",
		Type:     "vless",
		Server:   "example.com",
		Port:     443,
		Outbound: json.RawMessage(`{"TYPE":"tor","TAG":"bad","executable_path":"evil.exe"}`),
	})
	if err := canonicalizeNodes(&cfg); err == nil {
		t.Fatal("case-folded tor outbound was accepted")
	}
}

func TestCanonicalizeRejectsShadowsocksPlugin(t *testing.T) {
	cfg := nodeConfig(t, config.Node{
		Tag:      "bad",
		Type:     "shadowsocks",
		Server:   "example.com",
		Port:     8388,
		Outbound: json.RawMessage(`{"type":"shadowsocks","tag":"bad","server":"example.com","server_port":8388,"method":"aes-256-gcm","password":"p","plugin":"evil"}`),
	})
	if err := canonicalizeNodes(&cfg); err == nil {
		t.Fatal("shadowsocks plugin was accepted")
	}
}

func TestCanonicalizeRejectsTLSFilePaths(t *testing.T) {
	cfg := nodeConfig(t, config.Node{
		Tag:      "bad",
		Type:     "vless",
		Server:   "example.com",
		Port:     443,
		Outbound: json.RawMessage(`{"type":"vless","tag":"bad","server":"example.com","server_port":443,"uuid":"u","tls":{"enabled":true,"certificate_path":"C:\\secret.pem"}}`),
	})
	if err := canonicalizeNodes(&cfg); err == nil {
		t.Fatal("tls certificate_path was accepted")
	}
}

func TestCanonicalizeAcceptsValidNode(t *testing.T) {
	cfg := nodeConfig(t, config.Node{
		Tag:      "ok",
		Type:     "vless",
		Server:   "example.com",
		Port:     443,
		Outbound: json.RawMessage(`{"type":"vless","tag":"ok","server":"example.com","server_port":443,"uuid":"u","tls":{"enabled":true}}`),
	})
	if err := canonicalizeNodes(&cfg); err != nil {
		t.Fatal(err)
	}
	node := cfg.ActiveNodes()[0]
	if node.Tag != "ok" || node.Type != "vless" {
		t.Fatalf("node = %+v", node)
	}
	var check map[string]any
	if err := json.Unmarshal(node.Outbound, &check); err != nil {
		t.Fatal(err)
	}
	if check["type"] != "vless" || check["tag"] != "ok" {
		t.Fatalf("outbound = %s", node.Outbound)
	}
}

func TestCanonicalizeSkipsDirectModeNodes(t *testing.T) {
	cfg := nodeConfig(t, config.Node{
		Tag:      "bad",
		Type:     "vless",
		Outbound: json.RawMessage(`null`),
	})
	cfg.Routing.Mode = config.ModeDirect
	if err := canonicalizeNodes(&cfg); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalizeRejectsDuplicateTags(t *testing.T) {
	cfg := config.Defaults()
	cfg.Subscriptions = []config.Subscription{{ID: "sub", Nodes: []config.Node{
		{Tag: "same", Type: "vless", Outbound: json.RawMessage(`{"type":"vless","tag":"same","server":"a.example","server_port":443,"uuid":"u"}`)},
		{Tag: "same", Type: "vless", Outbound: json.RawMessage(`{"type":"vless","tag":"same","server":"b.example","server_port":443,"uuid":"u"}`)},
	}}}
	cfg.ActiveSubscription = "sub"
	if err := canonicalizeNodes(&cfg); err == nil {
		t.Fatal("duplicate tags were accepted")
	}
}
