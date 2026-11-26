package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	M "github.com/sagernet/sing/common/metadata"

	"qbox/internal/config"
	"qbox/internal/model"
)

func fakeHTTPProxy(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				request, err := http.ReadRequest(bufio.NewReader(conn))
				if err != nil {
					return
				}
				_ = request
				_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port
}

func httpNode(t *testing.T, tag string, port int) config.Node {
	t.Helper()
	return config.Node{
		Tag:      tag,
		Type:     "http",
		Server:   "127.0.0.1",
		Port:     port,
		Outbound: json.RawMessage(fmt.Sprintf(`{"type":"http","tag":%q,"server":"127.0.0.1","server_port":%d}`, tag, port)),
	}
}

func TestPingOne(t *testing.T) {
	proxyPort := fakeHTTPProxy(t)
	target := M.ParseSocksaddrHostPort("example.com", 443)
	engine := New(Options{LogFile: t.TempDir() + "/ping.log"})
	defer engine.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result := engine.pingOne(ctx, httpNode(t, "node-http", proxyPort), target)
	if result.Err != "" {
		t.Fatalf("ping failed: %s", result.Err)
	}
	if result.Latency > time.Second {
		t.Fatalf("latency = %v", result.Latency)
	}
}

func TestPingStreamsResults(t *testing.T) {
	proxyPort := fakeHTTPProxy(t)
	previousTarget := pingTarget
	pingTarget = M.ParseSocksaddrHostPort("example.com", 443)
	defer func() { pingTarget = previousTarget }()

	engine := New(Options{LogFile: t.TempDir() + "/ping.log"})
	defer engine.Close()

	nodes := []config.Node{
		httpNode(t, "ok", proxyPort),
		{Tag: "bad", Type: "nope", Outbound: json.RawMessage(`{"type":"nope","tag":"bad"}`)},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	results := make(map[string]model.PingResult)
	for result := range engine.Ping(ctx, nodes) {
		results[result.Tag] = result
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v", results)
	}
	if results["ok"].Err != "" {
		t.Fatalf("ok node failed: %s", results["ok"].Err)
	}
	if results["bad"].Err == "" {
		t.Fatal("bad node did not fail")
	}
}
