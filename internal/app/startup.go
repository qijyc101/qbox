package app

import "github.com/egoist/mygo"

// LoginItem registers qbox to start when the user logs in.
type LoginItem interface {
	Set(enabled bool) error
}

type mygoLoginItem struct{}

func (mygoLoginItem) Set(enabled bool) error {
	return mygo.App.SetOpenAtLogin(enabled)
}

func (a *App) setLaunchAtLogin(enabled bool) {
	if a.cfg.Startup.LaunchAtLogin == enabled {
		return
	}
	if err := a.deps.LoginItem.Set(enabled); err != nil {
		a.err = "Не удалось изменить автозапуск: " + err.Error()
		a.invalidate()
		return
	}
	a.cfg.Startup.LaunchAtLogin = enabled
	a.err = ""
	if enabled {
		a.notice = "qbox будет запускаться при входе в систему"
	} else {
		a.notice = "qbox больше не запускается при входе в систему"
	}
	a.scheduleSave()
	a.invalidate()
}

func (a *App) setAutoConnect(enabled bool) {
	if a.cfg.Startup.AutoConnect == enabled {
		return
	}
	a.cfg.Startup.AutoConnect = enabled
	a.err = ""
	if enabled {
		a.notice = "qbox будет подключаться при запуске"
	} else {
		a.notice = "Автоподключение при запуске выключено"
	}
	a.scheduleSave()
	a.invalidate()
}

// shouldAutoConnect reports whether this launch must start the tunnel. The
// explicit --autostart flag comes from an elevated relaunch after the user
// already asked to connect.
func (a *App) shouldAutoConnect() bool {
	return a.deps.Autostart || a.cfg.Startup.AutoConnect
}
