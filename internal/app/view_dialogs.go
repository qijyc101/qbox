package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"qbox/internal/model"
	"qbox/internal/rulesets"
)

func (a *App) childHeader(c *ui.Context, d design, title, subtitle string, actions func(), close func()) {
	ui.Row(c).FillWidth().Gap(spaceM).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Text(c, title).FontSize(fontSizeDialogTitle).Bold()
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(fontSizeSmall).TextColor(d.T.TextMuted).SingleLine().Grow(1)
		} else {
			ui.Spacer(c)
		}
		if actions != nil {
			actions()
		}
		a.childCloseButton(c, d, close)
	})
}

func (a *App) childCloseButton(c *ui.Context, d design, close func()) {
	btn := ui.ButtonBase(c).Size(closeButtonSize, closeButtonSize).Radius(d.RadiusS).Center().
		Label("Закрыть").Tooltip("Закрыть")
	if btn.Hovered() {
		btn.Background(ui.Hex(windowCloseHover))
	} else {
		btn.Background(d.T.SurfaceHover)
	}
	btn.Children(func() {
		color := d.T.TextMuted
		if btn.Hovered() {
			color = ui.Hex(colorWhite)
		}
		ui.Icon(c, iconX).Size(iconSizeXL, iconSizeXL).TextColor(color)
	})
	btn.OnClick(close)
}

func (a *App) dialogHeader(c *ui.Context, d design, title string, close func()) {
	a.childHeader(c, d, title, "", nil, close)
}

func (a *App) addSubscriptionView(c *ui.Context) {
	d := a.prepare(c)
	close := func() { a.closeChild(winAddSubscription) }
	a.dialogFrame(c, d, "Новая подписка", close, func() {
		bodyHint(c, d, "Поддерживаются base64/URI-списки и sing-box JSON.")
		baseInput(c, d, "Название", "например, Provider", &a.addName, nil)
		baseInput(c, d, "URL", "https://example.com/sub", &a.addURL, nil)
		if a.workLabel() != "" {
			hintText(c, d, a.workTS)
		}
		if a.err != "" {
			errorText(c, d, a.err)
		}
		dialogFooter(c, d, "Импортировать", buttonPrimary,
			a.working() || trimText(a.addURL) == "",
			a.importSubscription, close)
	})
}

func (a *App) renameSubscriptionView(c *ui.Context) {
	d := a.prepare(c)
	close := func() { a.closeChild(winRenameSub) }
	a.dialogFrame(c, d, "Переименовать подписку", close, func() {
		baseInput(c, d, "Название", "", &a.subNameDraft, nil)
		if a.err != "" {
			errorText(c, d, a.err)
		}
		dialogFooter(c, d, "Сохранить", buttonPrimary, false, a.renameSubscription, close)
	})
}

func (a *App) deleteSubscriptionView(c *ui.Context) {
	d := a.prepare(c)
	name := a.subDeleting
	if sub := a.cfg.Subscription(a.subDeleting); sub != nil {
		name = sub.Name
	}
	close := func() { a.closeChild(winDeleteSub) }
	a.dialogFrame(c, d, "Удалить подписку?", close, func() {
		warningText(c, d, "«"+name+"» и её ноды будут удалены. Это действие нельзя отменить.")
		dialogFooter(c, d, "Удалить", buttonDanger, false, a.confirmDeleteSubscription, close)
	})
}

func (a *App) newProfileView(c *ui.Context) {
	d := a.prepare(c)
	close := func() { a.closeChild(winNewProfile) }
	a.dialogFrame(c, d, "Новый профиль", close, func() {
		bodyHint(c, d, "Профиль начнёт с текущих правил; их можно изменить отдельно.")
		baseInput(c, d, "Название профиля", "", &a.profileNameDraft, nil)
		if a.err != "" {
			errorText(c, d, a.err)
		}
		dialogFooter(c, d, "Создать", buttonPrimary, false, a.saveNewProfile, close)
	})
}

