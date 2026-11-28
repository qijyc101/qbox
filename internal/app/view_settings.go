package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"qbox/internal/config"
	"qbox/internal/model"
)

func (a *App) settingsPage(c *ui.Context, d design) {
	ui.Column(c).Grow(1).Background(d.T.Background).Children(func() {
		ui.Row(c).Padding(spaceXL, spaceXL, spaceXS, spaceXL).Gap(spaceL).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Настройки").FontSize(fontSizePageTitle).Bold()
			ui.Spacer(c)
			if a.needsRestart() {
				a.restartButton(c, "Применить и перезапустить")
			}
		})
		ui.Scroll(c).Grow(1).Background(d.T.Background).
			Padding(spaceXL, spaceXL, space2XL, spaceXL).Gap(spaceXL).Children(func() {
			baseCard(c, d, iconPalette, "Внешний вид", func() {
				a.themeRow(c, d)
				a.accentRow(c, d)
				a.cornersRow(c, d)
			})
			baseCard(c, d, iconNetwork, "Подключение", func() {
				tun := a.cfg.TUN.Enabled
				switchRow(c, d, "Режим TUN",
					"TUN перехватывает весь трафик и требует прав администратора. Без TUN qbox использует системный прокси.",
					&tun, func() { a.setTUNEnabled(tun) })
				if a.cfg.TUN.Enabled {
					a.tunSettings(c, d)
				} else {
					a.proxySettings(c, d)
				}
				a.dnsSettings(c, d)
			})
			baseCard(c, d, iconPower, "Запуск", func() {
				launch := a.cfg.Startup.LaunchAtLogin
				switchRow(c, d, "Запускать qbox при входе в систему",
					"Добавляет qbox в автозапуск текущего пользователя.",
					&launch, func() { a.setLaunchAtLogin(launch) })
				connect := a.cfg.Startup.AutoConnect
				switchRow(c, d, "Подключаться при запуске qbox",
					"Сразу запускает выбранную ноду после открытия. Для TUN потребуются права администратора.",
					&connect, func() { a.setAutoConnect(connect) })
			})
			baseCard(c, d, iconActivity, "Пинг", func() {
				auto := a.cfg.Ping.Auto
				switchRow(c, d, "Автоматический пинг",
					fmt.Sprintf("Каждые %d секунд, пока qbox запущен.", int(autoPingInterval.Seconds())),
					&auto, func() { a.setAutoPing(auto) })
				last := "ещё не выполнялся"
				if !a.lastPingAt.IsZero() {
					last = a.lastPingAt.Format(dateTimeLayout)
				}
				hintText(c, d, "Последний пинг: "+last)
			})
			baseCard(c, d, iconScrollText, "Журнал", func() {
				level := a.logLevelIndex()
				ui.Row(c).Gap(spaceXL).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Gap(spaceXXS).Grow(1).Children(func() {
						ui.Text(c, "Уровень логирования")
						hintText(c, d, "Подробные уровни нужны только для диагностики.")
					})
					ui.Segmented(c, &level, "Info", "Debug", "Trace").OnChange(func() {
						a.setLogLevel(level)
					})
				})
			})
			baseCard(c, d, iconInfo, "О программе", func() {
				hintText(c, d, "qbox · GPLv3 · sing-box v1.14.4 · MyGo")
			})
			a.restartHint(c, d)
		})
	})
}

func (a *App) themeRow(c *ui.Context, d design) {
	theme := a.themeIndex()
	ui.Row(c).Gap(spaceXL).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Тема").Grow(1)
		ui.Segmented(c, &theme, "Системная", "Светлая", "Тёмная").OnChange(func() {
			a.setThemeIndex(theme)
		})
	})
}

func (a *App) accentRow(c *ui.Context, d design) {
	ui.Row(c).Gap(spaceXL).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Акцент").Grow(1)
		ui.Row(c).Gap(spaceS).Children(func() {
			for _, def := range accentDefs {
				selected := a.cfg.Appearance.Accent == def.Name
				swatch := ui.ButtonBase(c).Size(menuButtonSize, menuButtonSize).Radius(menuButtonSize / 2).
					Background(accentColor(def.Name, d.T.Dark)).Center().
					Label(def.Label).Tooltip(def.Label)
				if selected {
					swatch.Border(selectionBorder, d.T.Text)
				}
				swatch.Children(func() {
					if selected {
						ui.Icon(c, iconCheck).Size(iconSizeS, iconSizeS).TextColor(ui.Hex(colorWhite))
					}
				})
				swatch.OnClick(func() { a.setAccent(def.Name) })
			}
		})
	})
}

