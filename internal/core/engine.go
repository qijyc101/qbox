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

	rootCtx    context.Context
	rootCancel context.CancelFunc

	opMu   sync.Mutex // serializes Start, Stop, Restart and Close
	closed bool

	mu       sync.Mutex
	state    model.State
	lastErr  string
	lastWarn string
	instance *box.Box
	cancel   context.CancelFunc
	traffic  *trafficcontrol.Manager

	tempMu     sync.Mutex
	tempBoxes  map[*box.Box]struct{}
	tempClosed bool
}

// New creates an engine. Call Start to run it.
func New(opts Options) *Engine {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	return &Engine{
		opts:       opts,
		logs:       NewLogSink(opts.LogFile),
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
		state:      model.StateStopped,
		tempBoxes:  make(map[*box.Box]struct{}),
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
	if e.closed {
		return errors.New("движок закрыт")
	}
	return e.startLocked(cfg, ruleSetPaths)
}

func (e *Engine) startLocked(cfg config.Config, ruleSetPaths map[string]string) error {
	cfg = cfg.Clone()
	if err := canonicalizeNodes(&cfg); err != nil {
		e.setState(model.StateError, err)
		return err
	}

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
	collectedWarnings := append([]string(nil), warnings...)

	instance, cancel, traffic, err := e.launch(data)
	if err != nil && !cfg.TUN.DisableIPv6 && isIPv6ConfigError(err) {
		message := "Windows не смогла назначить IPv6-адрес TUN; повтор запуска без IPv6"
		e.logs.WriteMessage(log.LevelWarn, message)
		buildInput.DisableIPv6 = true
		data, warnings, buildErr := boxconf.Build(buildInput)
		e.logWarnings(warnings)
		if buildErr == nil {
			collectedWarnings = append([]string(nil), warnings...)
			instance, cancel, traffic, err = e.launch(data)
			if err == nil {
				collectedWarnings = append(collectedWarnings,
					"TUN работает только по IPv4. strict_route блокирует IPv6-утечки; проверьте настройки IPv6.")
			}
		} else {
			e.logs.WriteMessage(log.LevelError, "повтор без IPv6: "+buildErr.Error())
		}
	}
	if err != nil {
		e.setState(model.StateError, err)
		return err
	}
	e.setWarning(strings.Join(collectedWarnings, "; "))

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
	ctx := include.Context(e.rootCtx)
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
		_ = instance.Close()
		cancel()
		return nil, nil, nil, fmt.Errorf("запуск sing-box: %w", err)
	}
	traffic := service.PtrFromContext[trafficcontrol.Manager](ctx)
	return instance, cancel, traffic, nil
}

func isIPv6ConfigError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "ipv6") ||
		strings.Contains(message, "element not found")
}

// Stop stops the running sing-box instance.
func (e *Engine) Stop() error {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	return e.stopLocked()
}

// Restart stops the current instance and starts a new one while holding the
// operation lock, so a quit cannot slip into the gap between them.
func (e *Engine) Restart(cfg config.Config, ruleSetPaths map[string]string) error {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if e.closed {
		return errors.New("движок закрыт")
	}
	if err := e.stopLocked(); err != nil {
		return err
	}
	return e.startLocked(cfg, ruleSetPaths)
}

func (e *Engine) stopLocked() error {
	e.mu.Lock()
	instance := e.instance
	cancel := e.cancel
	if instance == nil {
		if e.state != model.StateError {
			e.state = model.StateStopped
		}
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

	if err != nil {
		// Keep the instance so a later Stop can retry the cleanup instead of
		// dropping the only reference to a half-closed TUN.
		e.mu.Lock()
		e.instance = instance
		e.cancel = nil
		e.traffic = nil
		e.state = model.StateError
		e.lastErr = fmt.Sprintf("остановка sing-box: %v", err)
		e.mu.Unlock()
		return fmt.Errorf("остановка sing-box: %w", err)
	}

	e.mu.Lock()
	e.instance = nil
	e.cancel = nil
	e.traffic = nil
	e.state = model.StateStopped
	e.mu.Unlock()

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

// LogRevision returns the log revision without copying entries.
func (e *Engine) LogRevision() uint64 {
	return e.logs.Revision()
}

// ClearLogs empties the UI log buffer.
func (e *Engine) ClearLogs() {
	e.logs.Clear()
}

// Close stops the engine and closes its log file. The engine cannot be
// started again afterwards.
func (e *Engine) Close() {
	// Cancel any in-flight Start before waiting for the operation lock, so a
	// stuck startup cannot block quitting.
	e.rootCancel()
	e.opMu.Lock()
	if e.closed {
		e.opMu.Unlock()
		return
	}
	e.closed = true
	err := e.stopLocked()
	e.opMu.Unlock()
	if err != nil {
		e.logs.WriteMessage(log.LevelError, err.Error())
	}

	e.tempMu.Lock()
	e.tempClosed = true
	tempBoxes := make([]*box.Box, 0, len(e.tempBoxes))
	for instance := range e.tempBoxes {
		tempBoxes = append(tempBoxes, instance)
	}
	e.tempBoxes = make(map[*box.Box]struct{})
	e.tempMu.Unlock()
	for _, instance := range tempBoxes {
		_ = instance.Close()
	}

	e.logs.Close()
}

// trackTemp registers a short-lived ping instance so Close can stop it. It
// returns false when the engine is already closed; the caller must then
// close the instance itself without starting it.
func (e *Engine) trackTemp(instance *box.Box) bool {
	e.tempMu.Lock()
	defer e.tempMu.Unlock()
	if e.tempClosed {
		return false
	}
	e.tempBoxes[instance] = struct{}{}
	return true
}

func (e *Engine) untrackTemp(instance *box.Box) {
	e.tempMu.Lock()
	delete(e.tempBoxes, instance)
	e.tempMu.Unlock()
}
