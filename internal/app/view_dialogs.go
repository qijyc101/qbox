package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"qbox/internal/model"
	"qbox/internal/rulesets"
)

const childWindowPadding = 14

func (a *App) childHeader(c *ui.Context, d design, title, subtitle string, actions func(), close func()) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Text(c, title).FontSize(18).Bold()
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(11).TextColor(d.T.TextMuted).SingleLine().Grow(1)
		} else {
			ui.Text(c, "").Grow(1)
		}
		if actions != nil {
			actions()
		}
		a.childCloseButton(c, d, close)
	})
}

func (a *App) childCloseButton(c *ui.Context, d design, close func()) {
	btn := ui.ButtonBase(c).Size(28, 28).Radius(d.RadiusS).Center().Label("Закрыть").Tooltip("Закрыть")
	if btn.Hovered() {
		btn.Background(ui.Hex(windowCloseHover))
	} else {
		btn.Background(d.T.SurfaceHover)
	}
	btn.Children(func() {
		color := d.T.TextMuted
		if btn.Hovered() {
			color = ui.Hex("#ffffff")
		}
		ui.Icon(c, iconX).Size(14, 14).TextColor(color)
	})
	if btn.Clicked() {
		close()
	}
}

func (a *App) dialogHeader(c *ui.Context, d design, title string, close func()) {
	a.childHeader(c, d, title, "", nil, close)
}

func (a *App) addSubscriptionView(c *ui.Context) {
	d := a.prepare(c)
	t := d.T
	ui.Column(c).Fill().Padding(childWindowPadding).Gap(10).Children(func() {
		a.dialogHeader(c, d, "Новая подписка", func() { a.closeChild(winAddSubscription) })
		ui.Text(c, "Поддерживаются base64/URI-списки и sing-box JSON.").FontSize(12).TextColor(t.TextMuted).Wrap()
		ui.Text(c, "Название").FontSize(11).TextColor(t.TextMuted)
		ui.TextInput(c, &a.addName).Label("Название").Placeholder("например, Provider")
		ui.Text(c, "URL").FontSize(11).TextColor(t.TextMuted)
		ui.TextInput(c, &a.addURL).Label("URL").Placeholder("https://example.com/sub")
		if a.workBusy && a.workTS != "" {
			ui.Text(c, a.workTS).FontSize(12).TextColor(t.TextMuted)
		}
		if a.err != "" {
			ui.Text(c, a.err).FontSize(12).TextColor(t.Danger).Wrap()
		}
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "").Grow(1)
			textButton(c, "Импортировать", true).
				Disabled(a.busy || a.workBusy || trimText(a.addURL) == "").
				OnClick(a.importSubscription)
			textButton(c, "Отмена", false).OnClick(func() { a.closeChild(winAddSubscription) })
		})
	})
}

func (a *App) renameSubscriptionView(c *ui.Context) {
	d := a.prepare(c)
	t := d.T
	ui.Column(c).Fill().Padding(childWindowPadding).Gap(10).Children(func() {
		a.dialogHeader(c, d, "Переименовать подписку", func() { a.closeChild(winRenameSub) })
		ui.Text(c, "Название").FontSize(11).TextColor(t.TextMuted)
		ui.TextInput(c, &a.subNameDraft).Label("Название")
		if a.err != "" {
			ui.Text(c, a.err).FontSize(12).TextColor(t.Danger).Wrap()
		}
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "").Grow(1)
			textButton(c, "Сохранить", true).OnClick(a.renameSubscription)
			textButton(c, "Отмена", false).OnClick(func() { a.closeChild(winRenameSub) })
		})
	})
}

func (a *App) deleteSubscriptionView(c *ui.Context) {
	d := a.prepare(c)
	name := a.subDeleting
	if sub := a.cfg.Subscription(a.subDeleting); sub != nil {
		name = sub.Name
	}
	ui.Column(c).Fill().Padding(childWindowPadding).Gap(10).Children(func() {
		a.dialogHeader(c, d, "Удалить подписку?", func() { a.closeChild(winDeleteSub) })
		ui.Text(c, "«"+name+"» и её ноды будут удалены. Это действие нельзя отменить.").
			FontSize(12).TextColor(d.T.Warning).Wrap()
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "").Grow(1)
			textButton(c, "Удалить", false).TextColor(d.T.Danger).OnClick(a.confirmDeleteSubscription)
			textButton(c, "Отмена", false).OnClick(func() { a.closeChild(winDeleteSub) })
		})
	})
}