func (a *App) cornersRow(c *ui.Context, d design) {
	corners := a.cornersIndex()
	ui.Row(c).Gap(spaceXL).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Скругления").Grow(1)
		ui.Segmented(c, &corners, "Компактные", "Мягкие", "Крупные").OnChange(func() {
			a.setCornersIndex(corners)
		})
	})
}

func (a *App) tunSettings(c *ui.Context, d design) {
	ui.Box(c).Height(dividerSize).Background(d.T.Border)
	stack := a.stackIndex()
	ui.Row(c).Gap(spaceXL).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(spaceXXS).Grow(1).Children(func() {
			ui.Text(c, "Стек")
			hintText(c, d, "Mixed требует сборки с with_gvisor; System быстрее, gVisor универсальнее.")
		})
		ui.Segmented(c, &stack, "System", "Mixed", "gVisor").OnChange(func() {
			a.setStack(stack)
		})
	})
	fieldLabel(c, d, "Имя интерфейса")
	ui.TextInput(c, &a.cfg.TUN.InterfaceName).Label("Имя интерфейса").
		OnChange(a.connectionSettingsChanged)
	fieldLabel(c, d, "MTU")
	mtu := float64(a.cfg.TUN.MTU)
	if ui.NumberInput(c, &mtu, config.MinMTU, config.MaxMTU, numberStep).Label("MTU").Changed() {
		a.setMTU(int(mtu))
	}
	switchRow(c, d, "Strict route",
		"Блокирует утечки DNS и обход TUN. Может ломать отдельные приложения.",
		&a.cfg.TUN.StrictRoute, a.connectionSettingsChanged)
	switchRow(c, d, "Отключить IPv6 в TUN",
		"Включайте, если система не может назначить IPv6-адрес TUN-интерфейсу.",
		&a.cfg.TUN.DisableIPv6, a.connectionSettingsChanged)
	if a.cfg.DNS.UseProxy && !a.cfg.TUN.StrictRoute && !a.cfg.TUN.DisableIPv6 && a.cfg.Routing.Mode != config.ModeDirect {
		warningText(c, d, "Strict route выключен: система может использовать DNS физических адаптеров в обход туннеля.")
	}
	if !a.elevated() {
		hintText(c, d, "При включении TUN система запросит права администратора.")
	}
}

func (a *App) proxySettings(c *ui.Context, d design) {
	ui.Box(c).Height(dividerSize).Background(d.T.Border)
	fieldLabel(c, d, "Порт прокси")
	port := float64(a.cfg.Proxy.Port)
	if ui.NumberInput(c, &port, 0, config.MaxPort, numberStep).Label("Порт прокси").Changed() {
		a.setProxyPort(int(port))
	}
	hintText(c, d, "0 — выбрать свободный порт автоматически.")
	address := a.proxyAddr
	if address == "" {
		address = "будет выбран при запуске"
	}
	ui.Row(c).Gap(spaceM).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Локальный прокси: "+address).FontSize(fontSizeBody).Grow(1)
		if a.proxyAddr != "" {
			iconButton(c, "Копировать", iconCopy, false).OnClick(func() { a.copyText(a.proxyAddr) })
		}
	})
	warningText(c, d, "Системный прокси видят не все приложения: QUIC, игры и программы со своим DNS могут обойти его. "+
		"Корпоративные домены, которые приложение резолвит само, могут не работать. Для полного перехвата включите TUN.")
	if a.state == model.StateRunning && !a.cfg.TUN.Enabled && a.proxyAddr == "" {
		hintText(c, d, "Прокси ещё не включён: перезапустите подключение.")
	}
}

func (a *App) dnsSettings(c *ui.Context, d design) {
	ui.Box(c).Height(dividerSize).Background(d.T.Border)
	ui.Row(c).Gap(spaceS).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconGlobe).Size(iconSizeL, iconSizeL).TextColor(d.T.TextMuted)
		ui.Text(c, "DNS").FontSize(fontSizeLabel).Bold()
	})
	baseInput(c, d, "DoH/DoT сервер", "https://1.1.1.1/dns-query", &a.cfg.DNS.Remote, a.connectionSettingsChanged)
	switchRow(c, d, "DNS через прокси",
		"В режиме Direct используется системный DNS. Direct-домены всегда резолвятся системным сервером.",
		&a.cfg.DNS.UseProxy, a.connectionSettingsChanged)
}
