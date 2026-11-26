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
	"qbox/internal/jsonutil"
	"qbox/internal/model"
)

const (
	pingTimeout     = 6 * time.Second
	pingWatchdog    = 12 * time.Second
	pingConcurrency = 6
)

var pingTarget = M.ParseSocksaddrHostPort("1.1.1.1", 443)

// Ping measures the latency of every node through its proxy protocol. Each
// node gets a short-lived sing-box instance without TUN or inbounds, so a
// ping works while disconnected and does not disturb the main engine.
//
// Results arrive as each node finishes; the channel closes when all nodes
// have been tested or the context is canceled.
func (e *Engine) Ping(ctx context.Context, nodes []config.Node) <-chan model.PingResult {
	out := make(chan model.PingResult, len(nodes))
	e.opMu.Lock()
	closed := e.closed
	e.opMu.Unlock()
	if closed || len(nodes) == 0 {
		close(out)
		return out
	}

	go func() {
		defer close(out)

		canonCtx := include.Context(context.Background())
		canonical := make([]config.Node, len(nodes))
		canonErrors := make([]error, len(nodes))
		for i := range nodes {
			canonical[i], canonErrors[i] = canonicalizeNode(canonCtx, nodes[i])
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
					if canonErrors[i] != nil {
						out <- model.PingResult{Tag: nodes[i].Tag, Err: canonErrors[i].Error()}
						continue
					}
					result := e.pingWatched(ctx, canonical[i])
					if result.Err != "" && ctx.Err() == nil {
						e.logs.WriteMessage(log.LevelWarn, fmt.Sprintf("пинг %s: %s", result.Tag, result.Err))
					}
					out <- result
				}
			}()
		}
		wg.Wait()
	}()
	return out
}

// pingWatched bounds the whole node test, including creating and starting
// the temporary sing-box instance, so a stuck outbound cannot block the UI.
func (e *Engine) pingWatched(ctx context.Context, node config.Node) model.PingResult {
	result := make(chan model.PingResult, 1)
	go func() {
		result <- e.pingOne(ctx, node, pingTarget)
	}()
	timer := time.NewTimer(pingWatchdog)
	defer timer.Stop()
	select {
	case value := <-result:
		return value
	case <-ctx.Done():
		return model.PingResult{Tag: node.Tag, Err: ctx.Err().Error()}
	case <-timer.C:
		return model.PingResult{Tag: node.Tag, Err: "таймаут"}
	}
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

	boxCtx, cancelBox := context.WithTimeout(ctx, pingWatchdog)
	defer cancelBox()
	boxCtx = include.Context(boxCtx)

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
	if !e.trackTemp(instance) {
		_ = instance.Close()
		result.Err = "движок закрыт"
		return result
	}
	defer e.untrackTemp(instance)

	if err := instance.Start(); err != nil {
		_ = instance.Close()
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
	return jsonutil.SetObjectTag(raw, tag)
}
