package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"qbox/internal/config"
	"qbox/internal/model"
	"qbox/internal/rulesets"
)

var pages = []string{"Подключение", "Серверы", "Маршрутизация", "Логи", "Настройки"}

func (a *App) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		a.header(c)
		ui.Segmented(c, &a.page, pages...).FillEqually().Label("Раздел").OnChange(func() {
			a.notice = ""
		})
		if a.notice != "" {
			ui.Text(c, a.notice).TextColor(t.TextMuted).FontSize(12)
		}
		if a.err != "" && a.page != 0 {
			ui.Text(c, a.err).TextColor(t.Danger).FontSize(12).Wrap()
		}
		if a.warn != "" {
			ui.Text(c, a.warn).TextColor(t.Warning).FontSize(12).Wrap()
		}
		switch a.page {
		case 0:
			a.connectionPage(c)
		case 1:
			a.serversPage(c)
		case 2:
			a.routingPage(c)
		case 3:
			a.logsPage(c)
		case 4:
			a.settingsPage(c)
		}
	})
}

func (a *App) header(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "qbox").FontSize(22).Bold()
				color := t.TextMuted
				switch a.state {
				case model.StateRunning:
					color = t.Success
				case model.StateError:
					color = t.Danger
				case model.StateStarting, model.StateStopping:
					color = t.Warning
				}
				ui.Text(c, a.stateText()).TextColor(color).FontSize(13)
			})
			ui.Text(c, a.statusDetail()).TextColor(t.TextMuted).FontSize(12)
		})
		if a.busy {
			ui.Button(c, a.busyTS).Disabled(true)
		} else if a.state == model.StateRunning {
			if a.dirty {
				ui.PrimaryButton(c, "Перезапустить").OnClick(a.restartNow)
			}
			ui.Button(c, "Отключиться").OnClick(a.stop)
		} else {
			label := "Подключиться"
			if a.state == model.StateError {
				label = "Повторить"
			}
			ui.PrimaryButton(c, label).OnClick(a.start)
		}
	})
}

func (a *App) statusDetail() string {
	parts := []string{fmt.Sprintf("↑ %s · ↓ %s", formatBytes(a.stats.Up), formatBytes(a.stats.Down))}
	if a.stats.Connections > 0 {
		parts = append(parts, fmt.Sprintf("соединений: %d", a.stats.Connections))
	}
	if sub := a.activeSub(); sub != nil {
		parts = append(parts, "подписка: "+sub.Name)
	}
	if a.cfg.SelectedNode != "" && a.cfg.SelectedNode != "auto" {
		parts = append(parts, "нода: "+a.cfg.SelectedNode)
	}
	return strings.Join(parts, "  ·  ")
}

func (a *App) connectionPage(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Grow(1).Gap(16).Children(func() {
		ui.Column(c).Gap(6).Children(func() {
			ui.Text(c, a.stateText()).FontSize(28).Bold()
			ui.Text(c, a.stateHint()).TextColor(t.TextMuted)
		})

		ui.Row(c).Gap(24).Children(func() {
			a.statBox(c, "Отдано", formatBytes(a.stats.Up))
			a.statBox(c, "Получено", formatBytes(a.stats.Down))
			a.statBox(c, "Соединений", fmt.Sprintf("%d", a.stats.Connections))
		})

		if a.err != "" {
			ui.Text(c, a.err).TextColor(t.Danger)
		}
		if a.dirty && a.state == model.StateRunning {
			ui.Text(c, "Настройки изменены: перезапустите подключение, чтобы применить их.").TextColor(t.Warning)
		}
		if !a.elevated() {
			ui.Text(c, "TUN требует прав администратора: при подключении Windows покажет запрос UAC.").TextColor(t.Warning)
		}
		if len(a.cfg.ActiveNodes()) == 0 {
			ui.Text(c, "Нет нод. Откройте «Серверы» и добавьте подписку.").TextColor(t.TextMuted)
		}
	})
}

func (a *App) statBox(c *ui.Context, title, value string) {
	t := c.Theme()
	ui.Column(c).Gap(4).MinWidth(150).Padding(12).Background(t.Surface).Radius(10).Children(func() {
		ui.Text(c, title).TextColor(t.TextMuted).FontSize(12)
		ui.Text(c, value).FontSize(18).Bold()
	})
}

