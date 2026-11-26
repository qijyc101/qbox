package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"qbox/internal/config"
	"qbox/internal/model"
	"qbox/internal/rulesets"
)

func (a *App) addSubscriptionView(c *ui.Context) {
	d := a.prepare(c)
	t := d.T
	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		ui.Text(c, "Новая подписка").FontSize(18).Bold()
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
			iconButton(c, "Импортировать", iconDownload, true).
				Disabled(a.busy || a.workBusy || trimText(a.addURL) == "").
				OnClick(a.importSubscription)
			iconButton(c, "Отмена", iconX, false).OnClick(func() { a.closeChild(winAddSubscription) })
		})
	})
}

func (a *App) renameSubscriptionView(c *ui.Context) {
	d := a.prepare(c)
	t := d.T
	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		ui.Text(c, "Переименовать подписку").FontSize(18).Bold()
		ui.Text(c, "Название").FontSize(11).TextColor(t.TextMuted)
		ui.TextInput(c, &a.subNameDraft).Label("Название")
		if a.err != "" {
			ui.Text(c, a.err).FontSize(12).TextColor(t.Danger).Wrap()
		}
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(8).Children(func() {
			iconButton(c, "Сохранить", iconCheck, true).OnClick(a.renameSubscription)
			iconButton(c, "Отмена", iconX, false).OnClick(func() { a.closeChild(winRenameSub) })
		})
	})
}

func (a *App) deleteSubscriptionView(c *ui.Context) {
	d := a.prepare(c)
	name := a.subDeleting
	if sub := a.cfg.Subscription(a.subDeleting); sub != nil {
		name = sub.Name
	}
	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		ui.Text(c, "Удалить подписку?").FontSize(18).Bold()
		ui.Text(c, "«"+name+"» и её ноды будут удалены. Это действие нельзя отменить.").
			FontSize(12).TextColor(d.T.Warning).Wrap()
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(8).Children(func() {
			iconButton(c, "Удалить", iconTrash2, false).TextColor(d.T.Danger).OnClick(a.confirmDeleteSubscription)
			iconButton(c, "Отмена", iconX, false).OnClick(func() { a.closeChild(winDeleteSub) })
		})
	})
}

func (a *App) newProfileView(c *ui.Context) {
	d := a.prepare(c)
	t := d.T
	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		ui.Text(c, "Новый профиль").FontSize(18).Bold()
		ui.Text(c, "Профиль начнёт с текущих правил; их можно изменить отдельно.").FontSize(12).TextColor(t.TextMuted).Wrap()
		ui.Text(c, "Название профиля").FontSize(11).TextColor(t.TextMuted)
		ui.TextInput(c, &a.profileNameDraft).Label("Название профиля")
		if a.err != "" {
			ui.Text(c, a.err).FontSize(12).TextColor(t.Danger).Wrap()
		}
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(8).Children(func() {
			iconButton(c, "Создать", iconPlus, true).OnClick(a.saveNewProfile)
			iconButton(c, "Отмена", iconX, false).OnClick(func() { a.closeChild(winNewProfile) })
		})
	})
}

func (a *App) renameProfileView(c *ui.Context) {
	d := a.prepare(c)
	t := d.T
	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		ui.Text(c, "Переименовать профиль").FontSize(18).Bold()
		ui.Text(c, "Название профиля").FontSize(11).TextColor(t.TextMuted)
		ui.TextInput(c, &a.profileNameDraft).Label("Название профиля")
		if a.err != "" {
			ui.Text(c, a.err).FontSize(12).TextColor(t.Danger).Wrap()
		}
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(8).Children(func() {
			iconButton(c, "Сохранить", iconCheck, true).OnClick(a.saveRoutingProfileName)
			iconButton(c, "Отмена", iconX, false).OnClick(func() { a.closeChild(winRenameProfile) })
		})
	})
}

func (a *App) deleteProfileView(c *ui.Context) {
	d := a.prepare(c)
	name := a.profileDeleting
	if profile := a.cfg.RoutingProfile(a.profileDeleting); profile != nil {
		name = profile.Name
	}
	ui.Column(c).Fill().Padding(16).Gap(10).Children(func() {
		ui.Text(c, "Удалить профиль?").FontSize(18).Bold()
		ui.Text(c, "Профиль «"+name+"» будет удалён. Активным станет соседний профиль.").
			FontSize(12).TextColor(d.T.Warning).Wrap()
		ui.Box(c).Grow(1)
		ui.Row(c).Gap(8).Children(func() {
			iconButton(c, "Удалить профиль", iconTrash2, false).TextColor(d.T.Danger).OnClick(a.confirmDeleteRoutingProfile)
			iconButton(c, "Отмена", iconX, false).OnClick(func() { a.closeChild(winDeleteProfile) })
		})
	})
}