func (a *App) routingEditorView(c *ui.Context) {
	d := a.prepare(c)
	profile := a.activeRoutingProfileName()
	ui.Column(c).Fill().Padding(dialogPadding).Gap(dialogGap).Children(func() {
		a.childHeader(c, d, "Профиль", profile, func() {
			if a.state == model.StateRunning {
				iconButton(c, "Перезапустить", iconRotateCW, true).
					Disabled(a.working()).OnClick(a.restartNow)
			}
		}, func() { a.closeChild(winRouting) })
		ui.Scroll(c).Grow(1).Background(d.T.Background).Gap(spaceXL).Children(func() {
			baseCard(c, d, iconPencil, "Название", func() {
				fieldLabel(c, d, "Имя профиля")
				ui.Row(c).Gap(spaceM).AlignItems(ui.End).Children(func() {
					ui.TextInput(c, &a.profileNameDraft).Label("Название профиля").Grow(1)
					iconButton(c, "Сохранить", iconCheck, true).OnClick(a.saveRoutingProfileName)
				})
				if a.err != "" {
					errorText(c, d, a.err)
				}
			})

			baseCard(c, d, iconRoute, "Режим", func() {
				mode := a.modeIndex()
				ui.Segmented(c, &mode, "Global", "Rules", "Direct").AlignSelf(ui.Start).OnChange(func() {
					a.setMode(mode)
				})
				hintText(c, d, "Global — весь трафик через прокси; LAN и блокировка рекламы действуют по настройкам. Rules — списки и гео-правила; Direct — без прокси.")
			})

			baseCard(c, d, iconGlobe, "Правила", func() {
				switchRow(c, d, "Обход локальной сети", "Приватные IP идут напрямую.", &a.cfg.Routing.BypassLAN, a.routingChanged)
				switchRow(c, d, "Обход РФ", "Домены и IP РФ идут напрямую; DNS для доменов РФ — системный.", &a.cfg.Routing.BypassRU, a.routingChanged)
				switchRow(c, d, "Блокировать рекламу", "geosite-category-ads-all отклоняется.", &a.cfg.Routing.BlockAds, a.routingChanged)
			})

			baseCard(c, d, iconServer, "Базы правил", func() {
				for _, st := range a.deps.RuleSets.Status() {
					a.ruleSetRow(c, d, st)
				}
				iconButton(c, "Обновить все базы", iconDownload, false).AlignSelf(ui.End).
					Disabled(a.working()).OnClick(a.updateAllRuleSets)
			})

			baseCard(c, d, iconScrollText, "Свои списки", func() {
				hintText(c, d, "Списки применяются только в режиме Rules. Введите значение и нажмите Enter; чипы можно удалять. Порядок: proxy → свои direct-списки → обход РФ → финальный маршрут.")
				baseTokenField(c, d, "Proxy: домены", "Добавить домен…", &a.cfg.Routing.ProxyDomains, a.routingChanged)
				baseTokenField(c, d, "Proxy: IP/CIDR", "Добавить IP или CIDR…", &a.cfg.Routing.ProxyCIDRs, a.routingChanged)
				baseTokenField(c, d, "Direct: домены", "Добавить домен…", &a.cfg.Routing.DirectDomains, a.routingChanged)
				hintText(c, d, "Домены и их поддомены идут напрямую; DNS для них резолвится системным (корпоративным) сервером.")
				baseTokenField(c, d, "Direct: IP/CIDR", "Добавить IP или CIDR…", &a.cfg.Routing.DirectCIDRs, a.routingChanged)
			})

			baseCard(c, d, iconTrash2, "Удаление профиля", func() {
				switch {
				case len(a.cfg.RoutingProfiles) <= 1:
					hintText(c, d, "Единственный профиль удалить нельзя.")
				case a.profileDeleteConfirm:
					warningText(c, d, "Удалить профиль «"+profile+"»? Это действие нельзя отменить.")
					ui.Row(c).Gap(spaceM).Children(func() {
						ui.Spacer(c)
						dialogButton(c, d, "Удалить", buttonDanger).OnClick(a.deleteActiveProfile)
						dialogButton(c, d, "Отмена", buttonSecondary).OnClick(func() {
							a.profileDeleteConfirm = false
							a.invalidate()
						})
					})
				default:
					hintText(c, d, "Профиль будет удалён вместе со своими правилами; активным станет соседний профиль.")
					dialogButton(c, d, "Удалить профиль", buttonDanger).AlignSelf(ui.End).OnClick(func() {
						a.profileDeleteConfirm = true
						a.invalidate()
					})
				}
			})

			a.restartHint(c, d)
		})
	})
}

func (a *App) textViewerView(c *ui.Context, kind textViewer) {
	d := a.prepare(c)
	view := a.viewers[kind]
	if view == nil {
		view = &textView{Title: "Нет данных"}
		a.viewers[kind] = view
	}
	window := viewerWindow(kind)
	ui.Column(c).Fill().Padding(dialogPadding).Gap(dialogGap).Children(func() {
		a.childHeader(c, d, view.Title, view.Path, func() {
			iconButton(c, "Обновить", iconRefreshCW, false).OnClick(func() { a.refreshTextViewer(kind) })
			iconButton(c, "Копировать", iconCopy, false).Disabled(view.Text == "").OnClick(func() { a.copyText(view.Text) })
		}, func() { a.closeChild(window) })
		if view.Err != "" {
			errorText(c, d, view.Err)
		}
		text := ui.TextArea(c, &view.Text).ReadOnly(true).Font(monoFont).FontSize(fontSizeBody)
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

func (a *App) ruleSetRow(c *ui.Context, d design, st rulesets.Status) {
	ui.Row(c).Gap(spaceXL).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(spaceXXS).Grow(1).Children(func() {
			ui.Text(c, st.Title)
			status := "не установлена"
			if st.Installed {
				status = fmt.Sprintf("%s · %s", formatBytes(st.Size), st.Updated.Format(dateLayout))
			}
			hintText(c, d, status)
		})
		tag := st.Tag
		label := "Скачать"
		if st.Installed {
			label = "Обновить"
		}
		iconButton(c, label, iconDownload, false).Disabled(a.working()).
			OnClick(func() { a.updateRuleSet(tag) })
	})
}
