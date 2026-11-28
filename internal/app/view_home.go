package app

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"qbox/internal/config"
	"qbox/internal/model"
)

func (a *App) homePage(c *ui.Context, d design) {
	ui.Row(c).Grow(1).FillWidth().Background(d.T.Background).Children(func() {
		a.subscriptionsSidebar(c, d)
		ui.Box(c).Width(dividerSize).FillHeight().Background(d.T.Border)
		a.contentPanel(c, d)
	})
}

func (a *App) subscriptionsSidebar(c *ui.Context, d design) {
	t := d.T
	ui.Column(c).Width(sidebarWidth).FillHeight().Background(t.Background).
		Padding(spaceXL, spaceL).Gap(spaceM).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Подписки").FontSize(fontSizeLabel).Bold()
			ui.Spacer(c)
			if a.workLabel() != "" {
				ui.Text(c, a.workTS).FontSize(fontSizeSmall).TextColor(t.TextMuted)
			}
		})
		ui.Scroll(c).Grow(1).Background(t.Background).Gap(spaceXXS).Children(func() {
			if len(a.cfg.Subscriptions) == 0 {
				ui.Column(c).FillWidth().Padding(spaceXL, spaceM).Gap(spaceXS).Children(func() {
					ui.Text(c, "Подписок пока нет").FontSize(fontSizeLabel)
					hintText(c, d, "Добавьте base64/URI-список или sing-box JSON.")
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
	row := ui.ButtonBase(c).FillWidth().Padding(spaceS, spaceM).Gap(spaceM).Radius(d.RadiusM).AlignItems(ui.Start)
	switch {
	case active:
		row.Background(t.Accent.Alpha(accentTintAlpha))
	case row.Hovered():
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		ui.Box(c).Size(avatarSize, avatarSize).Radius(d.RadiusS).Background(avatarColor(sub.ID)).Center().Children(func() {
			ui.Text(c, strings.ToUpper(avatarLetter(sub.Name))).FontSize(fontSizeAvatar).Bold().TextColor(ui.Hex(colorWhite))
		})
		ui.Column(c).Gap(spaceXXS).Grow(1).Children(func() {
			ui.Row(c).Gap(spaceS).AlignItems(ui.Center).Children(func() {
				if active {
					ui.Box(c).Size(statusDotSize, statusDotSize).Radius(statusDotRadius).Background(t.Accent)
				}
				ui.Text(c, sub.Name).FontSize(fontSizeLabel).Grow(1).SingleLine()
			})
			ui.Text(c, displaySubscriptionURL(sub.URL)).FontSize(fontSizeSmall).TextColor(t.TextMuted).SingleLine()
			updated := "ещё не обновлялась"
			if !sub.UpdatedAt.IsZero() {
				updated = "обновлено " + sub.UpdatedAt.Format(dateTimeLayout)
			}
			ui.Text(c, updated).FontSize(fontSizeSmall).TextColor(t.TextMuted).SingleLine()
		})
		a.subscriptionMenu(c, d, sub)
	})
	row.OnClick(func() { a.selectSubscription(sub.ID) })
}

func displaySubscriptionURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "ссылка скрыта"
	}
	display := u.Scheme
	if display == "" {
		display = "https"
	}
	display += "://" + u.Host
	if strings.Trim(u.EscapedPath(), "/") != "" {
		display += "/…"
	}
	return display
}

func (a *App) subscriptionMenu(c *ui.Context, d design, sub config.Subscription) {
	trigger := iconAction(c, d, "Меню подписки", iconEllipsis, menuButtonSize, iconSize2XL, nil)
	trigger.PopoverMenuButton(func(m *ui.Menu) {
		if m.Item("Обновить").Icon(menuIconRefresh).Disabled(a.working()).Chosen() {
			a.refreshSubscription(sub.ID)
		}
		m.Separator()
		if m.Item("Переименовать…").Icon(menuIconPencil).Chosen() {
			a.openRenameSubscription(sub.ID)
		}
		if m.Item("Удалить…").Icon(menuIconTrash).Danger().Chosen() {
			a.openDeleteSubscription(sub.ID)
		}
	}, menuOptionsAt(d, nil, ui.AnchorBottomEnd, ui.AnchorTopEnd))
}

func (a *App) contentPanel(c *ui.Context, d design) {
	ui.Column(c).Grow(1).FillHeight().Background(d.T.Background).Padding(contentPadding).Gap(cardGap).Children(func() {
		a.contentToolbar(c, d)
		switch a.panel {
		case panelNodes:
			a.nodesView(c, d)
		case panelLogs:
			a.logsView(c, d)
		case panelConnections:
			a.connectionsView(c, d)
		}
		a.trafficCard(c, d)
	})
}

func (a *App) contentToolbar(c *ui.Context, d design) {
	tabs := a.panel
	ui.Row(c).Gap(spaceS).AlignItems(ui.Center).Children(func() {
		ui.Segmented(c, &tabs, "Ноды", "Логи", "Подключения").OnChange(func() {
			a.panel = tabs
			a.refreshConnections()
			a.invalidate()
		})
		ui.Spacer(c)
		if a.pinging {
			iconButton(c, fmt.Sprintf("Отмена (%d/%d)", a.pingDone, a.pingTotal), iconX, false).OnClick(a.cancelPing)
		} else {
			iconButton(c, "Пинг", iconActivity, false).
				Disabled(a.working() || len(a.cfg.ActiveNodes()) == 0).
				OnClick(a.pingNodes)
		}
		a.restartButton(c, "Перезапустить")
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
			iconButton(c, label, icon, true).Disabled(a.working()).OnClick(a.start)
		}
	})
	a.restartHint(c, d)
}

