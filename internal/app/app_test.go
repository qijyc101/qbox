package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"qbox/internal/config"
	"qbox/internal/model"
	"qbox/internal/rulesets"
	"qbox/internal/subscription"
)

type fakeLoginItem struct {
	enabled bool
	err     error
}

func (f *fakeLoginItem) Set(enabled bool) error {
	if f.err != nil {
		return f.err
	}
	f.enabled = enabled
	return nil
}

type fakeEngine struct {
	mu         sync.Mutex
	state      model.State
	stats      model.Stats
	logs       []model.LogEntry
	started    int
	stopped    int
	restarted  int
	pinged     int
	startErr   error
	stopErr    error
	cfg        config.Config
	paths      map[string]string
	proxyAddr  string
	lastConfig []byte
	conns      []model.Connection
}

func (f *fakeEngine) Start(cfg config.Config, paths map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started++
	if f.startErr != nil {
		f.state = model.StateError
		return f.startErr
	}
	f.cfg = cfg
	f.paths = paths
	if cfg.TUN.Enabled {
		f.proxyAddr = ""
	} else if f.proxyAddr == "" {
		f.proxyAddr = "127.0.0.1:2080"
	}
	f.lastConfig = []byte(`{"log":{"level":"info"}}`)
	f.state = model.StateRunning
	return nil
}

func (f *fakeEngine) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped++
	f.state = model.StateStopped
	f.proxyAddr = ""
	return f.stopErr
}

func (f *fakeEngine) Restart(cfg config.Config, paths map[string]string) error {
	f.mu.Lock()
	f.stopped++
	f.restarted++
	f.mu.Unlock()
	return f.Start(cfg, paths)
}

func (f *fakeEngine) State() model.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *fakeEngine) LastError() string { return "" }
func (f *fakeEngine) Warning() string   { return "" }

func (f *fakeEngine) Stats() model.Stats {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stats
}

func (f *fakeEngine) Connections() []model.Connection {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]model.Connection(nil), f.conns...)
}

func (f *fakeEngine) ProxyAddress() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.proxyAddr
}

func (f *fakeEngine) LastConfig() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]byte(nil), f.lastConfig...)
}

func (f *fakeEngine) Logs() ([]model.LogEntry, uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]model.LogEntry(nil), f.logs...), uint64(len(f.logs))
}

func (f *fakeEngine) LogRevision() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return uint64(len(f.logs))
}

func (f *fakeEngine) ClearLogs() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = nil
}

func (f *fakeEngine) Close() {
	_ = f.Stop()
}

func (f *fakeEngine) Ping(ctx context.Context, nodes []config.Node) <-chan model.PingResult {
	f.mu.Lock()
	f.pinged++
	f.mu.Unlock()
	results := make(chan model.PingResult, len(nodes))
	for _, node := range nodes {
		results <- model.PingResult{Tag: node.Tag, Latency: 123 * time.Millisecond}
	}
	close(results)
	return results
}

type fakeRules struct {
	paths  map[string]string
	status []rulesets.Status
	update func(context.Context, ...string) error
}

type fakeSystemProxy struct {
	enabled  int
	restored int
	address  string
	err      error
	pending  bool
}

func (f *fakeSystemProxy) Enable(address string) error {
	f.enabled++
	f.address = address
	return f.err
}

func (f *fakeSystemProxy) Restore() error {
	f.restored++
	f.pending = false
	return f.err
}

func (f *fakeSystemProxy) Pending() bool { return f.pending }

func (f *fakeRules) Paths() map[string]string  { return f.paths }
func (f *fakeRules) Status() []rulesets.Status { return f.status }
func (f *fakeRules) Update(ctx context.Context, tags ...string) error {
	if f.update != nil {
		return f.update(ctx, tags...)
	}
	return nil
}

type fakeFetcher struct {
	data []byte
	err  error
}

func (f fakeFetcher) Fetch(context.Context, string) ([]byte, error) {
	return f.data, f.err
}

const testVLESS = "vless://11111111-1111-1111-1111-111111111111@one.example:443?security=tls#one"

var testTime = time.Unix(1700000000, 0).UTC()

func testNode(t *testing.T, line string) config.Node {
	t.Helper()
	nodes, _, err := subscription.Parse([]byte(line))
	if err != nil || len(nodes) != 1 {
		t.Fatalf("parse: %v", err)
	}
	return nodes[0]
}

