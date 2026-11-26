package app

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"qbox/internal/model"
)

func (a *App) initTray() {
	icon := readTrayIcon(a.deps.ResourcesDir)
	if icon == nil {
		a.notice = "Трей недоступен: не найден resources/tray.png"
		return
	}

	connect := &mygo.MenuItem{
		ID:    "connect",
		Label: "Подключиться",
		Click: func(*mygo.MenuItem, *mygo.Window) { a.trayToggle() },
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{
			Label: "Открыть qbox",
			Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() },
		},
		mygo.Separator(),
		connect,
		mygo.Separator(),
		{
			Label: "Выход",
			Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() },
		},
	})

	tray, err := mygo.NewTray(mygo.TrayOptions{
		ID:      "qbox",
		Icon:    icon,
		ToolTip: "qbox",
		Menu:    menu,
	})
	if err != nil {
		a.notice = "Трей недоступен: " + err.Error()
		return
	}
	a.tray = tray
	a.trayConnect = connect
	a.updateTray()
}

func readTrayIcon(resourcesDir string) []byte {
	candidates := make([]string, 0, 4)
	if resourcesDir != "" {
		candidates = append(candidates,
			filepath.Join(resourcesDir, "tray.png"),
			filepath.Join(resourcesDir, "icon.png"),
		)
	}
	candidates = append(candidates,
		filepath.Join("resources", "tray.png"),
		filepath.Join("resources", "icon.png"),
	)
	for _, path := range candidates {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return data
		}
	}
	return nil
}

func (a *App) trayToggle() {
	if a.busy {
		return
	}
	if a.state == model.StateRunning {
		a.stop()
		return
	}
	a.start()
}

func (a *App) showWindow() {
	if a.win == nil {
		return
	}
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	a.win.Focus()
}

// ShowWindow brings the main window back from the tray.
func (a *App) ShowWindow() {
	a.showWindow()
}

func (a *App) updateTray() {
	if a.tray == nil || a.trayConnect == nil {
		return
	}
	label := "Подключиться"
	enabled := true
	switch a.state {
	case model.StateStarting:
		label = "Подключение…"
		enabled = false
	case model.StateStopping:
		label = "Отключение…"
		enabled = false
	case model.StateRunning:
		label = "Отключиться"
	case model.StateError:
		label = "Повторить"
	}
	a.trayConnect.SetLabel(label)
	a.trayConnect.SetEnabled(enabled)

	tip := "qbox — " + a.stateText()
	if a.state == model.StateRunning && a.cfg.SelectedNode != "" && a.cfg.SelectedNode != "auto" {
		tip += " · " + a.cfg.SelectedNode
	}
	a.tray.SetToolTip(tip)
}
