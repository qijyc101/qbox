package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"qbox/internal/config"
	"qbox/internal/model"
)

func (a *App) view(c *ui.Context) {
	d := a.prepare(c)
	ui.Column(c).Fill().Background(d.T.Background).Children(func() {
		a.titleBar(c, d)
		a.statusBanner(c, d)
		if a.settingsOpen {
			a.settingsPage(c, d)
		} else {
			a.homePage(c, d)
		}
	})
}

func (a *App) titleBar(c *ui.Context, d design) {
	bar := c.TitleBar()
	height := max(bar.Height, float32(titleBarMinHeight))
	ui.Column(c).Background(d.T.Background).Children(func() {
		ui.Row(c).Height(height).Background(d.T.Background).Padding(0, bar.Right, 0, bar.Left).DragWindow().Children(func() {
			ui.Row(c).Gap(2).PaddingStart(8).AlignItems(ui.Center).Children(func() {
				a.fileMenu(c, d)
				a.routingMenu(c, d)
				a.settingsButton(c, d)
			})
			ui.Text(c, "").Grow(1)
			a.windowControls(c, d, height)
		})
		ui.Box(c).Height(1).Background(d.T.Border)
	})
}

func (a *App) windowControls(c *ui.Context, d design, height float32) {
	ui.Row(c).AlignItems(ui.Stretch).Children(func() {
		a.windowButton(c, d, iconMinus, "Свернуть", height, false, func() {
			if a.win != nil {
				a.win.Minimize()
				a.invalidate()
			}
		})
		if a.win != nil && a.win.IsMaximized() {
			a.windowButton(c, d, iconCopy, "Восстановить", height, false, func() {
				if a.win != nil {
					a.win.Unmaximize()
					a.invalidate()
				}
			})
		} else {
			a.windowButton(c, d, iconSquare, "Развернуть", height, false, func() {
				if a.win != nil {
					a.win.Maximize()
					a.invalidate()
				}
			})
		}
		a.windowButton(c, d, iconX, "Закрыть", height, true, func() {
			if a.win != nil {
				a.win.Close()
			}
		})
	})
}

func (a *App) windowButton(c *ui.Context, d design, svg *ui.SVG, label string, height float32, close bool, onClick func()) {
	btn := ui.ButtonBase(c).Width(windowControlWidth).Height(height).Center().Label(label).Tooltip(label)
	switch {
	case close && btn.Hovered():
		btn.Background(ui.Hex("#e81123"))
	case btn.Hovered():
		btn.Background(d.T.SurfaceHover)
	}
	btn.Children(func() {
		color := d.T.Text
		if close && btn.Hovered() {
			color = ui.Hex("#ffffff")
		}
		ui.Icon(c, svg).Size(14, 14).TextColor(color)
	})
	if btn.Clicked() {
		onClick()
	}
}

func (a *App) menuTrigger(c *ui.Context, d design, label string, svg *ui.SVG, open *bool) ui.Element {
	trigger := ui.ButtonBase(c).Padding(6, 10).Gap(6).Radius(d.RadiusS)
	if *open || trigger.Hovered() {
		trigger.Background(d.T.SurfaceHover)
	}
	if *open {
		trigger.TextColor(d.T.Accent)
	}
	trigger.Children(func() {
		ui.Icon(c, svg).Size(14, 14).TextColor(d.T.TextMuted)
		ui.Text(c, label).FontSize(13)
		ui.Icon(c, iconChevronDown).Size(11, 11).TextColor(d.T.TextMuted)
	})
	return trigger
}

func (a *App) menuOptions(d design, open *bool) ui.PopoverMenuButtonOptions {
	return ui.PopoverMenuButtonOptions{
		At:   ui.AnchorBottomStart,
		Self: ui.AnchorTopStart,
		Open: open,
		Panel: func(panel ui.Element) {
			panel.Padding(4).Radius(d.RadiusM).Background(d.T.Background).Border(1, d.T.Border)
		},
		Item: func(item ui.Element) {
			item.Padding(6, 10).Radius(d.RadiusS)
			if item.Highlighted() {
				item.Background(d.T.Accent)
				item.TextColor(d.T.AccentText)
			}
		},
	}
}

