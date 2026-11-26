package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	M "github.com/sagernet/sing/common/metadata"

	"qbox/internal/config"
	"qbox/internal/model"
)

const (
	pingTimeout     = 5 * time.Second
	pingConcurrency = 4
)

var pingTarget = M.ParseSocksaddrHostPort("www.gstatic.com", 443)

// Ping measures the latency of every node through its proxy protocol. Each
// node gets a short-lived sing-box instance without TUN or inbounds, so a
// ping works while disconnected and does not disturb the main engine.
func (e *Engine) Ping(ctx context.Context, nodes []config.Node) []model.PingResult {
	results := make([]model.PingResult, len(nodes))
	if len(nodes) == 0 {
		return results
	}

	jobs := make(chan int, len(nodes))
	for i := range nodes {
		jobs <- i
	}
	close(jobs)

	workers := pingConcurrency
	if workers > len(nodes) {
		workers = len(nodes)
	}
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = e.pingOne(ctx, nodes[i], pingTarget)
			}
		}()
	}
	wg.Wait()

	for i := range results {
		if results[i].Tag == "" {
			results[i] = model.PingResult{Tag: nodes[i].Tag, Err: "отменено"}
		}
		if results[i].Err != "" {
			e.logs.WriteMessage(log.LevelWarn, fmt.Sprintf("пинг %s: %s", results[i].Tag, results[i].Err))
		}
	}
	return results
}

func (e *Engine) pingOne(ctx context.Context, node config.Node, target M.Socksaddr) model.PingResult {
	result := model.PingResult{Tag: node.Tag}
	if err := ctx.Err(); err != nil {
		result.Err = err.Error()
		return result
	}

	raw, err := withRawTag(node.Outbound, node.Tag)
	if err != nil {
		result.Err = err.Error()
		return result
	}
	data, err := json.Marshal(map[string]any{
		"log": map[string]any{"disabled": true},
		"dns": map[string]any{
			"servers": []any{map[string]any{"type": "local", "tag": "local"}},
			"final":   "local",
		},
		"outbounds": []json.RawMessage{raw, json.RawMessage(`{"type":"direct","tag":"direct"}`)},
		"route": map[string]any{
			"final":                   node.Tag,
			"auto_detect_interface":   true,
			"default_domain_resolver": map[string]any{"server": "local"},
		},
	})
	if err != nil {
		result.Err = err.Error()
		return result
	}

	boxCtx := include.Context(context.Background())
	boxCtx, cancelBox := context.WithCancel(boxCtx)
	defer cancelBox()

	opts, err := singjson.UnmarshalExtendedContext[option.Options](boxCtx, data)
	if err != nil {
		result.Err = err.Error()
		return result
	}
	instance, err := box.New(box.Options{Context: boxCtx, Options: opts})
	if err != nil {
		result.Err = err.Error()
		return result
	}
	if err := instance.Start(); err != nil {
		result.Err = err.Error()
		return result
	}
	defer instance.Close()

	outbound, loaded := instance.Outbound().Outbound(node.Tag)
	if !loaded {
		result.Err = "outbound не найден"
		return result
	}

	dialCtx, cancelDial := context.WithTimeout(ctx, pingTimeout)
	defer cancelDial()
	started := time.Now()
	conn, err := outbound.DialContext(dialCtx, "tcp", target)
	if err != nil {
		result.Err = err.Error()
		return result
	}
	result.Latency = time.Since(started)
	_ = conn.Close()
	return result
}

func withRawTag(raw json.RawMessage, tag string) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(tag)
	if err != nil {
		return nil, err
	}
	obj["tag"] = encoded
	return json.Marshal(obj)
}
