package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"testing"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/common/trafficcontrol"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"

	"qbox/internal/config"
)

func TestBoxStartsAndStops(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	ctx := include.Context(context.Background())
	cache := filepath.Join(t.TempDir(), "cache.db")
	configJSON := fmt.Sprintf(`{
		"log": {"level": "info", "timestamp": true},
		"inbounds": [{"type": "mixed", "tag": "in", "listen": "127.0.0.1", "listen_port": %d}],
		"outbounds": [{"type": "direct", "tag": "direct"}],
		"route": {"final": "direct"},
		"experimental": {"cache_file": {"enabled": true, "path": %q}}
	}`, port, cache)

	opts, err := singjson.UnmarshalExtendedContext[option.Options](ctx, []byte(configJSON))
	if err != nil {
		t.Fatal(err)
	}
	sink := NewLogSink(filepath.Join(t.TempDir(), "qbox.log"))
	defer sink.Close()

	instance, err := box.New(box.Options{
		Context:           ctx,
		Options:           opts,
		PlatformLogWriter: sink,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Start(); err != nil {
		_ = instance.Close()
		t.Fatal(err)
	}
	traffic := service.PtrFromContext[trafficcontrol.Manager](ctx)
	if traffic == nil {
		t.Error("traffic manager is nil")
	}
	if err := instance.Close(); err != nil {
		t.Fatal(err)
	}
	logs, _ := sink.Snapshot()
	if len(logs) == 0 {
		t.Error("no logs were captured")
	}
}

func TestEngineClosedIsTerminal(t *testing.T) {
	engine := New(Options{LogFile: t.TempDir() + "/closed.log"})
	engine.Close()
	if err := engine.Start(config.Defaults(), nil); err == nil {
		t.Fatal("Start succeeded after Close")
	}
	if err := engine.Restart(config.Defaults(), nil); err == nil {
		t.Fatal("Restart succeeded after Close")
	}
}

func TestIsIPv6ConfigError(t *testing.T) {
	if !isIPv6ConfigError(errors.New("start inbound/tun[tun-in]: configure tun interface: set ipv6 address: Element not found.")) {
		t.Fatal("ipv6 address error was not recognized")
	}
	if !isIPv6ConfigError(errors.New("set ipv6 dns: Element not found")) {
		t.Fatal("ipv6 dns error was not recognized")
	}
	if !isIPv6ConfigError(errors.New("Element not found")) {
		t.Fatal("element-not-found error was not recognized")
	}
	if isIPv6ConfigError(errors.New("permission denied")) {
		t.Fatal("unrelated error was recognized as IPv6")
	}
}
