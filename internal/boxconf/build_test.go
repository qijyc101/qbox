package boxconf

import (
	"encoding/json"
	"strings"
	"testing"

	"qbox/internal/config"
)

func sampleConfig() config.Config {
	cfg := config.Defaults()
	cfg.SelectedNode = "node1"
	cfg.Subscriptions = []config.Subscription{{
		ID:   "sub-1",
		Name: "Test",
		URL:  "https://example.com/sub",
		Nodes: []config.Node{{
			Tag:      "node1",
			Type:     "vless",
			Server:   "example.com",
			Port:     443,
			Outbound: json.RawMessage(`{"type":"vless","tag":"node1","server":"example.com","server_port":443,"uuid":"u","tls":{"enabled":true}}`),
		}},
	}}
	cfg.ActiveSubscription = "sub-1"
	return cfg
}

func buildDoc(t *testing.T, cfg config.Config, paths map[string]string) (map[string]any, []string) {
	t.Helper()
	data, warnings, err := Build(Input{
		Config:       cfg,
		RuleSetPaths: paths,
		ExeName:      "qbox.exe",
		CacheFile:    `C:\cache\cache.db`,
	})
	if err != nil {
		t.Fatalf("Build: %v (warnings %v)", err, warnings)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	return doc, warnings
}

func rules(doc map[string]any) []map[string]any {
	route := doc["route"].(map[string]any)
	raw := route["rules"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		out = append(out, item.(map[string]any))
	}
	return out
}

func TestBuildRulesMode(t *testing.T) {
	cfg := sampleConfig()
	paths := map[string]string{
		"geosite-ru":  `C:\rules\geosite-ru.srs`,
		"geoip-ru":    `C:\rules\geoip-ru.srs`,
		"geosite-ads": `C:\rules\ads.srs`,
	}
	cfg.Routing.ProxyDomains = []string{"openai.com"}
	cfg.Routing.DirectDomains = []string{"example.org"}
	cfg.Routing.DirectCIDRs = []string{"10.0.0.1", "bad-value"}
	doc, warnings := buildDoc(t, cfg, paths)

	inbounds := doc["inbounds"].([]any)
	tun := inbounds[0].(map[string]any)
	if tun["type"] != "tun" || tun["auto_route"] != true || tun["dns_mode"] != "hijack" || tun["stack"] != "mixed" {
		t.Fatalf("tun = %#v", tun)
	}

	route := doc["route"].(map[string]any)
	if route["final"] != "node1" {
		t.Fatalf("final = %v", route["final"])
	}
	if route["auto_detect_interface"] != true {
		t.Fatalf("auto_detect_interface = %v", route["auto_detect_interface"])
	}

	outbounds := doc["outbounds"].([]any)
	if len(outbounds) != 3 {
		t.Fatalf("outbounds = %d", len(outbounds))
	}
	auto := outbounds[1].(map[string]any)
	if auto["type"] != "urltest" || auto["tag"] != "auto" {
		t.Fatalf("auto = %#v", auto)
	}

	rs := route["rule_set"].([]any)
	if len(rs) != 3 {
		t.Fatalf("rule_set = %#v", rs)
	}
	for _, item := range rs {
		set := item.(map[string]any)
		if set["type"] != "local" || set["format"] != "binary" || set["path"] == "" {
			t.Fatalf("rule-set = %#v", set)
		}
	}

	list := rules(doc)
	var actions []string
	for _, rule := range list {
		if action, _ := rule["action"].(string); action != "" {
			actions = append(actions, action)
		}
	}
	if len(actions) < 4 || actions[0] != "sniff" {
		t.Fatalf("actions = %v", actions)
	}
	foundProcess := false
	foundReject := false
	foundRU := false
	foundDirectCIDR := false
	for _, rule := range list {
		if procs, ok := rule["process_name"].([]any); ok && len(procs) == 1 && procs[0] == "qbox.exe" {
			foundProcess = true
		}
		if rule["action"] == "reject" {
			foundReject = true
		}
		if cidrs, ok := rule["ip_cidr"].([]any); ok {
			for _, c := range cidrs {
				if c == "10.0.0.1/32" {
					foundDirectCIDR = true
				}
			}
		}
		if sets, ok := rule["rule_set"].([]any); ok && len(sets) == 2 && rule["outbound"] == "direct" {
			foundRU = true
		}
	}
	if !foundProcess || !foundReject || !foundRU || !foundDirectCIDR {
		t.Fatalf("missing rules: process=%v reject=%v ru=%v cidr=%v\n%#v", foundProcess, foundReject, foundRU, foundDirectCIDR, list)
	}

	dns := doc["dns"].(map[string]any)
	if dns["final"] != "remote" {
		t.Fatalf("dns final = %v", dns["final"])
	}
	servers := dns["servers"].([]any)
	remote := servers[1].(map[string]any)
	if remote["type"] != "https" || remote["server"] != "1.1.1.1" || remote["detour"] != "node1" {
		t.Fatalf("remote dns = %#v", remote)
	}

	if len(warnings) != 1 || !strings.Contains(warnings[0], "bad-value") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestBuildDirectMode(t *testing.T) {
	cfg := sampleConfig()
	cfg.Routing.Mode = config.ModeDirect
	cfg.Routing.ProxyDomains = []string{"openai.com"}
	paths := map[string]string{
		"geosite-ru":  `C:\rules\geosite-ru.srs`,
		"geosite-ads": `C:\rules\ads.srs`,
	}
	doc, _ := buildDoc(t, cfg, paths)
	route := doc["route"].(map[string]any)
	if route["final"] != "direct" {
		t.Fatalf("final = %v", route["final"])
	}
	outbounds := doc["outbounds"].([]any)
	if len(outbounds) != 2 {
		t.Fatalf("outbounds = %d", len(outbounds))
	}
	dns := doc["dns"].(map[string]any)
	if dns["final"] != "local" {
		t.Fatalf("dns final = %v", dns["final"])
	}
	for _, rule := range rules(doc) {
		if rule["outbound"] == "auto" {
			t.Fatalf("proxy rule in direct mode: %#v", rule)
		}
	}
}

func TestBuildNoNodes(t *testing.T) {
	cfg := config.Defaults()
	_, _, err := Build(Input{Config: cfg})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestBuildMissingRuleSetsWarns(t *testing.T) {
	cfg := sampleConfig()
	_, warnings := buildDoc(t, cfg, nil)
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "рекламы") || !strings.Contains(joined, "РФ") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestBuildSelectedNodeFallback(t *testing.T) {
	cfg := sampleConfig()
	cfg.SelectedNode = "missing"
	doc, warnings := buildDoc(t, cfg, nil)
	route := doc["route"].(map[string]any)
	if route["final"] != "auto" {
		t.Fatalf("final = %v", route["final"])
	}
	if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, "\n"), "не найдена") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestBuildDisableIPv6(t *testing.T) {
	cfg := sampleConfig()
	cfg.TUN.DisableIPv6 = true
	doc, _ := buildDoc(t, cfg, nil)
	tun := doc["inbounds"].([]any)[0].(map[string]any)
	addresses := tun["address"].([]any)
	if len(addresses) != 1 || addresses[0] != "172.19.0.1/30" {
		t.Fatalf("address = %#v", addresses)
	}
}

func TestBuildInputDisableIPv6(t *testing.T) {
	cfg := sampleConfig()
	data, warnings, err := Build(Input{Config: cfg, DisableIPv6: true, CacheFile: `C:\cache\cache.db`})
	if err != nil {
		t.Fatalf("Build: %v (warnings %v)", err, warnings)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	tun := doc["inbounds"].([]any)[0].(map[string]any)
	if addresses := tun["address"].([]any); len(addresses) != 1 {
		t.Fatalf("address = %#v", addresses)
	}
}

func TestBuildDuplicateTags(t *testing.T) {
	cfg := sampleConfig()
	node := cfg.Subscriptions[0].Nodes[0]
	node.Tag = "node1"
	cfg.Subscriptions[0].Nodes = append(cfg.Subscriptions[0].Nodes, node)
	_, _, err := Build(Input{Config: cfg})
	if err == nil || !strings.Contains(err.Error(), "дублирующийся") {
		t.Fatalf("err = %v", err)
	}
}