func (a *App) fileMenu(c *ui.Context, d design) {
	trigger := a.menuTrigger(c, d, "Файл", iconFileJson, &a.fileMenuOpen)
	trigger.PopoverMenuButton(func(m *ui.Menu) {
		if m.Item("config.json").Icon(menuIconFile).Subtitle("настройки qbox").Chosen() {
			a.openTextViewer(viewerConfig)
		}
		if m.Item("Конфигурация sing-box").Icon(menuIconRoute).Subtitle("последняя применённая").Chosen() {
			a.openTextViewer(viewerSingBox)
		}
		if m.Item("Журнал qbox").Icon(menuIconScroll).Subtitle("qbox.log").Chosen() {
			a.openTextViewer(viewerLog)
		}
		m.Separator()
		if m.Item("Выход").Icon(menuIconX).Danger().Chosen() {
			mygo.App.Quit()
		}
	}, a.menuOptions(d, &a.fileMenuOpen))
}

func (a *App) routingMenu(c *ui.Context, d design) {
	trigger := a.menuTrigger(c, d, "Маршрутизация", iconRoute, &a.routingMenuOpen)
	trigger.PopoverMenuButton(func(m *ui.Menu) {
		for i := range a.cfg.RoutingProfiles {
			profile := a.cfg.RoutingProfiles[i]
			active := profile.ID == a.cfg.ActiveRoutingProfileID
			if m.Item(profile.Name).Checked(active).Chosen() {
				a.selectRoutingProfile(profile.ID)
			}
		}
		m.Separator()
		if m.Item("Новый профиль…").Icon(menuIconPlus).Disabled(len(a.cfg.RoutingProfiles) >= config.MaxRoutingProfiles).Chosen() {
			a.openNewProfile()
		}
		if m.Item("Переименовать…").Icon(menuIconPencil).Disabled(a.cfg.ActiveRoutingProfile() == nil).Chosen() {
			a.openRenameProfile()
		}
		if m.Item("Удалить…").Icon(menuIconTrash).Disabled(len(a.cfg.RoutingProfiles) <= 1).Danger().Chosen() {
			a.openDeleteProfile()
		}
		m.Separator()
		if m.Item("Просмотр всех маршрутов…").Icon(menuIconSettings).Chosen() {
			a.openRoutingWindow()
		}
	}, a.menuOptions(d, &a.routingMenuOpen))
}

func (a *App) settingsButton(c *ui.Context, d design) {
	btn := ui.ButtonBase(c).Padding(6, 10).Gap(6).Radius(d.RadiusS)
	if a.settingsOpen || btn.Hovered() {
		btn.Background(d.T.SurfaceHover)
	}
	if a.settingsOpen {
		btn.TextColor(d.T.Accent)
	}
	btn.Children(func() {
		ui.Icon(c, iconSettings).Size(14, 14).TextColor(d.T.TextMuted)
		ui.Text(c, "Настройки").FontSize(13)
	})
	if btn.Clicked() {
		a.settingsOpen = !a.settingsOpen
		a.err = ""
		a.invalidate()
	}
}

func (a *App) statusBanner(c *ui.Context, d design) {
	t := d.T
	if a.err == "" && a.warn == "" && a.notice == "" && !(a.workBusy && a.workTS != "") {
		return
	}
	ui.Column(c).Children(func() {
		if a.err != "" {
			a.bannerRow(c, d, a.err, iconTriangleAlert, t.Danger, func() { a.err = "" })
		}
		if a.warn != "" {
			a.bannerRow(c, d, a.warn, iconTriangleAlert, t.Warning, nil)
		}
		if a.workBusy && a.workTS != "" {
			a.bannerRow(c, d, a.workTS, iconActivity, t.TextMuted, nil)
		}
		if a.notice != "" {
			a.bannerRow(c, d, a.notice, iconInfo, t.TextMuted, func() { a.notice = "" })
		}
	})
}

func (a *App) bannerRow(c *ui.Context, d design, text string, icon *ui.SVG, color ui.Color, dismiss func()) {
	ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).Background(d.T.Surface).Children(func() {
		ui.Icon(c, icon).Size(13, 13).TextColor(color)
		ui.Text(c, text).FontSize(12).TextColor(color).Grow(1).Wrap()
		if dismiss != nil {
			if ui.ButtonBase(c).Size(22, 22).Radius(d.RadiusXS).Center().Label("Скрыть").Children(func() {
				ui.Icon(c, iconX).Size(12, 12).TextColor(d.T.TextMuted)
			}).Clicked() {
				dismiss()
			}
		}
	})
}

