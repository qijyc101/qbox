package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultsNormalize(t *testing.T) {
	cfg := Defaults()
	cfg.Normalize()
	if cfg.Version != Version {
		t.Fatalf("version = %d", cfg.Version)
	}
	if cfg.Routing.Mode != ModeRules || !cfg.Routing.BypassLAN || !cfg.Routing.BypassRU || !cfg.Routing.BlockAds {
		t.Fatalf("unexpected routing defaults: %+v", cfg.Routing)
	}
	if cfg.TUN.Stack != StackMixed || cfg.SelectedNode != "auto" {
		t.Fatalf("unexpected tun defaults: %+v selected=%q", cfg.TUN, cfg.SelectedNode)
	}

	var broken Config
	broken.Normalize()
	if broken.Routing.Mode != ModeRules || broken.TUN.Stack != StackMixed {
		t.Fatalf("zero config was not normalized: %+v", broken)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	cfg.Subscriptions = []Subscription{{
		ID:        "sub-1",
		Name:      "Provider",
		URL:       "https://example.com/sub",
		UpdatedAt: time.Unix(1700000000, 0).UTC(),
		Nodes: []Node{{
			Tag:      "node",
			Type:     "vless",
			Server:   "example.com",
			Port:     443,
			Outbound: json.RawMessage(`{"type":"vless","tag":"node","server":"example.com","server_port":443,"uuid":"u"}`),
		}},
	}}
	cfg.ActiveSubscription = "sub-1"
	cfg.SelectedNode = "node"

	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ActiveSubscription != "sub-1" || loaded.SelectedNode != "node" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if len(loaded.Subscriptions) != 1 || len(loaded.Subscriptions[0].Nodes) != 1 {
		t.Fatalf("nodes not round-tripped: %+v", loaded.Subscriptions)
	}
	if got := loaded.ActiveNodes(); len(got) != 1 || got[0].Tag != "node" {
		t.Fatalf("active nodes = %+v", got)
	}
}

func TestCorruptFallsBackToBackup(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)
	cfg := Defaults()
	cfg.LogLevel = "debug"
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	// The next save writes the previous file to .bak.
	cfg.LogLevel = "trace"
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err == nil {
		t.Fatal("expected a warning for the damaged file")
	}
	if loaded.LogLevel != "debug" {
		t.Fatalf("backup not used, log level = %q", loaded.LogLevel)
	}
}

func TestCloneIsDeep(t *testing.T) {
	cfg := Defaults()
	cfg.Subscriptions = []Subscription{{ID: "a", Nodes: []Node{{Tag: "n", Outbound: json.RawMessage(`{"a":1}`)}}}}
	clone := cfg.Clone()
	clone.Subscriptions[0].Nodes[0].Tag = "changed"
	clone.Subscriptions[0].Nodes[0].Outbound[2] = '9'
	if cfg.Subscriptions[0].Nodes[0].Tag != "n" {
		t.Fatal("tag was shared")
	}
	if string(cfg.Subscriptions[0].Nodes[0].Outbound) != `{"a":1}` {
		t.Fatal("raw outbound was shared")
	}
}

func TestCleanList(t *testing.T) {
	got := CleanList([]string{" a ", "", "# comment", "a", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("CleanList = %#v", got)
	}
}
