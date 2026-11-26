// Package app implements qbox's native UI and the glue to the engine.
package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"qbox/internal/config"
	"qbox/internal/model"
	"qbox/internal/platform"
	"qbox/internal/rulesets"
	"qbox/internal/subscription"
)

// Engine is the part of the core the UI needs.
type Engine interface {
	Start(config.Config, map[string]string) error
	Stop() error
	State() model.State
	LastError() string
	Stats() model.Stats
	Logs() ([]model.LogEntry, uint64)
	ClearLogs()
}

// Fetcher downloads a subscription.
type Fetcher interface {
	Fetch(context.Context, string) ([]byte, error)
}

// RuleSets stores and updates binary rule-sets.
type RuleSets interface {
	Paths() map[string]string
	Status() []rulesets.Status
	Update(context.Context, ...string) error
}

// Deps wires the app to its environment.
type Deps struct {
	Config        config.Config
	Store         *config.Store
	Engine        Engine
	Fetcher       Fetcher
	RuleSets      RuleSets
	ResourcesDir  string
	ConfigPath    string
	CacheDir      string
	ConfigWarning string
	Autostart     bool
	Elevated      func() bool
}

// App is the window state.
type App struct {
	deps Deps
	cfg  config.Config
	win  *mygo.Window

	done   chan struct{}
	closed atomic.Bool
	save   *time.Timer

	page   int
	state  model.State
	stats  model.Stats
	busy   bool
	busyTS string
	notice string
	err    string
	dirty  bool

	logs    []model.LogEntry
	logRev  uint64
	logView int

	nodeList  ui.ListState
	logList   ui.ListState
	nodeIndex int

	adding      bool
	addName     string
	addURL      string
	confirmDel  string
	directText  string
	directCIDRs string
	proxyText   string
	proxyCIDRs  string
}

// New creates the app state.
func New(deps Deps) *App {
	if deps.Elevated == nil {
		deps.Elevated = platform.IsElevated
	}
	cfg := deps.Config.Clone()
	cfg.Normalize()

	a := &App{
		deps:   deps,
		cfg:    cfg,
		done:   make(chan struct{}),
		state:  model.StateStopped,
		logRev: ^uint64(0),
	}
	a.directText = config.JoinList(cfg.Routing.DirectDomains)
	a.directCIDRs = config.JoinList(cfg.Routing.DirectCIDRs)
	a.proxyText = config.JoinList(cfg.Routing.ProxyDomains)
	a.proxyCIDRs = config.JoinList(cfg.Routing.ProxyCIDRs)
	if deps.ConfigWarning != "" {
		a.notice = deps.ConfigWarning
	}
	return a
}

// Open creates the main window and starts polling the engine.
func (a *App) Open() {
	if a.deps.ResourcesDir != "" {
		if err := platform.PreloadWintun(a.deps.ResourcesDir); err != nil {
			a.notice = err.Error()
		}
	}
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:     "qbox",
		Width:     1000,
		Height:    720,
		MinWidth:  760,
		MinHeight: 560,
		StateKey:  "main",
		Content:   ui.View(a.view),
	})
	a.startPolling()
	if a.deps.Autostart && a.deps.Elevated() {
		a.startEngine()
	}
}

// Shutdown stops the engine and saves the configuration.
func (a *App) Shutdown() {
	if a.closed.Swap(true) {
		return
	}
	close(a.done)
	if a.save != nil {
		a.save.Stop()
	}
	a.saveNow()
	_ = a.deps.Engine.Stop()
}

func (a *App) startPolling() {
	ticker := time.NewTicker(600 * time.Millisecond)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-a.done:
				return
			case <-ticker.C:
				state := a.deps.Engine.State()
				stats := a.deps.Engine.Stats()
				logs, rev := a.deps.Engine.Logs()
				lastErr := a.deps.Engine.LastError()
				mygo.RunOnMain(func() {
					if a.closed.Load() {
						return
					}
					changed := state != a.state || stats != a.stats || rev != a.logRev
					if !changed {
						return
					}
					a.state = state
					a.stats = stats
					a.logs = logs
					a.logRev = rev
					if state == model.StateError && lastErr != "" {
						a.err = lastErr
					}
					if a.win != nil {
						a.win.Invalidate()
					}
				})
			}
		}
	}()
}

func (a *App) post(fn func()) {
	if a.closed.Load() {
		return
	}
	if a.win == nil {
		fn()
		return
	}
	a.win.Update(fn)
}

func (a *App) saveNow() {
	if err := a.deps.Store.Save(a.cfg); err != nil {
		a.notice = "Не удалось сохранить настройки: " + err.Error()
	}
}