func (a *App) stateHint() string {
	switch a.state {
	case model.StateRunning:
		if a.cfg.Routing.Mode == config.ModeDirect {
			return "Трафик идёт напрямую, без прокси."
		}
		return "Весь трафик направлен через выбранную ноду."
	case model.StateStarting:
		return "Создаётся TUN-интерфейс и применяются маршруты."
	case model.StateStopping:
		return "Удаляются маршруты и закрывается TUN."
	case model.StateError:
		return "Проверьте журнал: причина запуска указана ниже."
	default:
		return "Нажмите «Подключиться», чтобы поднять TUN и применить правила."
	}
}

func (a *App) serversPage(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Grow(1).Gap(12).Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Подписки").FontSize(18).Bold().Grow(1)
			if !a.adding && len(a.cfg.ActiveNodes()) > 0 {
				label := "Пинг всех"
				if a.pinging {
					label = "Пинг…"
				}
				ui.Button(c, label).Disabled(a.pinging || a.busy).OnClick(a.pingNodes)
			}
			if !a.adding {
				ui.Button(c, "Добавить подписку").OnClick(func() {
					a.adding = true
					a.err = ""
				})
			}
		})

		if a.adding {
			a.addSubscriptionForm(c)
		}

		if len(a.cfg.Subscriptions) == 0 && !a.adding {
			ui.Text(c, "Подписок пока нет. Поддерживаются base64/URI-списки и sing-box JSON.").TextColor(t.TextMuted)
		}

		for i := range a.cfg.Subscriptions {
			sub := a.cfg.Subscriptions[i]
			a.subscriptionCard(c, sub)
		}

		sub := a.activeSub()
		if sub == nil {
			return
		}
		ui.Textf(c, "Ноды активной подписки: %d", len(sub.Nodes)).FontSize(16).Bold().Padding(0, 4)
		a.syncNodeIndex()
		rows := a.nodeRows()
		a.nodeList.Key = func(i int) any {
			if i >= 0 && i < len(rows) {
				return rows[i].tag
			}
			return i
		}
		a.nodeList.Selected = &a.nodeIndex
		ui.List(c, &a.nodeList, len(rows), func(i int) {
			row := rows[i]
			ui.Column(c).Gap(2).Padding(8, 12).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, row.title).Grow(1)
					if !row.auto {
						if text, tone := a.latency(row.tag); text != "" {
							color := t.TextMuted
							switch tone {
							case "ok":
								color = t.Success
							case "error":
								color = t.Danger
							}
							ui.Text(c, text).TextColor(color).FontSize(12)
						}
					}
				})
				ui.Text(c, row.detail).TextColor(t.TextMuted).FontSize(12)
			})
		}).Height(280).OnChange(func() {
			a.selectNode(a.nodeIndex)
		})
		if a.state == model.StateRunning {
			ui.Text(c, "Смена ноды перезапустит подключение.").TextColor(t.TextMuted).FontSize(12)
		}
	})
}

func (a *App) addSubscriptionForm(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(10).Padding(12).Background(t.Surface).Radius(10).Children(func() {
		ui.Text(c, "Новая подписка").Bold()
		ui.TextInput(c, &a.addName).Label("Название").Placeholder("например, Provider")
		ui.TextInput(c, &a.addURL).Label("URL").Placeholder("https://example.com/sub")
		ui.Row(c).Gap(8).Children(func() {
			ui.PrimaryButton(c, "Импортировать").Disabled(a.busy || strings.TrimSpace(a.addURL) == "").OnClick(a.importSubscription)
			ui.Button(c, "Отмена").OnClick(func() {
				a.adding = false
				a.addName = ""
				a.addURL = ""
				a.err = ""
			})
		})
	})
}