func (a *App) newProfileView(c *ui.Context) {
	d := a.prepare(c)
	t := d.T
	ui.Column(c).Fill().Padding(childWindowPadding).Gap(10).Children(func() {
		a.dialogHeader(c, d, "Новый профиль", func() { a.closeChild(winNewProfile) })
		ui.Text(c, "Профиль начнёт с текущих правил; их можно изменить отдельно.").FontSize(12).TextColor(t.TextMuted).Wrap()
		ui.Text(c, "Название профиля").FontSize(11).TextColor(t.TextMuted)
		ui.TextInput(c, &a.profileNameDraft).Label("Название профиля")
		if a.err != "" {
			ui.Text(c, a.err).FontSize(12).TextColor(t.Danger).Wrap()
		}
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "").Grow(1)
			textButton(c, "Создать", true).OnClick(a.saveNewProfile)
			textButton(c, "Отмена", false).OnClick(func() { a.closeChild(winNewProfile) })
		})
	})
}

func (a *App) routingEditorView(c *ui.Context) {
	d := a.prepare(c)
	t := d.T
	profile := a.activeRoutingProfileName()
	ui.Column(c).Fill().Padding(childWindowPadding).Gap(10).Children(func() {
		a.childHeader(c, d, "Профиль", profile, func() {
			if a.state == model.StateRunning {
				iconButton(c, "Перезапустить", iconRotateCW, true).Disabled(a.busy || a.workBusy).OnClick(a.restartNow)
			}
		}, func() { a.closeChild(winRouting) })
		ui.Scroll(c).Grow(1).Background(t.Background).Gap(12).Children(func() {
			a.settingsCard(c, d, iconPencil, "Название", func() {
				ui.Text(c, "Имя профиля").FontSize(11).TextColor(t.TextMuted)
				ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
					ui.TextInput(c, &a.profileNameDraft).Label("Название профиля").Grow(1)
					iconButton(c, "Сохранить", iconCheck, true).OnClick(a.saveRoutingProfileName)
				})
				if a.err != "" {
					ui.Text(c, a.err).FontSize(12).TextColor(t.Danger).Wrap()
				}
			})

			a.settingsCard(c, d, iconRoute, "Режим", func() {
				mode := a.modeIndex()
				ui.Segmented(c, &mode, "Global", "Rules", "Direct").AlignSelf(ui.Start).OnChange(func() {
					a.setMode(mode)
				})
				ui.Text(c, "Global — весь трафик через прокси; LAN и блокировка рекламы действуют по настройкам. Rules — списки и гео-правила; Direct — без прокси.").
					FontSize(11).TextColor(t.TextMuted).Wrap()
			})

			a.settingsCard(c, d, iconGlobe, "Правила", func() {
				a.switchRow(c, "Обход локальной сети", &a.cfg.Routing.BypassLAN, "Приватные IP идут напрямую.")
				a.switchRow(c, "Обход РФ", &a.cfg.Routing.BypassRU, "Домены и IP РФ идут напрямую; DNS для доменов РФ — системный.")
				a.switchRow(c, "Блокировать рекламу", &a.cfg.Routing.BlockAds, "geosite-category-ads-all отклоняется.")
			})

			a.settingsCard(c, d, iconServer, "Базы правил", func() {
				for _, st := range a.deps.RuleSets.Status() {
					a.ruleSetRow(c, st)
				}
				iconButton(c, "Обновить все базы", iconDownload, false).AlignSelf(ui.End).Disabled(a.busy || a.workBusy).OnClick(a.updateAllRuleSets)
			})

			a.settingsCard(c, d, iconScrollText, "Свои списки", func() {
				ui.Text(c, "Списки применяются только в режиме Rules. Введите значение и нажмите Enter; чипы можно удалять. Порядок: proxy → свои direct-списки → обход РФ → финальный маршрут.").
					FontSize(11).TextColor(t.TextMuted).Wrap()
				ui.Text(c, "Proxy: домены").FontSize(11).TextColor(t.TextMuted)
				if ui.TokenField(c, &a.cfg.Routing.ProxyDomains, nil).Label("Proxy: домены").Placeholder("Добавить домен…").FillWidth().Changed() {
					a.routingChanged()
				}
				ui.Text(c, "Proxy: IP/CIDR").FontSize(11).TextColor(t.TextMuted)
				if ui.TokenField(c, &a.cfg.Routing.ProxyCIDRs, nil).Label("Proxy: IP/CIDR").Placeholder("Добавить IP или CIDR…").FillWidth().Changed() {
					a.routingChanged()
				}
				ui.Text(c, "Direct: домены").FontSize(11).TextColor(t.TextMuted)
				ui.Text(c, "Домены и их поддомены идут напрямую; DNS для них резолвится системным (корпоративным) сервером.").
					FontSize(11).TextColor(t.TextMuted).Wrap()
				if ui.TokenField(c, &a.cfg.Routing.DirectDomains, nil).Label("Direct: домены").Placeholder("Добавить домен…").FillWidth().Changed() {
					a.routingChanged()
				}
				ui.Text(c, "Direct: IP/CIDR").FontSize(11).TextColor(t.TextMuted)
				if ui.TokenField(c, &a.cfg.Routing.DirectCIDRs, nil).Label("Direct: IP/CIDR").Placeholder("Добавить IP или CIDR…").FillWidth().Changed() {
					a.routingChanged()
				}
			})

			a.settingsCard(c, d, iconTrash2, "Удаление профиля", func() {
				if len(a.cfg.RoutingProfiles) <= 1 {
					ui.Text(c, "Единственный профиль удалить нельзя.").FontSize(11).TextColor(t.TextMuted)
				} else if a.profileDeleteConfirm {
					ui.Text(c, "Удалить профиль «"+profile+"»? Это действие нельзя отменить.").
						FontSize(11).TextColor(t.Warning).Wrap()
					ui.Row(c).Gap(8).Children(func() {
						ui.Text(c, "").Grow(1)
						textButton(c, "Удалить", false).TextColor(t.Danger).OnClick(a.deleteActiveProfile)
						textButton(c, "Отмена", false).OnClick(func() {
							a.profileDeleteConfirm = false
							a.invalidate()
						})
					})
				} else {
					ui.Text(c, "Профиль будет удалён вместе со своими правилами; активным станет соседний профиль.").
						FontSize(11).TextColor(t.TextMuted).Wrap()
					textButton(c, "Удалить профиль", false).AlignSelf(ui.End).TextColor(t.Danger).OnClick(func() {
						a.profileDeleteConfirm = true
						a.invalidate()
					})
				}
			})

			if a.dirty && a.state == model.StateRunning {
				ui.Text(c, "Изменения вступят в силу после перезапуска подключения.").FontSize(11).TextColor(t.Warning)
			}
		})
	})
}

