package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	"golang.org/x/net/proxy"

	"qbox/internal/boxconf"
	"qbox/internal/config"
)

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func TestRoutingRulesSelectOutbound(t *testing.T) {
	proxyPort := freePort(t)
	proxyConnections := make(chan string, 4)
	proxyListener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort))
	if err != nil {
		t.Fatal(err)
	}
	defer proxyListener.Close()
	go func() {
		for {
			conn, err := proxyListener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				reader := bufio.NewReader(conn)
				request, err := http.ReadRequest(reader)
				if err != nil {
					return
				}
				host := request.Host
				if host == "" {
					host = request.RequestURI
				}
				proxyConnections <- host
				_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer target.Close()
	targetAddr := target.Listener.Addr().String()
	_, targetPort, err := net.SplitHostPort(targetAddr)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Defaults()
	cfg.Routing.Mode = config.ModeRules
	cfg.Routing.BypassLAN = false
	cfg.Routing.BypassRU = false
	cfg.Routing.BlockAds = false
	cfg.Routing.ProxyDomains = []string{"proxied.invalid"}
	cfg.Routing.DirectDomains = []string{"localhost"}
	cfg.DNS.UseProxy = false
	cfg.SelectedNode = "proxy"
	cfg.Subscriptions = []config.Subscription{{
		ID:   "sub",
		Name: "test",
		URL:  "https://example.com",
		Nodes: []config.Node{{
			Tag:      "proxy",
			Type:     "http",
			Server:   "127.0.0.1",
			Port:     proxyPort,
			Outbound: json.RawMessage(fmt.Sprintf(`{"type":"http","tag":"proxy","server":"127.0.0.1","server_port":%d}`, proxyPort)),
		}},
	}}
	cfg.ActiveSubscription = "sub"

	cache := t.TempDir() + "/cache.db"
	data, warnings, err := boxconf.Build(boxconf.Input{
		Config:    cfg,
		ExeName:   "route-test.exe",
		CacheFile: cache,
	})
	if err != nil {
		t.Fatalf("Build: %v (warnings %v)", err, warnings)
	}

	inboundPort := freePort(t)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc["inbounds"] = []any{map[string]any{
		"type":        "mixed",
		"tag":         "in",
		"listen":      "127.0.0.1",
		"listen_port": inboundPort,
	}}
	data, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	ctx := include.Context(context.Background())
	opts, err := singjson.UnmarshalExtendedContext[option.Options](ctx, data)
	if err != nil {
		t.Fatalf("sing-box rejected config: %v", err)
	}
	instance, err := box.New(box.Options{Context: ctx, Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Start(); err != nil {
		_ = instance.Close()
		t.Fatal(err)
	}
	defer instance.Close()

	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", inboundPort), nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}

	// Direct rule: localhost goes straight to the target HTTP server.
	directConn, err := dialer.Dial("tcp", "localhost:"+targetPort)
	if err != nil {
		t.Fatalf("direct dial: %v", err)
	}
	_ = directConn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := fmt.Fprintf(directConn, "GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(directConn), nil)
	if err != nil {
		t.Fatalf("direct response: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	_ = directConn.Close()
	if response.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("direct response = %d %q", response.StatusCode, body)
	}

	// Proxy rule: the HTTP outbound receives CONNECT for the domain. The
	// auto group may probe www.gstatic.com first, so wait for our host.
	proxyConn, err := dialer.Dial("tcp", "proxied.invalid:80")
	if err != nil {
		t.Fatalf("proxy dial: %v", err)
	}
	_ = proxyConn.Close()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case host := <-proxyConnections:
			if strings.HasPrefix(host, "proxied.invalid:80") {
				return
			}
		case <-deadline:
			t.Fatal("proxy outbound was not used for proxied.invalid")
		}
	}
}