func (a *App) homePage(c *ui.Context, d design) {
	ui.Row(c).Grow(1).FillWidth().Background(d.T.Background).Children(func() {
		a.subscriptionsSidebar(c, d)
		ui.Box(c).Width(1).FillHeight().Background(d.T.Border)
		a.contentPanel(c, d)
	})
}

func (a *App) subscriptionsSidebar(c *ui.Context, d design) {
	t := d.T
	ui.Column(c).Width(270).FillHeight().Background(t.Background).Padding(12, 10).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Подписки").FontSize(13).Bold()
			ui.Text(c, "").Grow(1)
			if a.workBusy && a.workTS != "" {
				ui.Text(c, a.workTS).FontSize(11).TextColor(t.TextMuted)
			}
		})
		ui.Scroll(c).Grow(1).Background(t.Background).Gap(2).Children(func() {
			if len(a.cfg.Subscriptions) == 0 {
				ui.Column(c).FillWidth().Padding(12, 8).Gap(4).Children(func() {
					ui.Text(c, "Подписок пока нет").FontSize(13)
					ui.Text(c, "Добавьте base64/URI-список или sing-box JSON.").FontSize(12).TextColor(t.TextMuted).Wrap()
				})
			}
			for i := range a.cfg.Subscriptions {
				a.subscriptionRow(c, d, a.cfg.Subscriptions[i])
			}
		})
		iconButton(c, "Добавить подписку", iconPlus, false).FillWidth().OnClick(a.openAddSubscription)
	})
}

func (a *App) subscriptionRow(c *ui.Context, d design, sub config.Subscription) {
	t := d.T
	active := a.cfg.ActiveSubscription == sub.ID
	row := ui.ButtonBase(c).FillWidth().Padding(6, 8).Radius(d.RadiusM)
	switch {
	case active:
		row.Background(t.Accent.Alpha(0.10))
	case row.Hovered():
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Box(c).Size(32, 32).Radius(d.RadiusS).Background(avatarColor(sub.ID)).Center().Children(func() {
			ui.Text(c, strings.ToUpper(avatarLetter(sub.Name))).FontSize(14).Bold().TextColor(ui.Hex("#ffffff"))
		})
		ui.Row(c).Gap(6).AlignItems(ui.Center).Grow(1).Children(func() {
			if active {
				ui.Box(c).Size(6, 6).Radius(3).Background(t.Accent)
			}
			ui.Text(c, sub.Name).FontSize(13).Grow(1)
		})
		a.subscriptionMenu(c, d, sub)
	})
	if row.Clicked() {
		a.selectSubscription(sub.ID)
	}
}

func (a *App) subscriptionMenu(c *ui.Context, d design, sub config.Subscription) {
	trigger := ui.ButtonBase(c).Size(26, 26).Radius(d.RadiusS).Center()
	if trigger.Hovered() {
		trigger.Background(d.T.SurfaceHover)
	}
	trigger.Children(func() {
		ui.Icon(c, iconEllipsis).Size(15, 15).TextColor(d.T.TextMuted)
	})
	trigger.PopoverMenuButton(func(m *ui.Menu) {
		if m.Item("Обновить").Icon(menuIconRefresh).Disabled(a.workBusy || a.busy).Chosen() {
			a.refreshSubscription(sub.ID)
		}
		m.Separator()
		if m.Item("Переименовать…").Icon(menuIconPencil).Chosen() {
			a.openRenameSubscription(sub.ID)
		}
		if m.Item("Удалить…").Icon(menuIconTrash).Danger().Chosen() {
			a.openDeleteSubscription(sub.ID)
		}
	}, ui.PopoverMenuButtonOptions{
		At:   ui.AnchorBottomEnd,
		Self: ui.AnchorTopEnd,
		Panel: func(panel ui.Element) {
			panel.Padding(4).Radius(d.RadiusM).Background(d.T.Background).Border(1, d.T.Border)
		},
		Item: func(item ui.Element) {
			item.Padding(6, 10).Radius(d.RadiusS)
			if item.Highlighted() {
				item.Background(d.T.Accent)
				item.TextColor(d.T.AccentText)
			}
		},
	})
}