func (a *App) textViewerView(c *ui.Context, kind textViewer) {
	d := a.prepare(c)
	t := d.T
	view := a.viewers[kind]
	if view == nil {
		view = &textView{Title: "Нет данных"}
		a.viewers[kind] = view
	}
	window := viewerWindow(kind)
	ui.Column(c).Fill().Padding(childWindowPadding).Gap(10).Children(func() {
		a.childHeader(c, d, view.Title, view.Path, func() {
			iconButton(c, "Обновить", iconRefreshCW, false).OnClick(func() { a.refreshTextViewer(kind) })
			iconButton(c, "Копировать", iconCopy, false).Disabled(view.Text == "").OnClick(func() { a.copyText(view.Text) })
		}, func() { a.closeChild(window) })
		if view.Err != "" {
			ui.Text(c, view.Err).FontSize(12).TextColor(t.Danger).Wrap()
		}
		text := ui.TextArea(c, &view.Text).ReadOnly(true).Font("Consolas").FontSize(12)
		text.Grow(1)
	})
}

func viewerWindow(kind textViewer) childWindow {
	switch kind {
	case viewerSingBox:
		return winSingBox
	case viewerLog:
		return winLog
	default:
		return winConfig
	}
}

func trimText(value string) string {
	return strings.TrimSpace(value)
}

func (a *App) switchRow(c *ui.Context, title string, value *bool, hint string) {
	t := c.Theme()
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(2).Grow(1).Children(func() {
			ui.Text(c, title)
			ui.Text(c, hint).TextColor(t.TextMuted).FontSize(11).Wrap()
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
			ui.Text(c, status).TextColor(t.TextMuted).FontSize(11)
		})
		tag := st.Tag
		label := "Скачать"
		if st.Installed {
			label = "Обновить"
		}
		iconButton(c, label, iconDownload, false).Disabled(a.busy || a.workBusy).OnClick(func() { a.updateRuleSet(tag) })
	})
}
