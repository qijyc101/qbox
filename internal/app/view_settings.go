package app

import (
	"github.com/egoist/mygo/ui"

	"qbox/internal/config"
	"qbox/internal/model"
)

func (a *App) settingsPage(c *ui.Context, d design) {
	t := d.T
	ui.Column(c).Grow(1).Background(t.Background).Children(func() {
		ui.Row(c).Padding(12, 12, 4, 12).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Настройки").FontSize(20).Bold()
			ui.Text(c, "").Grow(1)
			if a.dirty && a.state == model.StateRunning {
				iconButton(c, "Применить и перезапустить", iconRotateCW, true).OnClick(a.restartNow)
			}
		})
		ui.Scroll(c).Grow(1).Background(t.Background).Padding(12, 12, 16, 12).Gap(12).Children(func() {
			a.settingsCard(c, d, iconPalette, "Внешний вид", func() {
				a.themeRow(c, d)
				a.accentRow(c, d)
				a.cornersRow(c, d)
			})
			a.settingsCard(c, d, iconNetwork, "Подключение", func() {
				tun := a.cfg.TUN.Enabled
				a.settingsSwitch(c, d, "Режим TUN",
					"TUN перехватывает весь трафик и требует прав администратора. Без TUN qbox использует системный прокси Windows.",
					&tun, func() { a.setTUNEnabled(tun) })
				if a.cfg.TUN.Enabled {
					a.tunSettings(c, d)
				} else {
					a.proxySettings(c, d)
				}
				a.dnsSettings(c, d)
			})
			a.settingsCard(c, d, iconPower, "Запуск", func() {
				launch := a.cfg.Startup.LaunchAtLogin
				a.settingsSwitch(c, d, "Запускать qbox при входе в Windows",
					"Добавляет qbox в автозапуск текущего пользователя.",
					&launch, func() { a.setLaunchAtLogin(launch) })
				connect := a.cfg.Startup.AutoConnect
				a.settingsSwitch(c, d, "Подключаться при запуске qbox",
					"Сразу запускает выбранную ноду после открытия. Для TUN Windows запросит права администратора.",
					&connect, func() { a.setAutoConnect(connect) })
			})
			a.settingsCard(c, d, iconActivity, "Пинг", func() {
				auto := a.cfg.Ping.Auto
				a.settingsSwitch(c, d, "Автоматический пинг",
					"Каждые 30 секунд, пока qbox запущен.",
					&auto, func() { a.setAutoPing(auto) })
				last := "ещё не выполнялся"
				if !a.lastPingAt.IsZero() {
					last = a.lastPingAt.Format("02.01.2006 15:04:05")
				}
				ui.Text(c, "Последний пинг: "+last).FontSize(11).TextColor(t.TextMuted)
			})
			a.settingsCard(c, d, iconScrollText, "Журнал", func() {
				level := a.logLevelIndex()
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Gap(2).Grow(1).Children(func() {
						ui.Text(c, "Уровень логирования")
						ui.Text(c, "Подробные уровни нужны только для диагностики.").FontSize(11).TextColor(t.TextMuted)
					})
					ui.Segmented(c, &level, "Info", "Debug", "Trace").OnChange(func() {
						a.setLogLevel(level)
					})
				})
			})
			a.settingsCard(c, d, iconInfo, "О программе", func() {
				ui.Text(c, "qbox · GPLv3 · sing-box v1.14.4 · MyGo").FontSize(11).TextColor(t.TextMuted)
			})
			if a.dirty && a.state == model.StateRunning {
				ui.Text(c, "Есть изменения, которые вступят в силу после перезапуска подключения.").
					FontSize(11).TextColor(t.Warning).Padding(0, 2)
			}
		})
	})
}

func (a *App) settingsCard(c *ui.Context, d design, icon *ui.SVG, title string, build func()) {
	ui.Column(c).FillWidth().Padding(14).Gap(10).Background(d.T.Surface).Radius(d.RadiusL).Border(1, d.T.Border).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon).Size(13, 13).TextColor(d.T.TextMuted)
			ui.Text(c, title).FontSize(11).Bold().TextColor(d.T.TextMuted)
		})
		build()
	})
}

func (a *App) settingsSwitch(c *ui.Context, d design, title, hint string, value *bool, onChange func()) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, title)
			if hint != "" {
				ui.Text(c, hint).FontSize(11).TextColor(d.T.TextMuted).Wrap()
			}
		})
		ui.Switch(c, value).Label(title).OnChange(onChange)
	})
}

func (a *App) themeRow(c *ui.Context, d design) {
	theme := a.themeIndex()
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Тема").Grow(1)
		ui.Segmented(c, &theme, "Системная", "Светлая", "Тёмная").OnChange(func() {
			a.setThemeIndex(theme)
		})
	})
}

