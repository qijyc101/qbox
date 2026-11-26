package core

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	M "github.com/sagernet/sing/common/metadata"

	"qbox/internal/config"
	"qbox/internal/model"
)

func TestPingOne(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	address := listener.Addr().(*net.TCPAddr)
	target := M.ParseSocksaddrHostPort("127.0.0.1", uint16(address.Port))
	engine := New(Options{LogFile: t.TempDir() + "/ping.log"})
	defer engine.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result := engine.pingOne(ctx, config.Node{
		Tag:      "node-direct",
		Type:     "direct",
		Server:   "127.0.0.1",
		Port:     address.Port,
		Outbound: json.RawMessage(`{"type":"direct","tag":"node-direct"}`),
	}, target)
	if result.Err != "" {
		t.Fatalf("ping failed: %s", result.Err)
	}
	if result.Latency <= 0 {
		t.Fatalf("latency = %v", result.Latency)
	}
}

func TestPingStreamsResults(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	address := listener.Addr().(*net.TCPAddr)
	previousTarget := pingTarget
	pingTarget = M.ParseSocksaddrHostPort("127.0.0.1", uint16(address.Port))
	defer func() { pingTarget = previousTarget }()

	engine := New(Options{LogFile: t.TempDir() + "/ping.log"})
	defer engine.Close()

	nodes := []config.Node{
		{Tag: "ok", Type: "direct", Outbound: json.RawMessage(`{"type":"direct","tag":"ok"}`)},
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