func testConfig(t *testing.T, nodes ...config.Node) config.Config {
	t.Helper()
	cfg := config.Defaults()
	cfg.Subscriptions = []config.Subscription{{
		ID:        "sub-1",
		Name:      "Test",
		URL:       "https://example.com/sub",
		UpdatedAt: testTime,
		Nodes:     nodes,
	}}
	cfg.ActiveSubscription = "sub-1"
	return cfg
}

func newApp(t *testing.T, cfg config.Config, eng *fakeEngine, fetch Fetcher) *App {
	t.Helper()
	store, err := config.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{
		Config:     cfg,
		Store:      store,
		Engine:     eng,
		Fetcher:    fetch,
		RuleSets:   &fakeRules{paths: map[string]string{}},
		ConfigPath: store.Path(),
		LoginItem:  &fakeLoginItem{},
		Elevated:   func() bool { return true },
	})
}

func TestViewAndStart(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})

	tt := ui.NewTester(a.view, 1100, 760)
	if !tt.HasText("Подписки") || !tt.HasText("Ноды") || !tt.HasText("Настройки") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	if !tt.HasText("one") {
		t.Fatalf("node not shown: %q", tt.Texts())
	}
	if err := tt.Click("Подключиться"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if eng.started != 1 {
		t.Fatalf("start calls = %d", eng.started)
	}
	if !tt.HasText("Подключено") {
		t.Fatalf("state not shown: %q", tt.Texts())
	}
}

func TestNavigation(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	tt := ui.NewTester(a.view, 1100, 760)

	if err := tt.Click("Настройки"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Внешний вид") || !tt.HasText("Режим TUN") {
		t.Fatalf("settings texts = %q", tt.Texts())
	}
	if err := tt.Click("На главную"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Подписки") {
		t.Fatalf("home texts = %q", tt.Texts())
	}
	if err := tt.Click("Маршрутизация"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Новый профиль…") || !tt.HasText("Основной") {
		t.Fatalf("routing menu = %q", tt.Texts())
	}
}

func TestMenuHoverSwitch(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	tt := ui.NewTester(a.view, 1100, 760)

	if err := tt.Click("Файл"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("config.json") {
		t.Fatalf("file menu = %q", tt.Texts())
	}

	rect, ok := tt.Find("Маршрутизация")
	if !ok {
		t.Fatal("routing trigger not found")
	}
	tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
	tt.Frame()
	if !tt.HasText("Новый профиль…") || tt.HasText("config.json") {
		t.Fatalf("menu after hover = %q", tt.Texts())
	}
}

func TestRoutingMenuEditAction(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	tt := ui.NewTester(a.view, 1100, 760)

	if err := tt.Click("Маршрутизация"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.profileNameDraft = "changed"
	if err := tt.Click("Редактировать Основной"); err != nil {
		t.Fatalf("edit action: %v", err)
	}
	tt.Frame()
	if a.profileNameDraft != "Основной" {
		t.Fatalf("edit action did not open the profile editor: %q", a.profileNameDraft)
	}
}

func TestNodeSelection(t *testing.T) {
	second := testNode(t, "vless://22222222-2222-2222-2222-222222222222@two.example:443?security=tls#two")
	cfg := testConfig(t, testNode(t, testVLESS), second)
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})

	a.selectNode(2)
	if a.cfg.SelectedNode != "two" {
		t.Fatalf("selected = %q", a.cfg.SelectedNode)
	}
	if eng.stopped != 0 {
		t.Fatal("restart was attempted while stopped")
	}
}

