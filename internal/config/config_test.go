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

func TestRoutingProfileDefaultsAndMigration(t *testing.T) {
	cfg := Defaults()
	cfg.Normalize()
	if len(cfg.RoutingProfiles) != 1 {
		t.Fatalf("profiles = %+v", cfg.RoutingProfiles)
	}
	active := cfg.ActiveRoutingProfile()
	if active == nil || active.ID != cfg.ActiveRoutingProfileID || active.Name == "" {
		t.Fatalf("active profile = %+v", active)
	}
	if !active.Routing.Equal(cfg.Routing) {
		t.Fatalf("active routing differs: %+v", active.Routing)
	}

	var old Config
	if err := json.Unmarshal([]byte(`{"version":1,"routing":{"mode":"global","bypass_lan":true,"direct_domains":["corp.example"]}}`), &old); err != nil {
		t.Fatal(err)
	}
	old.Normalize()
	if old.Version != Version {
		t.Fatalf("version = %d", old.Version)
	}
	if len(old.RoutingProfiles) != 1 || old.RoutingProfiles[0].Name == "" {
		t.Fatalf("profiles = %+v", old.RoutingProfiles)
	}
	if old.Routing.Mode != ModeGlobal || !old.RoutingProfiles[0].Routing.Equal(old.Routing) {
		t.Fatalf("migration lost routing: flat=%+v profile=%+v", old.Routing, old.RoutingProfiles[0].Routing)
	}

	var profilesOnly Config
	if err := json.Unmarshal([]byte(`{"version":2,"routing_profiles":[{"id":"p","name":"Работа","routing":{"mode":"direct"}}],"active_routing_profile":"p"}`), &profilesOnly); err != nil {
		t.Fatal(err)
	}
	profilesOnly.Normalize()
	if profilesOnly.Routing.Mode != ModeDirect || profilesOnly.ActiveRoutingProfileID != "p" {
		t.Fatalf("profiles-only config = %+v", profilesOnly)
	}
}

func TestRoutingProfileSwitchKeepsEdits(t *testing.T) {
	cfg := Defaults()
	firstID := cfg.ActiveRoutingProfileID
	secondID, err := cfg.AddRoutingProfile("Работа")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SelectRoutingProfile(secondID) {
		t.Fatal("cannot select the new profile")
	}
	cfg.Routing.Mode = ModeDirect
	cfg.Routing.DirectDomains = []string{"corp.example"}
	if !cfg.SelectRoutingProfile(firstID) {
		t.Fatal("cannot switch back")
	}
	if cfg.Routing.Mode != ModeRules || len(cfg.Routing.DirectDomains) != 0 {
		t.Fatalf("first profile = %+v", cfg.Routing)
	}
	if got := cfg.RoutingProfile(secondID); got == nil || got.Routing.Mode != ModeDirect || len(got.Routing.DirectDomains) != 1 {
		t.Fatalf("second profile = %+v", got)
	}
	if !cfg.SelectRoutingProfile(secondID) || cfg.Routing.Mode != ModeDirect {
		t.Fatalf("second profile not restored: %+v", cfg.Routing)
	}
}

func TestRoutingProfileNamesAndDelete(t *testing.T) {
	cfg := Defaults()
	firstID := cfg.ActiveRoutingProfileID
	secondID, err := cfg.AddRoutingProfile("Работа")
	if err != nil {
		t.Fatal(err)
	}
	thirdID, err := cfg.AddRoutingProfile("Работа")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RoutingProfile(secondID).Name == cfg.RoutingProfile(thirdID).Name {
		t.Fatalf("duplicate names: %+v", cfg.RoutingProfiles)
	}
	if name, ok := cfg.RenameRoutingProfile(secondID, "  "); ok || name != "" {
		t.Fatal("empty rename was accepted")
	}
	if name, ok := cfg.RenameRoutingProfile(secondID, "Личная"); !ok || name != "Личная" {
		t.Fatalf("rename = %q %v", name, ok)
	}
	if name, ok := cfg.RenameRoutingProfile(thirdID, "Личная"); !ok || name == "Личная" {
		t.Fatalf("duplicate rename was not made unique: %q", name)
	}
	if !cfg.DeleteRoutingProfile(firstID) {
		t.Fatal("active profile deletion failed")
	}
	if cfg.ActiveRoutingProfileID == firstID || len(cfg.RoutingProfiles) != 2 {
		t.Fatalf("active after delete = %q profiles=%d", cfg.ActiveRoutingProfileID, len(cfg.RoutingProfiles))
	}
	if !cfg.DeleteRoutingProfile(cfg.ActiveRoutingProfileID) {
		t.Fatal("second delete failed")
	}
	if cfg.DeleteRoutingProfile(cfg.ActiveRoutingProfileID) {
		t.Fatal("the last profile was deleted")
	}
	if len(cfg.RoutingProfiles) != 1 {
		t.Fatalf("profiles = %d", len(cfg.RoutingProfiles))
	}
}