func (a *App) contentPanel(c *ui.Context, d design) {
	ui.Column(c).Grow(1).FillHeight().Background(d.T.Background).Padding(12).Gap(10).Children(func() {
		a.contentToolbar(c, d)
		switch a.panel {
		case 0:
			a.nodesView(c, d)
		case 1:
			a.logsView(c, d)
		case 2:
			a.connectionsView(c, d)
		}
		a.trafficCard(c, d)
	})
}

func (a *App) contentToolbar(c *ui.Context, d design) {
	t := d.T
	tabs := a.panel
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Segmented(c, &tabs, "Ноды", "Логи", "Подключения").Width(260).OnChange(func() {
			a.panel = tabs
			a.refreshConnections()
			a.invalidate()
		})
		ui.Text(c, "").Grow(1)
		if a.pinging {
			iconButton(c, fmt.Sprintf("Отмена (%d/%d)", a.pingDone, a.pingTotal), iconX, false).OnClick(a.cancelPing)
		} else {
			iconButton(c, "Пинг", iconActivity, false).
				Disabled(a.busy || a.workBusy || len(a.cfg.ActiveNodes()) == 0).
				OnClick(a.pingNodes)
		}
		restart := func() ui.Element {
			if a.dirty && a.state == model.StateRunning {
				return iconButton(c, "Перезапустить", iconRotateCW, true).Disabled(a.busy || a.workBusy).OnClick(a.restartNow)
			}
			return iconButton(c, "Перезапустить", iconRotateCW, false).
				Disabled(a.busy || a.workBusy || a.state != model.StateRunning).
				OnClick(a.restartNow)
		}
		restart()
		if a.busy {
			iconButton(c, a.busyTS, iconActivity, false).Disabled(true)
		} else if a.state == model.StateRunning {
			iconButton(c, "Отключиться", iconPowerOff, false).OnClick(a.stop)
		} else {
			label := "Подключиться"
			icon := iconPower
			if a.state == model.StateError {
				label = "Повторить"
				icon = iconRefreshCW
			}
			iconButton(c, label, icon, true).Disabled(a.workBusy).OnClick(a.start)
		}
	})
	if a.dirty && a.state == model.StateRunning {
		ui.Text(c, "Настройки изменены: перезапустите подключение, чтобы применить их.").FontSize(11).TextColor(t.Warning)
	}
}

func (a *App) nodesView(c *ui.Context, d design) {
	t := d.T
	sub := a.activeSub()
	if sub == nil || len(sub.Nodes) == 0 {
		ui.Column(c).Grow(1).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(6).Padding(20).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconServer).Size(24, 24).TextColor(t.TextMuted)
				ui.Text(c, "Нод пока нет").FontSize(15).Bold()
				ui.Text(c, "Выберите подписку слева или добавьте новую.").FontSize(12).TextColor(t.TextMuted).Wrap()
				iconButton(c, "Добавить подписку", iconPlus, false).OnClick(a.openAddSubscription)
			})
		})
		return
	}
	a.syncNodeIndex()
	rows := a.nodeRows()
	a.nodeList.Key = func(i int) any {
		if i >= 0 && i < len(rows) {
			return rows[i].tag
		}
		return i
	}
	a.nodeList.Selected = nil
	ui.List(c, &a.nodeList, len(rows), func(i int) {
		a.nodeRow(c, d, rows[i], i == a.nodeIndex, i)
	}).Grow(1).Background(t.Background)
}

func (a *App) nodeRow(c *ui.Context, d design, row nodeRow, selected bool, index int) {
	t := d.T
	line := ui.ButtonBase(c).FillWidth().Padding(8, 10).Gap(10).Radius(d.RadiusM)
	switch {
	case selected:
		line.Background(t.Accent.Alpha(0.10))
	case line.Hovered():
		line.Background(t.SurfaceHover)
	}
	line.Children(func() {
		if selected {
			ui.Box(c).Width(3).Height(24).Radius(2).Background(t.Accent)
		} else {
			ui.Box(c).Width(3)
		}
		ui.Text(c, strings.ToUpper(row.typ)).FontSize(10).Bold().TextColor(t.Accent).
			Padding(3, 6).Background(t.Accent.Alpha(0.10)).Radius(d.RadiusXS)
		ui.Column(c).Gap(1).Grow(1).Children(func() {
			ui.Text(c, row.title).FontSize(13)
			ui.Text(c, row.detail).FontSize(11).TextColor(t.TextMuted)
		})
		if !row.auto {
			if text, tone := a.latency(row.tag); text != "" {
				color := t.TextMuted
				switch tone {
				case "ok":
					color = t.Success
				case "error":
					color = t.Danger
				}
				ui.Text(c, text).FontSize(12).TextColor(color)
			}
		}
	})
	if line.Clicked() {
		a.selectNode(index)
	}
}