func (a *App) subscriptionCard(c *ui.Context, sub config.Subscription) {
	t := c.Theme()
	active := a.cfg.ActiveSubscription == sub.ID
	ui.Column(c).Gap(8).Padding(12).Background(t.Surface).Radius(10).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, sub.Name).Bold().Grow(1)
			if active {
				ui.Text(c, "активна").TextColor(t.Success).FontSize(12)
			} else {
				id := sub.ID
				ui.Button(c, "Выбрать").OnClick(func() { a.selectSubscription(id) })
			}
		})
		ui.Text(c, displayURL(sub.URL)).TextColor(t.TextMuted).FontSize(12)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			updated := "ещё не обновлялась"
			if !sub.UpdatedAt.IsZero() {
				updated = sub.UpdatedAt.Format("2006-01-02 15:04")
			}
			ui.Textf(c, "%d нод · %s", len(sub.Nodes), updated).TextColor(t.TextMuted).FontSize(12).Grow(1)
			id := sub.ID
			ui.Button(c, "Обновить").Disabled(a.busy).OnClick(func() { a.refreshSubscription(id) })
			if a.confirmDel == id {
				ui.Button(c, "Удалить?").OnClick(func() { a.deleteSubscription(id) })
				ui.Button(c, "Нет").OnClick(func() { a.confirmDel = "" })
			} else {
				ui.Button(c, "Удалить").OnClick(func() { a.confirmDel = id })
			}
		})
	})
}

func (a *App) routingPage(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Grow(1).Gap(14).Children(func() {
		ui.Text(c, "Режим").FontSize(18).Bold()
		mode := a.modeIndex()
		ui.Segmented(c, &mode, "Global", "Rules", "Direct").FillEqually().OnChange(func() {
			a.setMode(mode)
		})
		ui.Text(c, "Global — всё через прокси; Rules — списки и гео-правила; Direct — без прокси.").TextColor(t.TextMuted).FontSize(12)

		a.switchRow(c, "Обход локальной сети", &a.cfg.Routing.BypassLAN, "Приватные IP идут напрямую.")
		a.switchRow(c, "Обход РФ", &a.cfg.Routing.BypassRU, "Домены и IP РФ идут напрямую (rule-sets).")
		a.switchRow(c, "Блокировать рекламу", &a.cfg.Routing.BlockAds, "geosite-category-ads-all отклоняется.")

		ui.Text(c, "Базы правил").FontSize(16).Bold()
		for _, st := range a.deps.RuleSets.Status() {
			a.ruleSetRow(c, st)
		}
		ui.Button(c, "Обновить все базы").Disabled(a.busy).OnClick(a.updateAllRuleSets)

		ui.Text(c, "Свои списки").FontSize(16).Bold()
		ui.Text(c, "Правила proxy проверяются раньше обхода РФ, затем direct и финальный маршрут.").TextColor(t.TextMuted).FontSize(12)

		ui.Text(c, "Proxy: домены (по одному в строке)").TextColor(t.TextMuted).FontSize(12)
		ui.TextArea(c, &a.proxyText).Lines(2, 5).OnChange(func() {
			a.cfg.Routing.ProxyDomains = config.SplitList(a.proxyText)
			a.routingChanged()
		})
		ui.Text(c, "Proxy: IP/CIDR").TextColor(t.TextMuted).FontSize(12)
		ui.TextArea(c, &a.proxyCIDRs).Lines(2, 4).OnChange(func() {
			a.cfg.Routing.ProxyCIDRs = config.SplitList(a.proxyCIDRs)
			a.routingChanged()
		})
		ui.Text(c, "Direct: домены").TextColor(t.TextMuted).FontSize(12)
		ui.TextArea(c, &a.directText).Lines(2, 5).OnChange(func() {
			a.cfg.Routing.DirectDomains = config.SplitList(a.directText)
			a.routingChanged()
		})
		ui.Text(c, "Direct: IP/CIDR").TextColor(t.TextMuted).FontSize(12)
		ui.TextArea(c, &a.directCIDRs).Lines(2, 4).OnChange(func() {
			a.cfg.Routing.DirectCIDRs = config.SplitList(a.directCIDRs)
			a.routingChanged()
		})
	})
}

func (a *App) switchRow(c *ui.Context, title string, value *bool, hint string) {
	t := c.Theme()
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, title)
			ui.Text(c, hint).TextColor(t.TextMuted).FontSize(12)
		})
		ui.Switch(c, value).Label(title).OnChange(a.routingChanged)
	})
}