func (a *App) scheduleSave() {
	if a.win == nil {
		a.saveNow()
		return
	}
	if a.save != nil {
		a.save.Stop()
	}
	a.save = time.AfterFunc(400*time.Millisecond, func() {
		mygo.RunOnMain(func() {
			if !a.closed.Load() {
				a.saveNow()
			}
		})
	})
}

func (a *App) elevated() bool {
	return a.deps.Elevated()
}

func (a *App) start() {
	if a.busy || a.state == model.StateRunning || a.state == model.StateStarting {
		return
	}
	if !a.elevated() {
		a.requestElevation()
		return
	}
	a.startEngine()
}

func (a *App) requestElevation() {
	a.saveNow()
	if err := platform.RelaunchElevated("--elevated", "--autostart"); err != nil {
		a.err = err.Error()
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}
	mygo.App.Quit()
}

func (a *App) startEngine() {
	a.busy = true
	a.busyTS = "Запуск ядра…"
	a.err = ""
	cfg := a.cfg.Clone()
	paths := a.deps.RuleSets.Paths()
	if a.win == nil {
		a.doStart(cfg, paths)
		return
	}
	a.win.Invalidate()
	go a.doStart(cfg, paths)
}

func (a *App) doStart(cfg config.Config, paths map[string]string) {
	err := a.deps.Engine.Start(cfg, paths)

	a.post(func() {
		a.busy = false
		a.busyTS = ""
		a.state = a.deps.Engine.State()
		if err != nil {
			a.err = err.Error()
		} else {
			a.dirty = false
			a.err = ""
			a.notice = ""
		}
		a.refreshLogs()
		if a.win != nil {
			a.win.Invalidate()
		}
	})
}

func (a *App) stop() {
	if a.busy {
		return
	}
	a.busy = true
	a.busyTS = "Остановка…"
	if a.win == nil {
		a.doStop()
		return
	}
	a.win.Invalidate()
	go a.doStop()
}

func (a *App) doStop() {
	err := a.deps.Engine.Stop()
	a.post(func() {
		a.busy = false
		a.busyTS = ""
		a.state = a.deps.Engine.State()
		if err != nil {
			a.err = err.Error()
		}
		a.refreshLogs()
		if a.win != nil {
			a.win.Invalidate()
		}
	})
}

func (a *App) restartNow() {
	if a.busy {
		return
	}
	a.busy = true
	a.busyTS = "Перезапуск ядра…"
	cfg := a.cfg.Clone()
	paths := a.deps.RuleSets.Paths()
	if a.win == nil {
		a.doRestart(cfg, paths)
		return
	}
	a.win.Invalidate()
	go a.doRestart(cfg, paths)
}

func (a *App) doRestart(cfg config.Config, paths map[string]string) {
	_ = a.deps.Engine.Stop()
	err := a.deps.Engine.Start(cfg, paths)
	a.post(func() {
		a.busy = false
		a.busyTS = ""
		a.state = a.deps.Engine.State()
		if err != nil {
			a.err = err.Error()
		} else {
			a.dirty = false
			a.err = ""
			a.notice = ""
		}
		a.refreshLogs()
		if a.win != nil {
			a.win.Invalidate()
		}
	})
}

func (a *App) refreshLogs() {
	logs, rev := a.deps.Engine.Logs()
	a.logs = logs
	a.logRev = rev
}

func (a *App) activeSub() *config.Subscription {
	return a.cfg.Active()
}

func (a *App) addSubscription(name, rawURL string, nodes []config.Node, warnings []string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = subscriptionName(rawURL)
	}
	id := fmt.Sprintf("sub-%d", time.Now().UnixNano())
	sub := config.Subscription{
		ID:        id,
		Name:      name,
		URL:       strings.TrimSpace(rawURL),
		UpdatedAt: time.Now(),
		Nodes:     nodes,
	}
	a.cfg.Subscriptions = append(a.cfg.Subscriptions, sub)
	if a.cfg.ActiveSubscription == "" {
		a.cfg.ActiveSubscription = id
		a.cfg.SelectedNode = "auto"
	}
	a.adding = false
	a.addName = ""
	a.addURL = ""
	a.nodeList = ui.ListState{}
	if len(warnings) > 0 {
		a.notice = fmt.Sprintf("Импортировано %d нод, пропущено %d", len(nodes), len(warnings))
	} else {
		a.notice = fmt.Sprintf("Импортировано %d нод", len(nodes))
	}
	a.saveNow()
}

func (a *App) replaceSubscription(id string, nodes []config.Node, warnings []string) {
	sub := a.cfg.Subscription(id)
	if sub == nil {
		return
	}
	sub.Nodes = nodes
	sub.UpdatedAt = time.Now()
	if len(warnings) > 0 {
		a.notice = fmt.Sprintf("Обновлено: %d нод, пропущено %d", len(nodes), len(warnings))
	} else {
		a.notice = fmt.Sprintf("Обновлено: %d нод", len(nodes))
	}
	if a.cfg.NodeByTag(a.cfg.SelectedNode) == nil {
		a.cfg.SelectedNode = "auto"
	}
	a.nodeList = ui.ListState{}
	if a.state == model.StateRunning {
		a.dirty = true
	}
	a.saveNow()
}

