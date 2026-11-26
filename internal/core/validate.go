package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"

	"qbox/internal/config"
)

var allowedOutboundTypes = map[string]struct{}{
	"vless":       {},
	"vmess":       {},
	"trojan":      {},
	"shadowsocks": {},
	"hysteria2":   {},
	"tuic":        {},
	"socks":       {},
	"http":        {},
}

// canonicalizeNodes decodes every active node with sing-box's typed option
// decoder and re-marshals it. This rejects unknown types (notably tor/ssh),
// case-folded keys that bypass text sanitizers, and dangerous fields such as
// Shadowsocks plugins that execute local binaries.
func canonicalizeNodes(cfg *config.Config) error {
	if cfg.Routing.Mode == config.ModeDirect {
		return nil
	}
	sub := cfg.Active()
	if sub == nil {
		return nil
	}
	ctx := include.Context(context.Background())
	seen := make(map[string]struct{}, len(sub.Nodes))
	for i := range sub.Nodes {
		node, err := canonicalizeNode(ctx, sub.Nodes[i])
		if err != nil {
			return fmt.Errorf("нода %q: %w", sub.Nodes[i].Tag, err)
		}
		if _, exists := seen[node.Tag]; exists {
			return fmt.Errorf("дублирующийся тег ноды %q", node.Tag)
		}
		seen[node.Tag] = struct{}{}
		sub.Nodes[i] = node
	}
	return nil
}

func canonicalizeNode(ctx context.Context, node config.Node) (config.Node, error) {
	parsed, err := singjson.UnmarshalExtendedContext[option.Outbound](ctx, node.Outbound)
	if err != nil {
		return config.Node{}, err
	}
	if _, ok := allowedOutboundTypes[parsed.Type]; !ok {
		return config.Node{}, fmt.Errorf("тип outbound %q запрещён", parsed.Type)
	}
	if node.Type != "" && parsed.Type != node.Type {
		return config.Node{}, fmt.Errorf("тип outbound %q не совпадает с %q", parsed.Type, node.Type)
	}
	if node.Tag == "" {
		return config.Node{}, errors.New("пустой тег ноды")
	}
	parsed.Tag = node.Tag

	if options, ok := parsed.Options.(*option.ShadowsocksOutboundOptions); ok && options.Plugin != "" {
		return config.Node{}, errors.New("плагин Shadowsocks запрещён")
	}
	if wrapper, ok := parsed.Options.(option.OutboundTLSOptionsWrapper); ok {
		if tls := wrapper.TakeOutboundTLSOptions(); tls != nil {
			switch {
			case tls.CertificatePath != "":
				return config.Node{}, errors.New("tls.certificate_path запрещён")
			case tls.ClientCertificatePath != "":
				return config.Node{}, errors.New("tls.client_certificate_path запрещён")
			case tls.ClientKeyPath != "":
				return config.Node{}, errors.New("tls.client_key_path запрещён")
			case tls.ECH != nil && tls.ECH.ConfigPath != "":
				return config.Node{}, errors.New("tls.ech.config_path запрещён")
			}
		}
	}

	raw, err := parsed.MarshalJSONContext(ctx)
	if err != nil {
		return config.Node{}, err
	}
	node.Type = parsed.Type
	node.Outbound = raw
	return node, nil
}