func TestRoutingProfileLimit(t *testing.T) {
	cfg := Defaults()
	for len(cfg.RoutingProfiles) < MaxRoutingProfiles {
		if _, err := cfg.AddRoutingProfile("Профиль"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := cfg.AddRoutingProfile("Лишний"); err == nil {
		t.Fatal("the profile limit was not enforced")
	}
}

func TestSaveLoadRoutingProfiles(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)
	cfg := Defaults()
	secondID, err := cfg.AddRoutingProfile("Работа")
	if err != nil {
		t.Fatal(err)
	}
	cfg.SelectRoutingProfile(secondID)
	cfg.Routing.Mode = ModeDirect
	cfg.Routing.DirectDomains = []string{"corp.example"}

	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ActiveRoutingProfileID != secondID {
		t.Fatalf("active = %q", loaded.ActiveRoutingProfileID)
	}
	if profile := loaded.RoutingProfile(secondID); profile == nil || profile.Name != "Работа" || profile.Routing.Mode != ModeDirect {
		t.Fatalf("active profile = %+v", profile)
	}
	if len(loaded.RoutingProfiles) != 2 {
		t.Fatalf("profiles = %+v", loaded.RoutingProfiles)
	}
}

func TestNormalizeRepairsTUNSettings(t *testing.T) {
	cfg := Defaults()
	cfg.TUN.InterfaceName = "qbox/x"
	cfg.TUN.MTU = 100
	cfg.Normalize()
	if cfg.TUN.InterfaceName != Defaults().TUN.InterfaceName {
		t.Fatalf("interface name = %q", cfg.TUN.InterfaceName)
	}
	if cfg.TUN.MTU != Defaults().TUN.MTU {
		t.Fatalf("mtu = %d", cfg.TUN.MTU)
	}

	cfg = Defaults()
	cfg.TUN.DisableIPv6 = true
	cfg.TUN.MTU = 1000
	cfg.Normalize()
	if cfg.TUN.MTU != 1000 {
		t.Fatalf("ipv4-only mtu = %d", cfg.TUN.MTU)
	}

	cfg = Defaults()
	cfg.TUN.MTU = 1000
	cfg.Normalize()
	if cfg.TUN.MTU != Defaults().TUN.MTU {
		t.Fatalf("mtu with ipv6 = %d", cfg.TUN.MTU)
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

func TestOversizeConfigFallsBackToBackup(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() {
		// Windows Defender may briefly hold the large sparse file, making
		// the testing package's own TempDir cleanup flaky.
		for i := 0; i < 20; i++ {
			if err := os.RemoveAll(dir); err == nil || os.IsNotExist(err) {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
	store, _ := NewStore(dir)
	cfg := Defaults()
	cfg.LogLevel = "debug"
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.LogLevel = "trace"
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}

	file, err := os.Create(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxConfigSize + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	_ = file.Close()

	loaded, err := store.Load()
	if err == nil {
		t.Fatal("expected a warning for the oversized config")
	}
	if loaded.LogLevel != "debug" {
		t.Fatalf("backup not used, log level = %q", loaded.LogLevel)
	}
}

func TestSaveKeepsGoodBackupWhenPrimaryCorrupt(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)
	cfg := Defaults()
	cfg.LogLevel = "debug"
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.LogLevel = "trace"
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.LogLevel = "info"
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}

	backupData, err := os.ReadFile(filepath.Join(dir, "config.json.bak"))
	if err != nil {
		t.Fatal(err)
	}
	var backup Config
	if err := json.Unmarshal(backupData, &backup); err != nil {
		t.Fatal(err)
	}
	if backup.LogLevel != "debug" {
		t.Fatalf("backup was clobbered: %q", backup.LogLevel)
	}
}

func TestCloneIsDeep(t *testing.T) {
	cfg := Defaults()
	cfg.RoutingProfiles[0].Routing.DirectDomains = []string{"corp.example"}
	cfg.Subscriptions = []Subscription{{ID: "a", Nodes: []Node{{Tag: "n", Outbound: json.RawMessage(`{"a":1}`)}}}}
	clone := cfg.Clone()
	clone.Subscriptions[0].Nodes[0].Tag = "changed"
	clone.Subscriptions[0].Nodes[0].Outbound[2] = '9'
	clone.RoutingProfiles[0].Routing.DirectDomains[0] = "changed.example"
	if cfg.Subscriptions[0].Nodes[0].Tag != "n" {
		t.Fatal("tag was shared")
	}
	if string(cfg.Subscriptions[0].Nodes[0].Outbound) != `{"a":1}` {
		t.Fatal("raw outbound was shared")
	}
	if cfg.RoutingProfiles[0].Routing.DirectDomains[0] != "corp.example" {
		t.Fatal("routing profile was shared")
	}
}

func TestCleanList(t *testing.T) {
	got := CleanList([]string{" a ", "", "# comment", "a", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("CleanList = %#v", got)
	}
}