func TestRoutingProfiles(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateRunning}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.state = model.StateRunning
	firstID := a.cfg.ActiveRoutingProfileID
	if firstID == "" {
		t.Fatal("no active routing profile")
	}

	a.cfg.Routing.Mode = config.ModeGlobal
	a.openNewProfile()
	a.profileNameDraft = "Работа"
	a.saveNewProfile()
	secondID := a.cfg.ActiveRoutingProfileID
	if secondID == firstID || len(a.cfg.RoutingProfiles) != 2 {
		t.Fatalf("profiles = %+v active=%q", a.cfg.RoutingProfiles, a.cfg.ActiveRoutingProfileID)
	}
	if a.cfg.RoutingProfile(firstID).Routing.Mode != config.ModeGlobal {
		t.Fatalf("first profile lost the edit: %+v", a.cfg.RoutingProfile(firstID).Routing)
	}

	a.setMode(2) // Direct for the second profile.
	if got := a.cfg.RoutingProfile(secondID); got == nil || got.Name != "Работа" || got.Routing.Mode != config.ModeDirect {
		t.Fatalf("second profile = %+v", got)
	}

	a.selectRoutingProfile(firstID)
	if a.cfg.Routing.Mode != config.ModeGlobal {
		t.Fatalf("first profile mode = %q", a.cfg.Routing.Mode)
	}
	a.selectRoutingProfile(secondID)
	if a.cfg.Routing.Mode != config.ModeDirect {
		t.Fatalf("second profile mode = %q", a.cfg.Routing.Mode)
	}
	if eng.restarted == 0 {
		t.Fatal("profile switch did not restart the engine")
	}

	a.openProfileEditor()
	a.profileNameDraft = "Личная"
	a.saveRoutingProfileName()
	if a.cfg.ActiveRoutingProfile().Name != "Личная" {
		t.Fatalf("rename = %q", a.cfg.ActiveRoutingProfile().Name)
	}

	a.profileDeleteConfirm = true
	a.deleteActiveProfile()
	if len(a.cfg.RoutingProfiles) != 1 || a.cfg.ActiveRoutingProfileID == secondID {
		t.Fatalf("profiles after delete = %+v active=%q", a.cfg.RoutingProfiles, a.cfg.ActiveRoutingProfileID)
	}
}

func TestRoutingProfileSelectUI(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.openNewProfile()
	a.profileNameDraft = "Работа"
	a.saveNewProfile()

	tt := ui.NewTester(a.view, 1100, 760)
	if err := tt.Click("Маршрутизация"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Работа") || !tt.HasText("Основной") {
		t.Fatalf("profiles not shown: %q", tt.Texts())
	}
	if err := tt.Click("Основной"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.activeRoutingProfileName() != "Основной" {
		t.Fatalf("active profile = %q", a.activeRoutingProfileName())
	}
}

func TestSetMTUHonorsIPv6Minimum(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	a := newApp(t, cfg, &fakeEngine{state: model.StateStopped}, fakeFetcher{})

	a.setMTU(1000)
	if a.cfg.TUN.MTU != config.DefaultMTU {
		t.Fatalf("IPv6 MTU below 1280 was accepted: %d", a.cfg.TUN.MTU)
	}
	a.setMTU(config.MinMTUIPv6)
	if a.cfg.TUN.MTU != config.MinMTUIPv6 {
		t.Fatalf("valid IPv6 MTU was rejected: %d", a.cfg.TUN.MTU)
	}

	a.cfg.TUN.DisableIPv6 = true
	a.setMTU(config.MinMTU)
	if a.cfg.TUN.MTU != config.MinMTU {
		t.Fatalf("IPv4-only MTU was rejected: %d", a.cfg.TUN.MTU)
	}
}

func TestPingNodes(t *testing.T) {
	second := testNode(t, "vless://22222222-2222-2222-2222-222222222222@two.example:443?security=tls#two")
	cfg := testConfig(t, testNode(t, testVLESS), second)
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})

	tt := ui.NewTester(a.view, 1100, 760)
	if err := tt.Click("Пинг"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if eng.pinged != 1 {
		t.Fatalf("ping calls = %d", eng.pinged)
	}
	if a.pinging {
		t.Fatal("still pinging")
	}
	if len(a.pings) != 2 {
		t.Fatalf("pings = %+v", a.pings)
	}
	if !tt.HasText("123 мс") {
		t.Fatalf("latency not shown: %q", tt.Texts())
	}
}

func TestImportSubscription(t *testing.T) {
	cfg := config.Defaults()
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{data: []byte(testVLESS)})

	a.adding = true
	a.addName = "Provider"
	a.addURL = "https://example.com/sub"
	a.importSubscription()

	if len(a.cfg.Subscriptions) != 1 || len(a.cfg.Subscriptions[0].Nodes) != 1 {
		t.Fatalf("subscriptions = %+v", a.cfg.Subscriptions)
	}
	if a.cfg.ActiveSubscription == "" || a.cfg.SelectedNode != "auto" {
		t.Fatalf("active = %q selected = %q", a.cfg.ActiveSubscription, a.cfg.SelectedNode)
	}
	if a.adding {
		t.Fatal("form is still open")
	}
}