func (a *App) importSubscription() {
	if a.busy {
		return
	}
	rawURL := strings.TrimSpace(a.addURL)
	if rawURL == "" {
		a.err = "Укажите URL подписки"
		return
	}
	name := a.addName

	a.busy = true
	a.busyTS = "Загрузка подписки…"
	if a.win == nil {
		a.doImport(name, rawURL)
		return
	}
	go a.doImport(name, rawURL)
}

func (a *App) doImport(name, rawURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	data, err := a.deps.Fetcher.Fetch(ctx, rawURL)
	if err == nil {
		var nodes []config.Node
		var warnings []string
		nodes, warnings, err = subscription.Parse(data)
		if err == nil {
			a.post(func() {
				a.busy = false
				a.busyTS = ""
				a.err = ""
				a.addSubscription(name, rawURL, nodes, warnings)
				if a.win != nil {
					a.win.Invalidate()
				}
			})
			return
		}
	}
	a.post(func() {
		a.busy = false
		a.busyTS = ""
		a.err = "Импорт не удался: " + err.Error()
		if a.win != nil {
			a.win.Invalidate()
		}
	})
}

func (a *App) refreshSubscription(id string) {
	if a.busy {
		return
	}
	sub := a.cfg.Subscription(id)
	if sub == nil {
		return
	}
	rawURL := sub.URL
	a.busy = true
	a.busyTS = "Обновление подписки…"
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		data, err := a.deps.Fetcher.Fetch(ctx, rawURL)
		if err == nil {
			var nodes []config.Node
			var warnings []string
			nodes, warnings, err = subscription.Parse(data)
			if err == nil {
				a.post(func() {
					a.busy = false
					a.busyTS = ""
					a.err = ""
					a.replaceSubscription(id, nodes, warnings)
					if a.win != nil {
						a.win.Invalidate()
					}
				})
				return
			}
		}
		a.post(func() {
			a.busy = false
			a.busyTS = ""
			a.err = "Обновление не удалось: " + err.Error()
			if a.win != nil {
				a.win.Invalidate()
			}
		})
	}
	if a.win == nil {
		run()
		return
	}
	go run()
}

func (a *App) deleteSubscription(id string) {
	for i := range a.cfg.Subscriptions {
		if a.cfg.Subscriptions[i].ID != id {
			continue
		}
		a.cfg.Subscriptions = append(a.cfg.Subscriptions[:i], a.cfg.Subscriptions[i+1:]...)
		break
	}
	if a.cfg.ActiveSubscription == id {
		a.cfg.ActiveSubscription = ""
		if len(a.cfg.Subscriptions) > 0 {
			a.cfg.ActiveSubscription = a.cfg.Subscriptions[0].ID
		}
		a.cfg.SelectedNode = "auto"
	}
	a.confirmDel = ""
	a.nodeList = ui.ListState{}
	a.saveNow()
	if a.state == model.StateRunning {
		if len(a.cfg.ActiveNodes()) > 0 {
			a.restartNow()
		} else {
			a.stop()
		}
	}
}

func (a *App) selectSubscription(id string) {
	if a.cfg.ActiveSubscription == id {
		return
	}
	a.cfg.ActiveSubscription = id
	a.cfg.SelectedNode = "auto"
	a.nodeList = ui.ListState{}
	a.nodeIndex = 0
	a.saveNow()
	if a.state == model.StateRunning {
		a.restartNow()
	}
}

func (a *App) selectNode(index int) {
	rows := a.nodeRows()
	if index < 0 || index >= len(rows) {
		return
	}
	tag := rows[index].tag
	if tag == a.cfg.SelectedNode {
		return
	}
	a.cfg.SelectedNode = tag
	a.saveNow()
	if a.state == model.StateRunning {
		a.restartNow()
	}
}

type nodeRow struct {
	tag    string
	title  string
	detail string
	auto   bool
}

func (a *App) nodeRows() []nodeRow {
	sub := a.activeSub()
	rows := make([]nodeRow, 0, 1)
	rows = append(rows, nodeRow{
		tag:    "auto",
		title:  "Авто",
		detail: "Выбор ноды по задержке (urltest)",
		auto:   true,
	})
	if sub == nil {
		return rows
	}
	for _, node := range sub.Nodes {
		title := node.Tag
		detail := node.Type
		if node.Server != "" {
			detail += " · " + node.Server
			if node.Port > 0 {
				detail += fmt.Sprintf(":%d", node.Port)
			}
		}
		rows = append(rows, nodeRow{tag: node.Tag, title: title, detail: detail})
	}
	return rows
}

