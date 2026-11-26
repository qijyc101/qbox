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
	Restart(config.Config, map[string]string) error
	Stop() error
	State() model.State
	LastError() string
	Warning() string
	Stats() model.Stats
	Logs() ([]model.LogEntry, uint64)
	LogRevision() uint64
	ClearLogs()
	Ping(context.Context, []config.Node) <-chan model.PingResult
	Close()
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

	page     int
	state    model.State
	stats    model.Stats
	busy     bool
	busyTS   string
	workBusy bool
	workTS   string
	notice   string
	err      string
	warn     string
	dirty    bool
	cfgRev   int

	logs    []model.LogEntry
	logRev  uint64
	logView int

	pings      map[string]model.PingResult
	pinging    bool
	pingGen    int
	pingCancel context.CancelFunc
	pingDone   int
	pingTotal  int

	fetchCancel context.CancelFunc
	rsCancel    context.CancelFunc

	tray         *mygo.Tray
	trayConnect  *mygo.MenuItem
	trayHint     bool
	trayReady    bool
	trayLabel    string
	trayTip      string
	trayEnabled  bool
	quitting     bool
	elevating    bool
	elevatedFlag bool

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

	profileRenaming  bool
	profileNameDraft string
	profileDeleting  string
}

// New creates the app state.
func New(deps Deps) *App {
	if deps.Elevated == nil {
		deps.Elevated = platform.IsElevated
	}
	cfg := deps.Config.Clone()
	cfg.Normalize()

	a := &App{
		deps:         deps,
		cfg:          cfg,
		done:         make(chan struct{}),
		state:        model.StateStopped,
		logRev:       ^uint64(0),
		pings:        make(map[string]model.PingResult),
		elevatedFlag: deps.Elevated(),
	}
	a.directText = config.JoinList(cfg.Routing.DirectDomains)
	a.directCIDRs = config.JoinList(cfg.Routing.DirectCIDRs)
	a.proxyText = config.JoinList(cfg.Routing.ProxyDomains)
	a.proxyCIDRs = config.JoinList(cfg.Routing.ProxyCIDRs)
	a.profileNameDraft = ""
	if profile := cfg.ActiveRoutingProfile(); profile != nil {
		a.profileNameDraft = profile.Name
	}
	if deps.ConfigWarning != "" {
		a.notice = deps.ConfigWarning
	}
	return a
}

// Open creates the main window and starts polling the engine.
func (a *App) Open() {
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:     "qbox",
		Width:     1000,
		Height:    720,
		MinWidth:  760,
		MinHeight: 560,
		StateKey:  "main",
		Content:   ui.View(a.view),
	})
	a.win.OnClose(func(e *mygo.CloseEvent) {
		if a.quitting || a.tray == nil {
			return
		}
		e.PreventDefault()
		a.win.Hide()
		if !a.trayHint {
			a.trayHint = true
			a.notice = "qbox продолжает работать в трее. Выход — в меню значка qbox."
		}
	})
	a.startPolling()
	a.initTray()
	if a.deps.Autostart && a.deps.Elevated() {
		a.startEngine()
	}
}