func (a *App) ruleSetRow(c *ui.Context, st rulesets.Status) {
	t := c.Theme()
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, st.Title)
			status := "не установлена"
			if st.Installed {
				status = fmt.Sprintf("%s · %s", formatBytes(st.Size), st.Updated.Format("2006-01-02"))
			}
			ui.Text(c, status).TextColor(t.TextMuted).FontSize(12)
		})
		tag := st.Tag
		label := "Скачать"
		if st.Installed {
			label = "Обновить"
		}
		ui.Button(c, label).Disabled(a.busy).OnClick(func() { a.updateRuleSet(tag) })
	})
}

func (a *App) logsPage(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		filter := a.logView
		ui.Segmented(c, &filter, "Все", "Info", "Warn", "Error").OnChange(func() { a.logView = filter })
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
		ui.Row(c).Gap(8).Padding(4, 8).Children(func() {
			ui.Text(c, entry.Time.Format("15:04:05")).TextColor(t.TextMuted).FontSize(11)
			ui.Text(c, strings.ToUpper(entry.Level)).TextColor(color).FontSize(11).Width(48)
			ui.Text(c, entry.Message).TextColor(color).FontSize(12).Grow(1)
		})
	}).Grow(1).Children(func() {
		if len(lines) == 0 {
			ui.Text(c, "Журнал пуст.").TextColor(t.TextMuted).Padding(12)
		}
	})
}

func (a *App) settingsPage(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Grow(1).Gap(14).Children(func() {
		ui.Text(c, "TUN").FontSize(18).Bold()
		stack := a.stackIndex()
		ui.Segmented(c, &stack, "System", "Mixed", "gVisor").FillEqually().OnChange(func() { a.setStack(stack) })
		ui.Text(c, "Mixed требует сборки с тегом with_gvisor; System быстрее, gVisor — универсальнее.").TextColor(t.TextMuted).FontSize(12)
		a.switchRow(c, "Отключить IPv6 в TUN", &a.cfg.TUN.DisableIPv6, "Включайте, если Windows не может назначить IPv6-адрес TUN-интерфейсу.")
		ui.Text(c, "Имя интерфейса").TextColor(t.TextMuted).FontSize(12)
		ui.TextInput(c, &a.cfg.TUN.InterfaceName).Label("Имя интерфейса").OnChange(a.routingChanged)
		a.switchRow(c, "Strict route", &a.cfg.TUN.StrictRoute, "Блокирует утечки DNS и обход TUN; может ломать отдельные приложения.")

		ui.Text(c, "DNS").FontSize(18).Bold()
		ui.Text(c, "DoH/DoT сервер").TextColor(t.TextMuted).FontSize(12)
		ui.TextInput(c, &a.cfg.DNS.Remote).Label("DNS-сервер").Placeholder("https://1.1.1.1/dns-query").OnChange(a.routingChanged)
		a.switchRow(c, "DNS через прокси", &a.cfg.DNS.UseProxy, "В режиме Direct используется системный DNS.")

		ui.Text(c, "Журнал").FontSize(18).Bold()
		level := a.logLevelIndex()
		ui.Segmented(c, &level, "Info", "Debug", "Trace").FillEqually().OnChange(func() { a.setLogLevel(level) })

		if a.state == model.StateRunning && a.dirty {
			ui.PrimaryButton(c, "Применить и перезапустить").OnClick(a.restartNow)
		}

		ui.Text(c, "Файлы").FontSize(16).Bold()
		ui.Text(c, "Конфиг: "+a.deps.ConfigPath).TextColor(t.TextMuted).FontSize(12)
		ui.Text(c, "Кэш: "+a.deps.CacheDir).TextColor(t.TextMuted).FontSize(12)
		ui.Text(c, "qbox · GPLv3 · sing-box v1.14.4 · MyGo").TextColor(t.TextMuted).FontSize(12)
	})
}

func displayURL(raw string) string {
	if len(raw) > 100 {
		return raw[:97] + "…"
	}
	return raw
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
