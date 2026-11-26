package main

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"qbox/internal/app"
	"qbox/internal/config"
	"qbox/internal/core"
	"qbox/internal/platform"
	"qbox/internal/rulesets"
	"qbox/internal/subscription"
)

func main() {
	mygo.App.SetName("qbox")

	if pid, ok := waitPIDArg(); ok {
		// An elevated relaunch must not forward its command line to the old,
		// unequal instance: wait for it to release the single-instance lock.
		_ = platform.WaitForProcessExit(pid, 10*time.Second)
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}

	dir, err := config.DefaultDir()
	if err != nil {
		log.Fatal("каталог конфигурации: ", err)
	}
	store, err := config.NewStore(dir)
	if err != nil {
		log.Fatal("хранилище конфигурации: ", err)
	}
	cfg, cfgErr := store.Load()

	cacheDir, err := config.CacheDir()
	if err != nil {
		cacheDir = filepath.Join(dir, "cache")
	}
	logDir, err := config.LogDir()
	if err != nil {
		logDir = filepath.Join(dir, "logs")
	}

	engine := core.New(core.Options{
		CacheFile: filepath.Join(cacheDir, "cache.db"),
		LogFile:   filepath.Join(logDir, "qbox.log"),
	})
	defer engine.Close()

	rules := rulesets.New(filepath.Join(cacheDir, "rulesets"))

	resourcesDir, _ := mygo.App.Path(mygo.PathResources)

	deps := app.Deps{
		Config:       cfg,
		Store:        store,
		Engine:       engine,
		Fetcher:      subscription.NewClient(),
		RuleSets:     rules,
		SystemProxy:  platform.NewSystemProxy(filepath.Join(dir, "system-proxy.json")),
		ResourcesDir: resourcesDir,
		ConfigPath:   store.Path(),
		CacheDir:     cacheDir,
		Autostart:    hasArg("--autostart"),
	}
	if cfgErr != nil {
		deps.ConfigWarning = cfgErr.Error()
	}

	instance := app.New(deps)

	mygo.App.OnSecondInstance(func([]string, string) {
		instance.ShowWindow()
	})
	mygo.App.WhenReady(instance.Open)
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		instance.BeforeQuit()
	})
	if err := mygo.App.Run(); err != nil {
		instance.Shutdown()
		log.Fatal(err)
	}
}

func hasArg(want string) bool {
	for _, arg := range os.Args[1:] {
		if arg == want {
			return true
		}
	}
	return false
}

func waitPIDArg() (int, bool) {
	const prefix = "--wait-pid="
	for _, arg := range os.Args[1:] {
		if !strings.HasPrefix(arg, prefix) {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimPrefix(arg, prefix))
		if err == nil && pid > 0 {
			return pid, true
		}
	}
	return 0, false
}