func TestImportError(t *testing.T) {
	cfg := testConfig(t)
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{err: errors.New("network down")})
	a.adding = true
	a.addURL = "https://example.com/sub"
	a.importSubscription()
	if a.err == "" {
		t.Fatal("no error shown")
	}
	if a.workBusy {
		t.Fatal("workBusy was not cleared")
	}
}

func TestStartErrorIsShown(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped, startErr: errors.New("wintun missing")}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.start()
	if a.err == "" {
		t.Fatal("error not shown")
	}
	if a.busy {
		t.Fatal("busy was not cleared")
	}
}

func TestDeleteSubscription(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.openDeleteSubscription("sub-1")
	if a.subDeleting == "" {
		t.Fatal("delete was not requested")
	}
	a.confirmDeleteSubscription()
	if len(a.cfg.Subscriptions) != 0 || a.cfg.ActiveSubscription != "" {
		t.Fatalf("cfg = %+v", a.cfg)
	}
}

func TestRenameSubscription(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.openRenameSubscription("sub-1")
	a.subNameDraft = "  Работа  "
	a.renameSubscription()
	if a.cfg.Subscription("sub-1").Name != "Работа" {
		t.Fatalf("name = %q", a.cfg.Subscription("sub-1").Name)
	}
}

func TestAppearanceSettings(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.setThemeIndex(2)
	a.setAccent(config.AccentViolet)
	a.setCornersIndex(0)
	if a.cfg.Appearance.Theme != config.ThemeDark ||
		a.cfg.Appearance.Accent != config.AccentViolet ||
		a.cfg.Appearance.Corners != config.CornersCompact {
		t.Fatalf("appearance = %+v", a.cfg.Appearance)
	}
	if a.themeIndex() != 2 || a.cornersIndex() != 0 {
		t.Fatal("appearance indexes are wrong")
	}
}

func TestTrafficHistory(t *testing.T) {
	a := newApp(t, testConfig(t, testNode(t, testVLESS)), &fakeEngine{state: model.StateStopped}, fakeFetcher{})
	now := time.Now()
	a.resetTraffic(model.Stats{Up: 100, Down: 1000})
	a.lastSample = now
	if !a.sampleTraffic(now.Add(2*time.Second), model.Stats{Up: 300, Down: 5000}) {
		t.Fatal("no sample after a second")
	}
	if len(a.traffic) != 1 {
		t.Fatalf("traffic = %+v", a.traffic)
	}
	if a.traffic[0].Up != 100 || a.traffic[0].Down != 2000 {
		t.Fatalf("rates = %+v", a.traffic[0])
	}
}

func TestConnectionsView(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateRunning, conns: []model.Connection{{
		ID: "c1", Host: "example.com", Destination: "example.com:443",
		Network: "tcp", Outbound: "node1", CreatedAt: time.Now(),
	}}}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.state = model.StateRunning
	a.panel = 2
	a.refreshConnections()
	tt := ui.NewTester(a.view, 1100, 760)
	if !tt.HasText("example.com") {
		t.Fatalf("connection not shown: %q", tt.Texts())
	}
}

func TestBeforeQuitStopsEngine(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateRunning}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.BeforeQuit()
	if !a.quitting {
		t.Fatal("quitting flag is not set")
	}
	if !a.closed.Load() {
		t.Fatal("app is not closed")
	}
	if eng.stopped == 0 {
		t.Fatal("engine was not stopped")
	}
}

func TestCancelPing(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	canceled := false
	a.pinging = true
	a.pingGen = 1
	a.pingTotal = 2
	a.pingDone = 1
	a.pingCancel = func() { canceled = true }
	a.cancelPing()
	if !canceled || a.pinging {
		t.Fatalf("canceled=%v pinging=%v", canceled, a.pinging)
	}
	if a.notice != "Пинг отменён" {
		t.Fatalf("notice = %q", a.notice)
	}
}

func TestRetryAfterErrorRestarts(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateError, startErr: errors.New("old failure")}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.state = model.StateError
	a.start()
	if eng.restarted != 1 {
		t.Fatalf("restarts = %d", eng.restarted)
	}
}

func TestStopDuringDownload(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateRunning}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.state = model.StateRunning
	a.workBusy = true
	a.stop()
	if eng.stopped != 1 {
		t.Fatalf("stops = %d", eng.stopped)
	}
}