func (a *App) routingEditorView(c *ui.Context) {
	d := a.prepare(c)
	t := d.T
	profile := a.activeRoutingProfileName()
	ui.Column(c).Fill().Padding(14).Gap(10).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Маршруты").FontSize(18).Bold()
			if profile != "" {
				ui.Text(c, "профиль: "+profile).FontSize(12).TextColor(t.TextMuted)
			}
			ui.Text(c, "").Grow(1)
			if a.state == model.StateRunning {
				iconButton(c, "Перезапустить", iconRotateCW, true).Disabled(a.busy || a.workBusy).OnClick(a.restartNow)
			}
			iconButton(c, "Готово", iconCheck, false).OnClick(func() { a.closeChild(winRouting) })
		})
		ui.Box(c).Height(1).Background(t.Border)
		ui.Scroll(c).Grow(1).Background(t.Background).Gap(12).Children(func() {
			ui.Text(c, "Режим").FontSize(15).Bold()
			mode := a.modeIndex()
			ui.Segmented(c, &mode, "Global", "Rules", "Direct").FillEqually().OnChange(func() {
				a.setMode(mode)
			})
			ui.Text(c, "Global — весь трафик через прокси; LAN и блокировка рекламы действуют по настройкам. Rules — списки и гео-правила; Direct — без прокси.").
				FontSize(11).TextColor(t.TextMuted).Wrap()

			a.switchRow(c, "Обход локальной сети", &a.cfg.Routing.BypassLAN, "Приватные IP идут напрямую.")
			a.switchRow(c, "Обход РФ", &a.cfg.Routing.BypassRU, "Домены и IP РФ идут напрямую; DNS для доменов РФ — системный.")
			a.switchRow(c, "Блокировать рекламу", &a.cfg.Routing.BlockAds, "geosite-category-ads-all отклоняется.")

			ui.Text(c, "Базы правил").FontSize(15).Bold()
			for _, st := range a.deps.RuleSets.Status() {
				a.ruleSetRow(c, st)
			}
			iconButton(c, "Обновить все базы", iconDownload, false).Disabled(a.busy || a.workBusy).OnClick(a.updateAllRuleSets)

			ui.Text(c, "Свои списки").FontSize(15).Bold()
			ui.Text(c, "Списки применяются только в режиме Rules. Порядок: proxy → свои direct-списки → обход РФ → финальный маршрут.").
				FontSize(11).TextColor(t.TextMuted).Wrap()
			ui.Text(c, "Proxy: домены").FontSize(11).TextColor(t.TextMuted)
			ui.TextArea(c, &a.proxyText).Lines(2, 5).OnChange(func() {
				a.cfg.Routing.ProxyDomains = config.SplitList(a.proxyText)
				a.routingChanged()
			})
			ui.Text(c, "Proxy: IP/CIDR").FontSize(11).TextColor(t.TextMuted)
			ui.TextArea(c, &a.proxyCIDRs).Lines(2, 4).OnChange(func() {
				a.cfg.Routing.ProxyCIDRs = config.SplitList(a.proxyCIDRs)
				a.routingChanged()
			})
			ui.Text(c, "Direct: домены").FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, "Домены и их поддомены идут напрямую; DNS для них резолвится системным (корпоративным) сервером.").
				FontSize(11).TextColor(t.TextMuted).Wrap()
			ui.TextArea(c, &a.directText).Lines(2, 5).OnChange(func() {
				a.cfg.Routing.DirectDomains = config.SplitList(a.directText)
				a.routingChanged()
			})
			ui.Text(c, "Direct: IP/CIDR").FontSize(11).TextColor(t.TextMuted)
			ui.TextArea(c, &a.directCIDRs).Lines(2, 4).OnChange(func() {
				a.cfg.Routing.DirectCIDRs = config.SplitList(a.directCIDRs)
				a.routingChanged()
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
	ui.Column(c).Fill().Padding(12).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, view.Title).FontSize(15).Bold()
			if view.Path != "" {
				ui.Text(c, view.Path).FontSize(11).TextColor(t.TextMuted)
			}
			ui.Text(c, "").Grow(1)
			iconButton(c, "Обновить", iconRefreshCW, false).OnClick(func() { a.refreshTextViewer(kind) })
			iconButton(c, "Копировать", iconCopy, false).Disabled(view.Text == "").OnClick(func() { a.copyText(view.Text) })
			iconButton(c, "Закрыть", iconX, false).OnClick(func() { a.closeChild(window) })
		})
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
