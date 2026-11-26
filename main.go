package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"qbox/internal/app"
	"qbox/internal/config"
	"qbox/internal/core"
	"qbox/internal/rulesets"
	"qbox/internal/subscription"
)

func main() {
	mygo.App.SetName("qbox")

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
		ResourcesDir: resourcesDir,
		ConfigPath:   store.Path(),
		CacheDir:     cacheDir,
		Autostart:    hasArg("--autostart"),
	}
	if cfgErr != nil {
		deps.ConfigWarning = cfgErr.Error()
	}

	instance := app.New(deps)

	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.WhenReady(instance.Open)
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		instance.Shutdown()
	})
	if err := mygo.App.Run(); err != nil {
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