func (a *App) logsView(c *ui.Context, d design) {
	t := d.T
	ui.Column(c).Grow(1).Gap(8).Children(func() {
		filter := a.logView
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Segmented(c, &filter, "Все", "Info", "Warn", "Error").Width(300).OnChange(func() {
				a.logView = filter
			})
			ui.Text(c, "").Grow(1)
			ui.Button(c, "Очистить").OnClick(a.clearLogs)
			ui.Button(c, "Где логи?").OnClick(a.openLogFolder)
		})
		lines := a.logLines()
		a.logList.FollowEnd = true
		ui.List(c, &a.logList, len(lines), func(i int) {
			entry := lines[i]
			color := t.Text
			switch entry.Level {
			case "warn":
				color = t.Warning
			case "error", "fatal", "panic":
				color = t.Danger
			}
			ui.Row(c).Gap(8).Padding(3, 8).Children(func() {
				ui.Text(c, entry.Time.Format("15:04:05")).TextColor(t.TextMuted).FontSize(11)
				ui.Text(c, strings.ToUpper(entry.Level)).TextColor(color).FontSize(11).Width(48)
				ui.Text(c, entry.Message).TextColor(color).FontSize(12).Grow(1)
			})
		}).Grow(1).Background(t.Background).Children(func() {
			if len(lines) == 0 {
				ui.Text(c, "Журнал пуст.").TextColor(t.TextMuted).Padding(12)
			}
		})
	})
}

func (a *App) connectionsView(c *ui.Context, d design) {
	t := d.T
	ui.Column(c).Grow(1).Gap(6).Children(func() {
		ui.Textf(c, "Активных соединений: %d", len(a.connections)).FontSize(11).TextColor(t.TextMuted)
		if len(a.connections) == 0 {
			ui.Column(c).Grow(1).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Gap(4).Padding(20).Children(func() {
					ui.Text(c, "Нет активных соединений").FontSize(13)
					ui.Text(c, "Список обновляется автоматически.").FontSize(12).TextColor(t.TextMuted)
				})
			})
			return
		}
		ui.List(c, &a.connList, len(a.connections), func(i int) {
			conn := a.connections[i]
			row := ui.Row(c).FillWidth().Padding(7, 8).Gap(10).AlignItems(ui.Center).Radius(d.RadiusM)
			if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			row.Children(func() {
				ui.Column(c).Gap(1).Grow(1).Children(func() {
					host := conn.Host
					if host == "" {
						host = conn.Destination
					}
					ui.Text(c, host).FontSize(12)
					parts := make([]string, 0, 4)
					for _, part := range []string{conn.Network, conn.Protocol, conn.Outbound, conn.Rule} {
						if part != "" {
							parts = append(parts, part)
						}
					}
					detail := strings.Join(parts, " · ")
					if detail == "" {
						detail = conn.Destination
					}
					ui.Text(c, detail).FontSize(11).TextColor(t.TextMuted)
				})
				ui.Column(c).Gap(1).AlignItems(ui.End).Children(func() {
					ui.Textf(c, "↑ %s  ↓ %s", formatBytes(conn.Upload), formatBytes(conn.Download)).FontSize(11).TextColor(t.TextMuted)
					ui.Text(c, formatDuration(time.Since(conn.CreatedAt))).FontSize(11).TextColor(t.TextMuted)
				})
			})
		}).Grow(1).Background(t.Background)
	})
}