func (a *App) restartButton(c *ui.Context, label string) {
	primary := a.needsRestart()
	button := iconButton(c, label, iconRotateCW, primary)
	if primary {
		button.Disabled(a.working())
	} else {
		button.Disabled(a.working() || a.state != model.StateRunning)
	}
	button.OnClick(a.restartNow)
}

func (a *App) nodesView(c *ui.Context, d design) {
	sub := a.activeSub()
	if sub == nil || len(sub.Nodes) == 0 {
		emptyState(c, d, iconServer, "Нод пока нет", "Выберите подписку слева или добавьте новую.", func() {
			iconButton(c, "Добавить подписку", iconPlus, false).OnClick(a.openAddSubscription)
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
	}).Grow(1).Background(d.T.Background)
}

func (a *App) nodeRow(c *ui.Context, d design, row nodeRow, selected bool, index int) {
	t := d.T
	line := ui.ButtonBase(c).FillWidth().Padding(spaceM, spaceL).Gap(spaceL).Radius(d.RadiusM)
	switch {
	case selected:
		line.Background(t.Accent.Alpha(accentTintAlpha))
	case line.Hovered():
		line.Background(t.SurfaceHover)
	}
	line.Children(func() {
		accentBar := ui.Box(c).Width(nodeAccentWidth)
		if selected {
			accentBar.Height(nodeAccentHeight).Radius(statusDotRadius).Background(t.Accent)
		}
		ui.Text(c, strings.ToUpper(row.typ)).FontSize(fontSizeCaption).Bold().TextColor(t.Accent).
			Padding(spaceXS, spaceS).Background(t.Accent.Alpha(accentTintAlpha)).Radius(d.RadiusXS)
		ui.Column(c).Gap(spaceXXS).Grow(1).Children(func() {
			ui.Text(c, row.title).FontSize(fontSizeLabel)
			ui.Text(c, row.detail).FontSize(fontSizeSmall).TextColor(t.TextMuted)
		})
		if !row.auto {
			if text, tone := a.latency(row.tag); text != "" {
				ui.Text(c, text).FontSize(fontSizeBody).TextColor(a.latencyColor(t, tone))
			}
		}
	})
	line.OnClick(func() { a.selectNode(index) })
}

func (a *App) latencyColor(t *ui.Theme, tone string) ui.Color {
	switch tone {
	case latencyOK:
		return t.Success
	case latencyError:
		return t.Danger
	default:
		return t.TextMuted
	}
}

func (a *App) logsView(c *ui.Context, d design) {
	t := d.T
	ui.Column(c).Grow(1).Gap(spaceM).Children(func() {
		filter := a.logView
		ui.Row(c).Gap(spaceM).AlignItems(ui.Center).Children(func() {
			ui.Segmented(c, &filter, "Все", "Info", "Warn", "Error").OnChange(func() {
				a.logView = filter
			})
			ui.Spacer(c)
			baseButton(c, "Очистить", false).OnClick(a.clearLogs)
			baseButton(c, "Где логи?", false).OnClick(a.openLogFolder)
		})
		lines := a.logLines()
		a.logList.FollowEnd = true
		ui.List(c, &a.logList, len(lines), func(i int) {
			entry := lines[i]
			color := logLevelColor(t, entry.Level)
			ui.Row(c).Gap(spaceM).Padding(spaceXS, spaceM).Children(func() {
				ui.Text(c, entry.Time.Format(clockLayout)).TextColor(t.TextMuted).FontSize(fontSizeSmall)
				ui.Text(c, strings.ToUpper(entry.Level)).TextColor(color).FontSize(fontSizeSmall).Width(logLevelWidth)
				ui.Text(c, entry.Message).TextColor(color).FontSize(fontSizeBody).Grow(1)
			})
		}).Grow(1).Background(t.Background).Children(func() {
			if len(lines) == 0 {
				ui.Text(c, "Журнал пуст.").TextColor(t.TextMuted).Padding(contentPadding)
			}
		})
	})
}

func (a *App) connectionsView(c *ui.Context, d design) {
	ui.Column(c).Grow(1).Gap(spaceS).Children(func() {
		ui.Textf(c, "Активных соединений: %d", len(a.connections)).FontSize(fontSizeSmall).TextColor(d.T.TextMuted)
		if len(a.connections) == 0 {
			emptyState(c, d, iconActivity, "Нет активных соединений", "Список обновляется автоматически.", nil)
			return
		}
		ui.List(c, &a.connList, len(a.connections), func(i int) {
			conn := a.connections[i]
			row := ui.Row(c).FillWidth().Padding(spaceS, spaceM).Gap(spaceL).AlignItems(ui.Center).Radius(d.RadiusM)
			if row.Hovered() {
				row.Background(d.T.SurfaceHover)
			}
			row.Children(func() {
				ui.Column(c).Gap(spaceXXS).Grow(1).Children(func() {
					host := conn.Host
					if host == "" {
						host = conn.Destination
					}
					ui.Text(c, host).FontSize(fontSizeBody)
					ui.Text(c, connectionDetail(conn)).FontSize(fontSizeSmall).TextColor(d.T.TextMuted)
				})
				ui.Column(c).Gap(spaceXXS).AlignItems(ui.End).Children(func() {
					ui.Textf(c, "↑ %s  ↓ %s", formatBytes(conn.Upload), formatBytes(conn.Download)).
						FontSize(fontSizeSmall).TextColor(d.T.TextMuted)
					ui.Text(c, formatDuration(time.Since(conn.CreatedAt))).
						FontSize(fontSizeSmall).TextColor(d.T.TextMuted)
				})
			})
		}).Grow(1).Background(d.T.Background)
	})
}

func logLevelColor(t *ui.Theme, level string) ui.Color {
	switch level {
	case "warn":
		return t.Warning
	case "error", "fatal", "panic":
		return t.Danger
	default:
		return t.Text
	}
}

func connectionDetail(conn model.Connection) string {
	parts := make([]string, 0, 4)
	for _, part := range []string{conn.Network, conn.Protocol, conn.Outbound, conn.Rule} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if detail := strings.Join(parts, " · "); detail != "" {
		return detail
	}
	return conn.Destination
}