// BeforeQuit runs synchronously in the quit sequence, before windows
// close. It stops the engine so TUN routes and the adapter are removed
// cleanly, then lets the windows close.
func (a *App) BeforeQuit() {
	a.quitting = true
	a.Shutdown()
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
	if a.pingCancel != nil {
		a.pingCancel()
	}
	if a.fetchCancel != nil {
		a.fetchCancel()
	}
	if a.rsCancel != nil {
		a.rsCancel()
	}
	a.saveNow()
	a.deps.Engine.Close()
	if a.tray != nil {
		a.tray.Destroy()
		a.tray = nil
		a.trayConnect = nil
		a.trayReady = false
	}
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
				mygo.RunOnMain(func() {
					if a.closed.Load() {
						return
					}
					state := a.deps.Engine.State()
					stats := a.deps.Engine.Stats()
					lastErr := a.deps.Engine.LastError()
					warning := a.deps.Engine.Warning()
					logRev := a.deps.Engine.LogRevision()

					changed := state != a.state || stats != a.stats || logRev != a.logRev || warning != a.warn
					if !changed {
						return
					}
					a.state = state
					a.stats = stats
					a.warn = warning
					if logRev != a.logRev {
						a.logs, a.logRev = a.deps.Engine.Logs()
					}
					if state == model.StateError && lastErr != "" {
						a.err = lastErr
					}
					a.updateTray()
					if a.win != nil && a.win.IsVisible() {
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
	run := func() {
		if a.closed.Load() {
			return
		}
		fn()
	}
	if a.win == nil {
		run()
		return
	}
	a.win.Update(run)
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
	return a.elevatedFlag
}

func (a *App) start() {
	if a.busy || a.workBusy || a.elevating || a.closed.Load() || a.state == model.StateRunning || a.state == model.StateStarting {
		return
	}
	if !a.elevated() {
		a.requestElevation()
		return
	}
	if a.state == model.StateError {
		a.restartNow()
		return
	}
	a.startEngine()
}

func (a *App) requestElevation() {
	if a.elevating {
		return
	}
	a.elevating = true
	a.busy = true
	a.busyTS = "Запрос прав администратора…"
	a.err = ""
	if err := a.deps.Store.Save(a.cfg); err != nil {
		a.elevating = false
		a.busy = false
		a.busyTS = ""
		a.err = "Не удалось сохранить настройки: " + err.Error()
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}
	args := []string{
		"--elevated",
		"--autostart",
		fmt.Sprintf("--wait-pid=%d", os.Getpid()),
	}
	if err := platform.RelaunchElevated(args...); err != nil {
		a.elevating = false
		a.busy = false
		a.busyTS = ""
		a.err = err.Error()
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}
	mygo.App.Quit()
}

func (a *App) startEngine() {
	if a.closed.Load() {
		return
	}
	a.busy = true
	a.busyTS = "Запуск ядра…"
	a.err = ""
	cfg := a.cfg.Clone()
	rev := a.cfgRev
	paths := a.deps.RuleSets.Paths()
	if a.win == nil {
		a.doStart(cfg, paths, rev)
		return
	}
	a.win.Invalidate()
	go a.doStart(cfg, paths, rev)
}

func (a *App) doStart(cfg config.Config, paths map[string]string, rev int) {
	if a.closed.Load() {
		return
	}
	err := a.deps.Engine.Start(cfg, paths)

	a.post(func() {
		a.busy = false
		a.busyTS = ""
		a.state = a.deps.Engine.State()
		a.updateTray()
		if err != nil {
			a.err = err.Error()
		} else {
			a.dirty = rev != a.cfgRev
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
	if a.busy || a.closed.Load() {
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
	if a.closed.Load() {
		return
	}
	err := a.deps.Engine.Stop()
	a.post(func() {
		a.busy = false
		a.busyTS = ""
		a.state = a.deps.Engine.State()
		a.updateTray()
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
	if a.busy || a.workBusy || a.closed.Load() {
		return
	}
	a.busy = true
	a.busyTS = "Перезапуск ядра…"
	cfg := a.cfg.Clone()
	rev := a.cfgRev
	paths := a.deps.RuleSets.Paths()
	if a.win == nil {
		a.doRestart(cfg, paths, rev)
		return
	}
	a.win.Invalidate()
	go a.doRestart(cfg, paths, rev)
}

func (a *App) doRestart(cfg config.Config, paths map[string]string, rev int) {
	if a.closed.Load() {
		return
	}
	err := a.deps.Engine.Restart(cfg, paths)
	a.post(func() {
		a.busy = false
		a.busyTS = ""
		a.state = a.deps.Engine.State()
		a.updateTray()
		if err != nil {
			a.err = err.Error()
		} else {
			a.dirty = rev != a.cfgRev
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
	activated := false
	if a.cfg.ActiveSubscription == "" || len(a.cfg.ActiveNodes()) == 0 {
		a.cfg.ActiveSubscription = id
		a.cfg.SelectedNode = "auto"
		activated = true
	}
	if activated {
		a.markConfigChanged()
	}
	a.adding = false
	a.addName = ""
	a.addURL = ""
	a.nodeList = ui.ListState{}
	a.cancelPing()
	a.clearPings()
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
	if a.cfg.ActiveSubscription == id {
		a.markConfigChanged()
	}
	a.nodeList = ui.ListState{}
	a.cancelPing()
	a.clearPings()
	if a.state == model.StateRunning {
		a.dirty = true
	}
	a.saveNow()
}

func (a *App) importSubscription() {
	if a.busy || a.workBusy || a.closed.Load() {
		return
	}
	rawURL := strings.TrimSpace(a.addURL)
	if rawURL == "" {
		a.err = "Укажите URL подписки"
		return
	}
	name := a.addName

	a.workBusy = true
	a.workTS = "Загрузка подписки…"
	if a.fetchCancel != nil {
		a.fetchCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	a.fetchCancel = cancel
	if a.win == nil {
		a.doImport(ctx, cancel, name, rawURL)
		return
	}
	go a.doImport(ctx, cancel, name, rawURL)
}

func (a *App) doImport(ctx context.Context, cancel context.CancelFunc, name, rawURL string) {
	defer cancel()
	data, err := a.deps.Fetcher.Fetch(ctx, rawURL)
	if err == nil {
		var nodes []config.Node
		var warnings []string
		nodes, warnings, err = subscription.Parse(data)
		if err == nil {
			a.post(func() {
				a.fetchCancel = nil
				a.workBusy = false
				a.workTS = ""
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
		a.fetchCancel = nil
		a.workBusy = false
		a.workTS = ""
		a.err = "Импорт не удался: " + err.Error()
		if a.win != nil {
			a.win.Invalidate()
		}
	})
}

func (a *App) refreshSubscription(id string) {
	if a.busy || a.workBusy || a.closed.Load() {
		return
	}
	sub := a.cfg.Subscription(id)
	if sub == nil {
		return
	}
	rawURL := sub.URL
	a.workBusy = true
	a.workTS = "Обновление подписки…"
	if a.fetchCancel != nil {
		a.fetchCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	a.fetchCancel = cancel
	run := func() {
		defer cancel()
		data, err := a.deps.Fetcher.Fetch(ctx, rawURL)
		if err == nil {
			var nodes []config.Node
			var warnings []string
			nodes, warnings, err = subscription.Parse(data)
			if err == nil {
				a.post(func() {
					a.fetchCancel = nil
					a.workBusy = false
					a.workTS = ""
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
			a.fetchCancel = nil
			a.workBusy = false
			a.workTS = ""
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
	wasActive := a.cfg.ActiveSubscription == id
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
	a.cancelPing()
	a.clearPings()
	if wasActive {
		a.markConfigChanged()
	}
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
	a.cancelPing()
	a.clearPings()
	a.markConfigChanged()
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
	a.markConfigChanged()
	a.cancelPing()
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
	a.cfg.SyncActiveRoutingProfile()
	a.markConfigChanged()
	a.scheduleSave()
}

func (a *App) markConfigChanged() {
	a.cfgRev++
	if a.state == model.StateRunning {
		a.dirty = true
	}
}

func (a *App) reloadRoutingFields() {
	a.directText = config.JoinList(a.cfg.Routing.DirectDomains)
	a.directCIDRs = config.JoinList(a.cfg.Routing.DirectCIDRs)
	a.proxyText = config.JoinList(a.cfg.Routing.ProxyDomains)
	a.proxyCIDRs = config.JoinList(a.cfg.Routing.ProxyCIDRs)
}

func (a *App) routingProfileNames() []string {
	names := make([]string, 0, len(a.cfg.RoutingProfiles))
	for _, profile := range a.cfg.RoutingProfiles {
		names = append(names, profile.Name)
	}
	return names
}

func (a *App) activeRoutingProfileName() string {
	if profile := a.cfg.ActiveRoutingProfile(); profile != nil {
		return profile.Name
	}
	return ""
}

func (a *App) selectRoutingProfileByName(name string) {
	for _, profile := range a.cfg.RoutingProfiles {
		if profile.Name == name {
			a.selectRoutingProfile(profile.ID)
			return
		}
	}
}

func (a *App) selectRoutingProfile(id string) {
	if id == "" || id == a.cfg.ActiveRoutingProfileID {
		return
	}
	previous := a.cfg.Routing.Clone()
	if !a.cfg.SelectRoutingProfile(id) {
		return
	}
	a.reloadRoutingFields()
	a.profileRenaming = false
	a.profileDeleting = ""
	profile := a.cfg.ActiveRoutingProfile()
	name := id
	if profile != nil {
		name = profile.Name
		a.profileNameDraft = profile.Name
	}
	changed := !previous.Equal(a.cfg.Routing)
	verb := "выбран"
	if changed {
		a.markConfigChanged()
		verb = "применён"
	}
	a.notice = "Профиль «" + name + "» " + verb
	a.saveNow()
	if changed && a.state == model.StateRunning {
		a.restartNow()
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) createRoutingProfile() {
	name := fmt.Sprintf("Профиль %d", len(a.cfg.RoutingProfiles)+1)
	id, err := a.cfg.AddRoutingProfile(name)
	if err != nil {
		a.err = err.Error()
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}
	a.cfg.SelectRoutingProfile(id)
	a.reloadRoutingFields()
	a.profileRenaming = true
	a.profileDeleting = ""
	if profile := a.cfg.ActiveRoutingProfile(); profile != nil {
		a.profileNameDraft = profile.Name
	}
	a.err = ""
	a.notice = "Создан профиль маршрутизации. Задайте имя и правила."
	a.saveNow()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) beginRenameRoutingProfile() {
	profile := a.cfg.ActiveRoutingProfile()
	if profile == nil {
		return
	}
	a.profileRenaming = true
	a.profileNameDraft = profile.Name
	a.profileDeleting = ""
	a.err = ""
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) cancelRenameRoutingProfile() {
	a.profileRenaming = false
	a.profileNameDraft = a.activeRoutingProfileName()
	a.err = ""
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) saveRoutingProfileName() {
	name, ok := a.cfg.RenameRoutingProfile(a.cfg.ActiveRoutingProfileID, a.profileNameDraft)
	if !ok {
		a.err = "Название профиля не может быть пустым"
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}
	a.profileRenaming = false
	a.profileNameDraft = name
	a.err = ""
	a.notice = "Профиль переименован: «" + name + "»"
	a.saveNow()
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) requestDeleteRoutingProfile() {
	if len(a.cfg.RoutingProfiles) <= 1 {
		a.notice = "Нельзя удалить единственный профиль маршрутизации."
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}
	profile := a.cfg.ActiveRoutingProfile()
	if profile == nil {
		return
	}
	a.profileDeleting = profile.ID
	a.profileRenaming = false
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) cancelDeleteRoutingProfile() {
	a.profileDeleting = ""
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) confirmDeleteRoutingProfile() {
	id := a.profileDeleting
	if id == "" {
		return
	}
	a.profileDeleting = ""
	previous := a.cfg.Routing.Clone()
	wasActive := id == a.cfg.ActiveRoutingProfileID
	if !a.cfg.DeleteRoutingProfile(id) {
		return
	}
	if !wasActive {
		a.notice = "Профиль маршрутизации удалён."
		a.saveNow()
		if a.win != nil {
			a.win.Invalidate()
		}
		return
	}
	a.reloadRoutingFields()
	if profile := a.cfg.ActiveRoutingProfile(); profile != nil {
		a.profileNameDraft = profile.Name
	}
	changed := !previous.Equal(a.cfg.Routing)
	if changed {
		a.markConfigChanged()
	}
	a.notice = "Профиль удалён; выбран «" + a.activeRoutingProfileName() + "»"
	a.saveNow()
	if changed && a.state == model.StateRunning {
		a.restartNow()
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) clearPings() {
	a.pings = make(map[string]model.PingResult)
	a.pingDone = 0
	a.pingTotal = 0
}

func (a *App) cancelPing() {
	if !a.pinging {
		return
	}
	a.pingGen++
	if a.pingCancel != nil {
		a.pingCancel()
		a.pingCancel = nil
	}
	a.pinging = false
	a.notice = "Пинг отменён"
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) pingNodes() {
	if a.pinging || a.busy || a.workBusy || a.closed.Load() {
		return
	}
	nodes := a.cfg.ActiveNodes()
	if len(nodes) == 0 {
		return
	}
	work := make([]config.Node, len(nodes))
	for i, node := range nodes {
		work[i] = node.Clone()
	}

	if a.pingCancel != nil {
		a.pingCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	a.pingCancel = cancel
	a.pingGen++
	gen := a.pingGen
	a.pinging = true
	a.clearPings()
	a.pingTotal = len(work)
	a.notice = ""

	run := func() {
		defer cancel()
		ok := 0
		for result := range a.deps.Engine.Ping(ctx, work) {
			if result.Err == "" {
				ok++
			}
			value := result
			a.post(func() {
				if gen != a.pingGen {
					return
				}
				a.pings[value.Tag] = value
				a.pingDone++
				if a.win != nil {
					a.win.Invalidate()
				}
			})
		}
		a.post(func() {
			if gen != a.pingGen {
				return
			}
			a.pinging = false
			a.pingCancel = nil
			a.notice = fmt.Sprintf("Пинг: %d из %d ответили", ok, len(work))
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

// latency returns the label and tone for a node's last ping.
func (a *App) latency(tag string) (string, string) {
	result, ok := a.pings[tag]
	if !ok {
		if a.pinging {
			return "…", "pending"
		}
		return "", ""
	}
	if result.Err != "" {
		return "ошибка", "error"
	}
	if result.Latency < time.Millisecond {
		return "<1 мс", "ok"
	}
	return fmt.Sprintf("%d мс", result.Latency.Milliseconds()), "ok"
}

func (a *App) updateRuleSet(tag string) {
	if a.busy || a.workBusy || a.closed.Load() {
		return
	}
	a.workBusy = true
	a.workTS = "Скачивание баз…"
	if a.rsCancel != nil {
		a.rsCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	a.rsCancel = cancel
	run := func() {
		defer cancel()
		err := a.deps.RuleSets.Update(ctx, tag)
		a.post(func() {
			a.rsCancel = nil
			a.workBusy = false
			a.workTS = ""
			if err != nil {
				a.err = "Базы: " + err.Error()
			} else {
				a.err = ""
				a.notice = "Базы обновлены. Перезапустите подключение, чтобы применить правила."
			}
			if a.state == model.StateRunning {
				a.markConfigChanged()
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
	if a.busy || a.workBusy || a.closed.Load() {
		return
	}
	a.workBusy = true
	a.workTS = "Скачивание баз…"
	if a.rsCancel != nil {
		a.rsCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	a.rsCancel = cancel
	run := func() {
		defer cancel()
		err := a.deps.RuleSets.Update(ctx)
		a.post(func() {
			a.rsCancel = nil
			a.workBusy = false
			a.workTS = ""
			if err != nil {
				a.err = "Базы: " + err.Error()
			} else {
				a.err = ""
				a.notice = "Базы обновлены. Перезапустите подключение, чтобы применить правила."
			}
			if a.state == model.StateRunning {
				a.markConfigChanged()
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
