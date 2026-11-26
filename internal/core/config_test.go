package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"

	"qbox/internal/boxconf"
	"qbox/internal/config"
)

func TestGeneratedConfigParses(t *testing.T) {
	cfg := config.Defaults()
	cfg.SelectedNode = "node1"
	cfg.Subscriptions = []config.Subscription{{
		ID:   "sub",
		Name: "Test",
		URL:  "https://example.com/sub",
		Nodes: []config.Node{{
			Tag:      "node1",
			Type:     "vless",
			Server:   "example.com",
			Port:     443,
			Outbound: json.RawMessage(`{"type":"vless","tag":"node1","server":"example.com","server_port":443,"uuid":"u","tls":{"enabled":true,"utls":{"enabled":true,"fingerprint":"chrome"}}}`),
		}},
	}}
	cfg.ActiveSubscription = "sub"
	cfg.Routing.ProxyDomains = []string{"openai.com"}
	cfg.Routing.DirectCIDRs = []string{"10.0.0.0/8"}

	data, warnings, err := boxconf.Build(boxconf.Input{
		Config: cfg,
		RuleSetPaths: map[string]string{
			"geosite-ru":  `C:\rules\geosite-ru.srs`,
			"geoip-ru":    `C:\rules\geoip-ru.srs`,
			"geosite-ads": `C:\rules\ads.srs`,
		},
		ExeName:   "qbox.exe",
		CacheFile: `C:\cache\cache.db`,
	})
	if err != nil {
		t.Fatalf("Build: %v (warnings %v)", err, warnings)
	}

	ctx := include.Context(context.Background())
	if _, err := singjson.UnmarshalExtendedContext[option.Options](ctx, data); err != nil {
		t.Fatalf("sing-box rejected the generated config: %v\n%s", err, data)
	}
}
