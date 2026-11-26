package subscription

import "testing"

func FuzzParse(f *testing.F) {
	seeds := []string{
		"vless://u@example.com:443?security=tls#node",
		"vmess://eyJ2IjoiMiJ9",
		"ss://YWVzLTI1Ni1nY206cGFzcw==@1.2.3.4:8388#ss",
		"hysteria2://pass@example.com:443?sni=example.com#h2",
		"tuic://uuid:pass@example.com:443#tuic",
		`{"outbounds":[{"type":"vless","tag":"a","server":"example.com","server_port":443,"uuid":"u"}]}`,
		"proxies:\n  - name: x",
		"",
		"not a subscription",
		"vless://u@example.com:99999#bad",
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		_, _, _ = Parse(data)
	})
}
