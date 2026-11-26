package app

import (
	"context"
	"errors"
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

type fakeEngine struct {
	mu        sync.Mutex
	state     model.State
	stats     model.Stats
	logs      []model.LogEntry
	started   int
	stopped   int
	restarted int
	pinged    int
	startErr  error
	stopErr   error
	cfg       config.Config
	paths     map[string]string
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
	f.state = model.StateRunning
	return nil
}

func (f *fakeEngine) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped++
	f.state = model.StateStopped
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
		CacheDir:   t.TempDir(),
		Elevated:   func() bool { return true },
	})
}

func TestViewAndStart(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})

	tt := ui.NewTester(a.view, 1000, 720)
	if !tt.HasText("Подключение") || !tt.HasText("qbox") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	if err := tt.Click("Подключиться"); err != nil {
		t.Fatal(err)
	}
	if eng.started != 1 {
		t.Fatalf("start calls = %d", eng.started)
	}
	if !tt.HasText("Подключено") {
		t.Fatalf("state not shown: %q", tt.Texts())
	}
	if err := tt.Click("Серверы"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("one") {
		t.Fatalf("node not shown: %q", tt.Texts())
	}
}

func TestNavigation(t *testing.T) {
	cfg := testConfig(t, testNode(t, testVLESS))
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})
	tt := ui.NewTester(a.view, 1000, 720)

	if err := tt.Click("Серверы"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Подписки") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	if err := tt.Click("Маршрутизация"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Обход РФ") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	if err := tt.Click("Настройки"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Strict route") {
		t.Fatalf("texts = %q", tt.Texts())
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

func TestPingNodes(t *testing.T) {
	second := testNode(t, "vless://22222222-2222-2222-2222-222222222222@two.example:443?security=tls#two")
	cfg := testConfig(t, testNode(t, testVLESS), second)
	eng := &fakeEngine{state: model.StateStopped}
	a := newApp(t, cfg, eng, fakeFetcher{})

	tt := ui.NewTester(a.view, 1000, 720)
	if err := tt.Click("Серверы"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Пинг всех"); err != nil {
		t.Fatal(err)
	}
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
	a.deleteSubscription("sub-1")
	if len(a.cfg.Subscriptions) != 0 || a.cfg.ActiveSubscription != "" {
		t.Fatalf("cfg = %+v", a.cfg)
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

func TestDisplayURLRedacts(t *testing.T) {
	got := displayURL("https://user:pass@example.com/sub/TOKEN?token=SECRET")
	if strings.Contains(got, "TOKEN") || strings.Contains(got, "SECRET") || strings.Contains(got, "user") {
		t.Fatalf("displayURL = %q", got)
	}
	if got != "https://example.com/…" {
		t.Fatalf("displayURL = %q", got)
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