func (a *App) syncNodeIndex() {
	rows := a.nodeRows()
	a.nodeIndex = 0
	for i, row := range rows {
		if row.tag == a.cfg.SelectedNode {
			a.nodeIndex = i
			return
		}
	}
}

func (a *App) routingChanged() {
	a.dirty = true
	a.scheduleSave()
}

func (a *App) updateRuleSet(tag string) {
	if a.busy {
		return
	}
	a.busy = true
	a.busyTS = "Скачивание баз…"
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		err := a.deps.RuleSets.Update(ctx, tag)
		a.post(func() {
			a.busy = false
			a.busyTS = ""
			if err != nil {
				a.err = "Базы: " + err.Error()
			} else {
				a.notice = "Базы обновлены. Перезапустите подключение, чтобы применить правила."
				if a.state == model.StateRunning {
					a.dirty = true
				}
			}
			if a.win != nil {
				a.win.Invalidate()
			}
		})
	}
	if a.win == nil {
		run()
		return
	}
	go run()
}

func (a *App) updateAllRuleSets() {
	if a.busy {
		return
	}
	a.busy = true
	a.busyTS = "Скачивание баз…"
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		err := a.deps.RuleSets.Update(ctx)
		a.post(func() {
			a.busy = false
			a.busyTS = ""
			if err != nil {
				a.err = "Базы: " + err.Error()
			} else {
				a.notice = "Базы обновлены. Перезапустите подключение, чтобы применить правила."
				if a.state == model.StateRunning {
					a.dirty = true
				}
			}
			if a.win != nil {
				a.win.Invalidate()
			}
		})
	}
	if a.win == nil {
		run()
		return
	}
	go run()
}

func (a *App) clearLogs() {
	a.deps.Engine.ClearLogs()
	a.refreshLogs()
}

func (a *App) openLogFolder() {
	dir, err := config.LogDir()
	if err != nil {
		a.err = err.Error()
		return
	}
	_ = os.MkdirAll(dir, 0o700)
	if err := mygo.Shell.OpenPath(dir); err != nil {
		a.err = "Не удалось открыть папку логов: " + err.Error()
	}
}

func subscriptionName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "Подписка"
	}
	return u.Host
}

// Accessors used by the view.

func (a *App) stateText() string {
	switch a.state {
	case model.StateStarting:
		return "Запуск…"
	case model.StateRunning:
		return "Подключено"
	case model.StateStopping:
		return "Остановка…"
	case model.StateError:
		return "Ошибка"
	default:
		return "Отключено"
	}
}

func (a *App) modeIndex() int {
	switch a.cfg.Routing.Mode {
	case config.ModeGlobal:
		return 0
	case config.ModeDirect:
		return 2
	default:
		return 1
	}
}

func (a *App) setMode(index int) {
	switch index {
	case 0:
		a.cfg.Routing.Mode = config.ModeGlobal
	case 2:
		a.cfg.Routing.Mode = config.ModeDirect
	default:
		a.cfg.Routing.Mode = config.ModeRules
	}
	a.routingChanged()
}

func (a *App) stackIndex() int {
	switch a.cfg.TUN.Stack {
	case config.StackSystem:
		return 0
	case config.StackGVisor:
		return 2
	default:
		return 1
	}
}

func (a *App) setStack(index int) {
	switch index {
	case 0:
		a.cfg.TUN.Stack = config.StackSystem
	case 2:
		a.cfg.TUN.Stack = config.StackGVisor
	default:
		a.cfg.TUN.Stack = config.StackMixed
	}
	a.routingChanged()
}

func (a *App) logLevelIndex() int {
	switch a.cfg.LogLevel {
	case "debug":
		return 1
	case "trace":
		return 2
	default:
		return 0
	}
}

func (a *App) setLogLevel(index int) {
	switch index {
	case 1:
		a.cfg.LogLevel = "debug"
	case 2:
		a.cfg.LogLevel = "trace"
	default:
		a.cfg.LogLevel = "info"
	}
	a.routingChanged()
}

func (a *App) logLines() []model.LogEntry {
	want := ""
	switch a.logView {
	case 1:
		want = "info"
	case 2:
		want = "warn"
	case 3:
		want = "error"
	}
	if want == "" {
		return a.logs
	}
	out := make([]model.LogEntry, 0, len(a.logs))
	for _, entry := range a.logs {
		switch want {
		case "info":
			if entry.Level == "info" || entry.Level == "warn" || entry.Level == "error" {
				out = append(out, entry)
			}
		case "warn":
			if entry.Level == "warn" || entry.Level == "error" {
				out = append(out, entry)
			}
		case "error":
			if entry.Level == "error" || entry.Level == "fatal" || entry.Level == "panic" {
				out = append(out, entry)
			}
		}
	}
	return out
}