func TestPostAfterShutdownIsDropped(t *testing.T) {
	cfg := testConfig(t)
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.closed.Store(true)
	called := false
	a.post(func() { called = true })
	if called {
		t.Fatal("post ran after shutdown")
	}
}

func TestConfigRevisionKeepsDirty(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.cfgRev = 5
	a.doStart(cfg, nil, 4)
	if !a.dirty {
		t.Fatal("dirty was cleared although config changed during start")
	}
}

func TestSavesConfiguration(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.cfg.Routing.Mode = config.ModeGlobal
	a.saveNow()
	loaded, err := a.deps.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Routing.Mode != config.ModeGlobal {
		t.Fatalf("mode = %q", loaded.Routing.Mode)
	}
}

func TestProxyModeEnablesAndRestoresSystemProxy(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	cfg.TUN.Enabled = false
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	proxy := &fakeSystemProxy{}
	a.deps.SystemProxy = proxy
	a.elevatedFlag = false

	a.start()
	if eng.started != 1 {
		t.Fatalf("start calls = %d", eng.started)
	}
	if proxy.enabled != 1 || proxy.address != "127.0.0.1:2080" {
		t.Fatalf("system proxy = %+v", proxy)
	}
	if a.proxyAddr != "127.0.0.1:2080" {
		t.Fatalf("proxy address = %q", a.proxyAddr)
	}

	a.stop()
	if proxy.restored != 1 {
		t.Fatalf("restore calls = %d", proxy.restored)
	}
	if a.proxyAddr != "" {
		t.Fatalf("proxy address after stop = %q", a.proxyAddr)
	}
}

func TestProxyModeRollsBackWhenSystemProxyFails(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	cfg.TUN.Enabled = false
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.deps.SystemProxy = &fakeSystemProxy{err: errors.New("denied")}

	a.doStart(cfg, nil, 0)
	if eng.State() != model.StateStopped {
		t.Fatalf("engine state = %q", eng.State())
	}
	if a.err == "" {
		t.Fatal("error was not shown")
	}
}

func TestTUNModeDoesNotTouchSystemProxy(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	proxy := &fakeSystemProxy{}
	a.deps.SystemProxy = proxy

	a.doStart(cfg, nil, 0)
	if proxy.enabled != 0 || proxy.restored != 0 {
		t.Fatalf("system proxy calls = %+v", proxy)
	}
}

func TestConfigViewer(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	a := newApp(t, cfg, &fakeEngine{state: model.StateStopped}, fakeFetcher{})
	a.saveNow()
	view := a.loadTextViewer(viewerConfig)
	if view.Err != "" {
		t.Fatal(view.Err)
	}
	if !strings.Contains(view.Text, `"version"`) || !strings.Contains(view.Text, `"routing"`) {
		t.Fatalf("config viewer text = %q", view.Text)
	}
}

func TestHomeLayoutWithManyNodes(t *testing.T) {
	nodes := make([]config.Node, 0, 15)
	for i := 0; i < 15; i++ {
		nodes = append(nodes, testNode(t, fmt.Sprintf(
			"vless://11111111-1111-1111-1111-11111111111%x@node%d.example:443?security=tls#node%d",
			i, i, i)))
	}
	cfg := testConfig(t, nodes...)
	a := newApp(t, cfg, &fakeEngine{state: model.StateStopped}, fakeFetcher{})
	tt := ui.NewTester(a.view, 1000, 684)
	tt.Frame()

	button, ok := tt.Find("Добавить подписку")
	if !ok {
		t.Fatal("the add subscription button is missing")
	}
	if button.H < 20 || button.Y+button.H > 684 {
		t.Fatalf("the add subscription button does not fit: %+v", button)
	}
	footer, ok := tt.Find("60 секунд")
	if !ok || footer.Y+footer.H > 684 {
		t.Fatalf("the traffic footer does not fit: %+v", footer)
	}
}

func TestSubscriptionURLHidden(t *testing.T) {
	got := displaySubscriptionURL("https://user:pass@example.com/api/sub/SECRET?token=TOKEN")
	if strings.Contains(got, "SECRET") || strings.Contains(got, "TOKEN") || strings.Contains(got, "user") {
		t.Fatalf("displaySubscriptionURL = %q", got)
	}
	if !strings.Contains(got, "example.com") {
		t.Fatalf("host is missing: %q", got)
	}
}

