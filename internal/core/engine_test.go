package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

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

func TestEngineSystemProxyMode(t *testing.T) {
	cfg := config.Defaults()
	cfg.TUN.Enabled = false
	cfg.Routing.Mode = config.ModeDirect
	cfg.Proxy.Port = 0
	cfg.LogLevel = "warn"

	engine := New(Options{
		CacheFile: filepath.Join(t.TempDir(), "cache.db"),
		LogFile:   filepath.Join(t.TempDir(), "qbox.log"),
	})
	defer engine.Close()
	if err := engine.Start(cfg, nil); err != nil {
		t.Fatal(err)
	}
	addr := engine.ProxyAddress()
	if addr == "" {
		t.Fatal("no proxy address")
	}
	if !bytes.Contains(engine.LastConfig(), []byte(`"type": "mixed"`)) {
		t.Fatalf("last config does not contain the mixed inbound: %s", engine.LastConfig())
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("through-qbox"))
	}))
	defer server.Close()

	proxyURL, err := url.Parse("http://" + addr)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   5 * time.Second,
	}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "through-qbox" {
		t.Fatalf("body = %q", body)
	}
	if engine.Connections() == nil {
		// Connections may be empty after the request completes; the call
		// itself must not panic.
		t.Log("no connections after the request")
	}
	if err := engine.Stop(); err != nil {
		t.Fatal(err)
	}
	if engine.ProxyAddress() != "" {
		t.Fatal("proxy address survived stop")
	}
}

func TestFreeProxyPortRejectsBusyPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if _, _, err := freeProxyPort(port); err == nil {
		t.Fatal("a busy port was accepted")
	}
	got, addr, err := freeProxyPort(0)
	if err != nil {
		t.Fatal(err)
	}
	if got == 0 || addr == "" {
		t.Fatalf("free port = %d %q", got, addr)
	}
}