func (a *App) accentRow(c *ui.Context, d design) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Акцент").Grow(1)
		ui.Row(c).Gap(6).Children(func() {
			for _, def := range accentDefs {
				selected := a.cfg.Appearance.Accent == def.Name
				swatch := ui.ButtonBase(c).Size(26, 26).Radius(13).Background(accentColor(def.Name, d.T.Dark)).Center().
					Label(def.Label).Tooltip(def.Label)
				if selected {
					swatch.Border(2, d.T.Text)
				}
				swatch.Children(func() {
					if selected {
						ui.Icon(c, iconCheck).Size(11, 11).TextColor(ui.Hex("#ffffff"))
					}
				})
				if swatch.Clicked() {
					a.setAccent(def.Name)
				}
			}
		})
	})
}

func (a *App) cornersRow(c *ui.Context, d design) {
	corners := a.cornersIndex()
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Скругления").Grow(1)
		ui.Segmented(c, &corners, "Компактные", "Мягкие", "Крупные").OnChange(func() {
			a.setCornersIndex(corners)
		})
	})
}

func (a *App) tunSettings(c *ui.Context, d design) {
	t := d.T
	ui.Box(c).Height(1).Background(t.Border)
	stack := a.stackIndex()
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, "Стек")
			ui.Text(c, "Mixed требует сборки с with_gvisor; System быстрее, gVisor универсальнее.").FontSize(11).TextColor(t.TextMuted).Wrap()
		})
		ui.Segmented(c, &stack, "System", "Mixed", "gVisor").OnChange(func() {
			a.setStack(stack)
		})
	})
	ui.Text(c, "Имя интерфейса").FontSize(11).TextColor(t.TextMuted)
	ui.TextInput(c, &a.cfg.TUN.InterfaceName).Label("Имя интерфейса").OnChange(a.connectionSettingsChanged)
	ui.Text(c, "MTU").FontSize(11).TextColor(t.TextMuted)
	mtu := float64(a.cfg.TUN.MTU)
	if ui.NumberInput(c, &mtu, 576, 65535, 1).Label("MTU").Changed() {
		a.setMTU(int(mtu))
	}
	a.settingsSwitch(c, d, "Strict route",
		"Блокирует утечки DNS и обход TUN. Может ломать отдельные приложения.",
		&a.cfg.TUN.StrictRoute, a.connectionSettingsChanged)
	a.settingsSwitch(c, d, "Отключить IPv6 в TUN",
		"Включайте, если Windows не может назначить IPv6-адрес TUN-интерфейсу.",
		&a.cfg.TUN.DisableIPv6, a.connectionSettingsChanged)
	if a.cfg.DNS.UseProxy && !a.cfg.TUN.StrictRoute && !a.cfg.TUN.DisableIPv6 && a.cfg.Routing.Mode != config.ModeDirect {
		ui.Text(c, "Strict route выключен: Windows может использовать DNS физических адаптеров в обход туннеля.").
			FontSize(11).TextColor(t.Warning).Wrap()
	}
	if !a.elevated() {
		ui.Text(c, "При включении TUN Windows запросит права администратора.").FontSize(11).TextColor(t.TextMuted)
	}
}

func (a *App) proxySettings(c *ui.Context, d design) {
	t := d.T
	ui.Box(c).Height(1).Background(t.Border)
	ui.Text(c, "Порт прокси").FontSize(11).TextColor(t.TextMuted)
	port := float64(a.cfg.Proxy.Port)
	if ui.NumberInput(c, &port, 0, 65535, 1).Label("Порт прокси").Changed() {
		a.setProxyPort(int(port))
	}
	ui.Text(c, "0 — выбрать свободный порт автоматически.").FontSize(11).TextColor(t.TextMuted)
	address := a.proxyAddr
	if address == "" {
		address = "будет выбран при запуске"
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Локальный прокси: "+address).FontSize(12).Grow(1)
		if a.proxyAddr != "" {
			iconButton(c, "Копировать", iconCopy, false).OnClick(func() { a.copyText(a.proxyAddr) })
		}
	})
	ui.Text(c, "Системный прокси видят не все приложения: QUIC, игры и программы со своим DNS могут обойти его. "+
		"Корпоративные домены, которые приложение резолвит само, могут не работать. Для полного перехвата включите TUN.").
		FontSize(11).TextColor(t.Warning).Wrap()
	if a.state == model.StateRunning && !a.cfg.TUN.Enabled && a.proxyAddr == "" {
		ui.Text(c, "Прокси ещё не включён: перезапустите подключение.").FontSize(11).TextColor(t.TextMuted)
	}
}

func (a *App) dnsSettings(c *ui.Context, d design) {
	t := d.T
	ui.Box(c).Height(1).Background(t.Border)
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconGlobe).Size(13, 13).TextColor(t.TextMuted)
		ui.Text(c, "DNS").FontSize(13).Bold()
	})
	ui.Text(c, "DoH/DoT сервер").FontSize(11).TextColor(t.TextMuted)
	ui.TextInput(c, &a.cfg.DNS.Remote).Label("DoH/DoT сервер").Placeholder("https://1.1.1.1/dns-query").
		OnChange(a.connectionSettingsChanged)
	a.settingsSwitch(c, d, "DNS через прокси",
		"В режиме Direct используется системный DNS. Direct-домены всегда резолвятся системным сервером.",
		&a.cfg.DNS.UseProxy, a.connectionSettingsChanged)
}