func TestSubscriptionMenu(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	a := newApp(t, cfg, &fakeEngine{state: model.StateStopped}, fakeFetcher{})
	tt := ui.NewTester(a.view, 1100, 760)
	if err := tt.Click("Меню подписки"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Обновить") || !tt.HasText("Переименовать…") || !tt.HasText("Удалить…") {
		t.Fatalf("subscription menu = %q", tt.Texts())
	}
}

func TestAutoPing(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	cfg.Ping.Auto = true
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	a.autoPingAt = time.Now().Add(-time.Second)
	a.maybeAutoPing(time.Now())
	if eng.pinged != 1 {
		t.Fatalf("ping calls = %d", eng.pinged)
	}
	if a.lastPingAt.IsZero() {
		t.Fatal("last ping time was not set")
	}
	if !a.cfg.Ping.Auto {
		t.Fatal("auto ping setting was lost")
	}
}

func TestRoutingEditorCards(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	a := newApp(t, cfg, &fakeEngine{state: model.StateStopped}, fakeFetcher{})
	tt := ui.NewTester(a.routingEditorView, 700, 900)
	tt.Frame()
	for _, want := range []string{"Название", "Режим", "Правила", "Базы правил", "Свои списки", "Удаление профиля"} {
		if !tt.HasText(want) {
			t.Fatalf("the routing editor has no %q card: %q", want, tt.Texts())
		}
	}
	if !tt.HasText("Закрыть") {
		t.Fatalf("the routing editor has no close button: %q", tt.Texts())
	}
}

func TestChildDialogHeader(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	a := newApp(t, cfg, &fakeEngine{state: model.StateStopped}, fakeFetcher{})
	a.adding = true
	tt := ui.NewTester(a.addSubscriptionView, 480, 340)
	tt.Frame()
	if !tt.HasText("Новая подписка") || !tt.HasText("Закрыть") {
		t.Fatalf("the dialog header = %q", tt.Texts())
	}
}

func TestFileExitHoverRed(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	a := newApp(t, cfg, &fakeEngine{state: model.StateStopped}, fakeFetcher{})
	tt := ui.NewTester(a.view, 1100, 760)

	if err := tt.Click("Файл"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	exit, ok := tt.Find("Выход")
	if !ok {
		t.Fatalf("the exit item is missing: %q", tt.Texts())
	}
	tt.Move(exit.X+exit.W/2, exit.Y+exit.H/2)
	tt.Frame()
	pixel := tt.Image().RGBAAt(int(exit.X)+10, int(exit.Y+exit.H/2))
	if pixel.R < 150 || pixel.G > 100 || pixel.B > 100 {
		t.Fatalf("the exit highlight is %v, want red", pixel)
	}
}

func TestStartupSettings(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	a := newApp(t, cfg, &fakeEngine{state: model.StateStopped}, fakeFetcher{})
	login := &fakeLoginItem{}
	a.deps.LoginItem = login

	a.setLaunchAtLogin(true)
	if !login.enabled || !a.cfg.Startup.LaunchAtLogin {
		t.Fatalf("launch at login = %v, config %v", login.enabled, a.cfg.Startup.LaunchAtLogin)
	}
	loaded, err := a.deps.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Startup.LaunchAtLogin {
		t.Fatal("launch at login was not saved")
	}

	a.setAutoConnect(true)
	if !a.cfg.Startup.AutoConnect || !a.shouldAutoConnect() {
		t.Fatal("auto connect was not saved")
	}
	loaded, err = a.deps.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Startup.AutoConnect {
		t.Fatal("auto connect was not persisted")
	}
	a.deps.Autostart = true
	a.cfg.Startup.AutoConnect = false
	if !a.shouldAutoConnect() {
		t.Fatal("an elevated connection relaunch must auto connect")
	}
	a.deps.Autostart = false

	a.settingsOpen = true
	tt := ui.NewTester(a.view, 1120, 1500)
	tt.Frame()
	if !tt.HasText("Запускать qbox при входе в систему") || !tt.HasText("Подключаться при запуске qbox") {
		t.Fatalf("startup settings = %q", tt.Texts())
	}
	a.settingsOpen = false

	a.setLaunchAtLogin(false)
	if login.enabled || a.cfg.Startup.LaunchAtLogin {
		t.Fatal("launch at login was not removed")
	}

	login.err = errors.New("denied")
	a.setLaunchAtLogin(true)
	if a.cfg.Startup.LaunchAtLogin || a.err == "" {
		t.Fatal("a failed autostart change was not reported")
	}
}