func (a *App) trafficCard(c *ui.Context, d design) {
	t := d.T
	ui.Column(c).Height(170).Padding(12).Gap(6).Background(t.Surface).Radius(d.RadiusL).Border(1, t.Border).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Трафик").FontSize(13).Bold()
			color := t.TextMuted
			switch a.state {
			case model.StateRunning:
				color = t.Success
			case model.StateError:
				color = t.Danger
			case model.StateStarting, model.StateStopping:
				color = t.Warning
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				switch a.state {
				case model.StateRunning:
					ui.Icon(c, iconCircleCheck).Size(13, 13).TextColor(t.Success)
				case model.StateError:
					ui.Icon(c, iconCircleAlert).Size(13, 13).TextColor(t.Danger)
				case model.StateStarting, model.StateStopping:
					ui.Icon(c, iconActivity).Size(13, 13).TextColor(t.Warning)
				default:
					ui.Box(c).Size(7, 7).Radius(4).Background(color)
				}
				ui.Text(c, a.stateText()).FontSize(12).TextColor(color)
			})
			if !a.cfg.TUN.Enabled && a.proxyAddr != "" && a.state == model.StateRunning {
				ui.Text(c, a.proxyAddr).FontSize(11).TextColor(t.TextMuted)
			}
			ui.Text(c, "").Grow(1)
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconArrowUp).Size(12, 12).TextColor(t.TextMuted)
				ui.Textf(c, "%s", formatBytes(a.stats.Up)).FontSize(12).TextColor(t.TextMuted)
			})
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconArrowDown).Size(12, 12).TextColor(t.Accent)
				ui.Textf(c, "%s", formatBytes(a.stats.Down)).FontSize(12).TextColor(t.Accent)
			})
		})
		ui.Box(c).Grow(1).Draw(func(p *ui.Painter, r ui.Rect) {
			a.drawTrafficChart(p, r, d)
		})
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconArrowDown).Size(10, 10).TextColor(t.Accent)
				ui.Text(c, "принято").FontSize(10).TextColor(t.Accent)
			})
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconArrowUp).Size(10, 10).TextColor(t.TextMuted)
				ui.Text(c, "отдано").FontSize(10).TextColor(t.TextMuted)
			})
			ui.Text(c, "").Grow(1)
			period := "60 секунд"
			if n := len(a.traffic); n > 1 && n < trafficPoints {
				period = fmt.Sprintf("последние %d с", n)
			}
			ui.Text(c, period).FontSize(10).TextColor(t.TextMuted)
		})
	})
}

func (a *App) drawTrafficChart(p *ui.Painter, r ui.Rect, d design) {
	t := d.T
	if r.W <= 2 || r.H <= 2 {
		return
	}
	padding := float32(4)
	left := r.X + padding
	top := r.Y + padding
	width := r.W - padding*2
	height := r.H - padding*2
	for i := 1; i < 4; i++ {
		y := top + height*float32(i)/4
		p.Line(left, y, left+width, y, 1, t.Border.Alpha(0.55))
	}
	if len(a.traffic) < 2 {
		p.Text(left, top+12, "нет данных", 10, t.TextMuted)
		return
	}
	maxRate := float64(a.trafficPeak())
	if maxRate < 1024 {
		maxRate = 1024
	}
	maxRate *= 1.12
	step := width / float32(len(a.traffic)-1)
	down := make([][2]float32, 0, len(a.traffic))
	up := make([][2]float32, 0, len(a.traffic))
	for i, point := range a.traffic {
		x := left + float32(i)*step
		down = append(down, [2]float32{x, top + height - float32(float64(point.Down)/maxRate)*height})
		up = append(up, [2]float32{x, top + height - float32(float64(point.Up)/maxRate)*height})
	}
	p.StrokePolyline(down, 1.6, t.Accent)
	p.StrokePolyline(up, 1.2, t.TextMuted)
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "<1 с"
	}
	seconds := int(d.Seconds())
	if seconds < 60 {
		return fmt.Sprintf("%d с", seconds)
	}
	minutes := seconds / 60
	seconds %= 60
	if minutes < 60 {
		return fmt.Sprintf("%dм %dс", minutes, seconds)
	}
	hours := minutes / 60
	minutes %= 60
	return fmt.Sprintf("%dч %dм", hours, minutes)
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d Б", n)
	}
	units := []string{"КБ", "МБ", "ГБ", "ТБ"}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < len(units)-1; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), units[exp])
}
