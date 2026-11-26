// Package core runs sing-box in-process.
package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/common/trafficcontrol"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	singjson "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"

	"qbox/internal/boxconf"
	"qbox/internal/config"
	"qbox/internal/model"
)

// Options configures an Engine.
type Options struct {
	CacheFile string
	LogFile   string
}

// Engine owns one sing-box instance.
type Engine struct {
	opts Options
	logs *LogSink

	opMu sync.Mutex // serializes Start and Stop

	mu       sync.Mutex
	state    model.State
	lastErr  string
	lastWarn string
	instance *box.Box
	cancel   context.CancelFunc
	traffic  *trafficcontrol.Manager
}

// New creates an engine. Call Start to run it.
func New(opts Options) *Engine {
	return &Engine{
		opts:  opts,
		logs:  NewLogSink(opts.LogFile),
		state: model.StateStopped,
	}
}

// Logs returns the log sink, mostly for tests.
func (e *Engine) LogSink() *LogSink { return e.logs }

// State returns the current lifecycle state.
func (e *Engine) State() model.State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state
}

// LastError returns the last startup error, if any.
func (e *Engine) LastError() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastErr
}

// Warning returns a non-fatal warning from the last start, such as the
// automatic IPv4-only fallback.
func (e *Engine) Warning() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastWarn
}

func (e *Engine) setWarning(message string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastWarn = message
}

func (e *Engine) setState(state model.State, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.state = state
	if err != nil {
		e.lastErr = err.Error()
	} else if state == model.StateRunning || state == model.StateStopped {
		e.lastErr = ""
	}
}

// Start builds a sing-box configuration and starts it.
func (e *Engine) Start(cfg config.Config, ruleSetPaths map[string]string) error {
	e.opMu.Lock()
	defer e.opMu.Unlock()

	e.mu.Lock()
	running := e.instance != nil
	e.mu.Unlock()
	if running {
		return errors.New("ядро уже запущено")
	}

	e.setState(model.StateStarting, nil)
	e.setWarning("")

	if e.opts.CacheFile != "" {
		if err := os.MkdirAll(filepath.Dir(e.opts.CacheFile), 0o700); err != nil {
			e.setState(model.StateError, err)
			return fmt.Errorf("каталог кэша: %w", err)
		}
	}

	exe, _ := os.Executable()
	buildInput := boxconf.Input{
		Config:       cfg,
		RuleSetPaths: ruleSetPaths,
		ExeName:      filepath.Base(exe),
		CacheFile:    e.opts.CacheFile,
	}
	data, warnings, err := boxconf.Build(buildInput)
	e.logWarnings(warnings)
	if err != nil {
		e.setState(model.StateError, err)
		return err
	}

	instance, cancel, traffic, err := e.launch(data)
	if err != nil && !cfg.TUN.DisableIPv6 && isIPv6ConfigError(err) {
		message := "Windows не смогла назначить IPv6-адрес TUN; повтор запуска без IPv6"
		e.logs.WriteMessage(log.LevelWarn, message)
		buildInput.DisableIPv6 = true
		data, warnings, buildErr := boxconf.Build(buildInput)
		e.logWarnings(warnings)
		if buildErr == nil {
			instance, cancel, traffic, err = e.launch(data)
			if err == nil {
				e.setWarning("TUN работает только по IPv4: Windows не может назначить IPv6-адрес. Проверьте утечки IPv6 или включите IPv6 в системе.")
			}
		}
	}
	if err != nil {
		e.setState(model.StateError, err)
		return err
	}

	if traffic == nil {
		e.logs.WriteMessage(log.LevelWarn, "статистика трафика недоступна")
	}

	e.mu.Lock()
	e.instance = instance
	e.cancel = cancel
	e.traffic = traffic
	e.state = model.StateRunning
	e.lastErr = ""
	e.mu.Unlock()

	e.logs.WriteMessage(log.LevelInfo, "qbox: подключение запущено")
	return nil
}

func (e *Engine) logWarnings(warnings []string) {
	for _, warning := range warnings {
		e.logs.WriteMessage(log.LevelWarn, warning)
	}
}

// launch parses a generated configuration and starts a sing-box instance.
// On failure the instance closes itself and the context is cancelled.
func (e *Engine) launch(data []byte) (*box.Box, context.CancelFunc, *trafficcontrol.Manager, error) {
	ctx := include.Context(context.Background())
	ctx, cancel := context.WithCancel(ctx)

	opts, err := singjson.UnmarshalExtendedContext[option.Options](ctx, data)
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("конфигурация sing-box: %w", err)
	}
	instance, err := box.New(box.Options{
		Context:           ctx,
		Options:           opts,
		PlatformLogWriter: e.logs,
	})
	if err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("создание sing-box: %w", err)
	}
	if err := instance.Start(); err != nil {
		cancel()
		return nil, nil, nil, fmt.Errorf("запуск sing-box: %w", err)
	}
	traffic := service.PtrFromContext[trafficcontrol.Manager](ctx)
	return instance, cancel, traffic, nil
}

func isIPv6ConfigError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "set ipv6 address") ||
		strings.Contains(message, "set ipv6 dns") ||
		strings.Contains(message, "set ipv6 options")
}

// Stop stops the running sing-box instance.
func (e *Engine) Stop() error {
	e.opMu.Lock()
	defer e.opMu.Unlock()

	e.mu.Lock()
	instance := e.instance
	cancel := e.cancel
	if instance == nil {
		e.state = model.StateStopped
		e.mu.Unlock()
		return nil
	}
	e.state = model.StateStopping
	e.mu.Unlock()

	e.logs.WriteMessage(log.LevelInfo, "qbox: остановка подключения")
	err := instance.Close()
	if cancel != nil {
		cancel()
	}

	e.mu.Lock()
	e.instance = nil
	e.cancel = nil
	e.traffic = nil
	e.state = model.StateStopped
	e.mu.Unlock()

	if err != nil {
		err = fmt.Errorf("остановка sing-box: %w", err)
		e.setState(model.StateError, err)
		return err
	}
	e.logs.WriteMessage(log.LevelInfo, "qbox: подключение остановлено")
	return nil
}

// Stats returns the traffic counters of the running instance.
func (e *Engine) Stats() model.Stats {
	e.mu.Lock()
	traffic := e.traffic
	running := e.instance != nil
	e.mu.Unlock()
	if !running || traffic == nil {
		return model.Stats{}
	}
	up, down := traffic.Total()
	return model.Stats{
		Up:          up,
		Down:        down,
		Connections: traffic.ConnectionsLen(),
	}
}

// Logs returns buffered log entries and their revision.
func (e *Engine) Logs() ([]model.LogEntry, uint64) {
	return e.logs.Snapshot()
}

// ClearLogs empties the UI log buffer.
func (e *Engine) ClearLogs() {
	e.logs.Clear()
}

// Close stops the engine and closes its log file.
func (e *Engine) Close() {
	_ = e.Stop()
	e.logs.Close()
}
