package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"qbox/internal/config"
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
	height := max(bar.Height, titleBarMinHeight)
	ui.Column(c).Background(d.T.Background).Children(func() {
		ui.Row(c).Height(height).Background(d.T.Background).Padding(0, bar.Right, 0, bar.Left).DragWindow().Children(func() {
			ui.Row(c).Gap(spaceXXS).PaddingStart(spaceM).AlignItems(ui.Center).Children(func() {
				a.fileMenu(c, d)
				a.routingMenu(c, d)
				a.settingsButton(c, d)
				if a.settingsOpen {
					a.homeButton(c, d)
				}
			})
			ui.Spacer(c)
			a.windowControls(c, d, height)
		})
		ui.Box(c).Height(dividerSize).Background(d.T.Border)
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
		btn.Background(ui.Hex(windowCloseHover))
	case btn.Hovered():
		btn.Background(d.T.SurfaceHover)
	}
	btn.Children(func() {
		color := d.T.Text
		if close && btn.Hovered() {
			color = ui.Hex(colorWhite)
		}
		ui.Icon(c, svg).Size(iconSizeXL, iconSizeXL).TextColor(color)
	})
	btn.OnClick(onClick)
}

func (a *App) menuTrigger(c *ui.Context, d design, label string, svg *ui.SVG, open *bool) ui.Element {
	return titleBarButton(c, d, label, svg, *open, true)
}

func (a *App) switchMenuOnHover(trigger ui.Element, open, other *bool) {
	if *open || !*other || !trigger.Hovered() {
		return
	}
	*other = false
	*open = true
	a.invalidate()
}

func (a *App) menuOptions(d design, open *bool) ui.PopoverMenuButtonOptions {
	return menuOptionsAt(d, open, ui.AnchorBottomStart, ui.AnchorTopStart)
}

func (a *App) fileMenu(c *ui.Context, d design) {
	trigger := a.menuTrigger(c, d, "Файл", iconFileJson, &a.fileMenuOpen)
	a.switchMenuOnHover(trigger, &a.fileMenuOpen, &a.routingMenuOpen)
	row, exitAt := 0, 0
	options := a.menuOptions(d, &a.fileMenuOpen)
	baseItem := options.Item
	options.Item = func(item ui.Element) {
		row++
		baseItem(item)
		if row == exitAt && item.Highlighted() {
			item.Background(ui.Hex(windowCloseHover)).TextColor(ui.Hex(colorWhite))
		}
	}
	trigger.PopoverMenuButton(func(m *ui.Menu) {
		row, exitAt = 0, 0
		if m.Item("config.json").Icon(menuIconFile).Subtitle("настройки qbox").Chosen() {
			a.openTextViewer(viewerConfig)
		}
		exitAt++
		if m.Item("Конфигурация sing-box").Icon(menuIconRoute).Subtitle("последняя применённая").Chosen() {
			a.openTextViewer(viewerSingBox)
		}
		exitAt++
		if m.Item("Журнал qbox").Icon(menuIconScroll).Subtitle("qbox.log").Chosen() {
			a.openTextViewer(viewerLog)
		}
		exitAt++
		m.Separator()
		if m.Item("Папка qbox").Icon(menuIconFolder).Subtitle("config.json и журналы").Chosen() {
			a.openDataFolder()
		}
		exitAt++
		if m.Item("Папка логов").Icon(menuIconFolder).Subtitle("qbox.log").Chosen() {
			a.openLogFolder()
		}
		exitAt++
		m.Separator()
		if m.Item("Выход").Icon(menuIconX).Danger().Chosen() {
			mygo.App.Quit()
		}
		exitAt++
	}, options)
}

func (a *App) routingMenu(c *ui.Context, d design) {
	trigger := a.menuTrigger(c, d, "Маршрутизация", iconRoute, &a.routingMenuOpen)
	a.switchMenuOnHover(trigger, &a.routingMenuOpen, &a.fileMenuOpen)
	trigger.PopoverMenuButton(func(m *ui.Menu) {
		for i := range a.cfg.RoutingProfiles {
			profile := a.cfg.RoutingProfiles[i]
			active := profile.ID == a.cfg.ActiveRoutingProfileID
			item := m.Item(profile.Name).ID(profile.ID)
			item.Content(func(content ui.MenuItemContent) {
				pc := content.Context
				ui.Row(pc).Grow(1).AlignItems(ui.Center).Gap(spaceM).Children(func() {
					if active {
						ui.Icon(pc, iconCheck).Size(iconSizeM, iconSizeM).TextColor(content.Text)
					} else {
						ui.Box(pc).Size(iconSizeM, iconSizeM)
					}
					ui.Text(pc, profile.Name).FontSize(fontSizeLabel).Grow(1)
					edit := ui.ButtonBase(pc).Size(smallIconButtonSize, smallIconButtonSize).
						Radius(d.RadiusXS).Center().Label("Редактировать " + profile.Name)
					if edit.Hovered() {
						edit.Background(content.Muted.Alpha(mutedTintAlpha))
					}
					edit.Children(func() {
						ui.Icon(pc, iconPencil).Size(iconSizeM, iconSizeM).TextColor(content.Muted)
					})
					edit.OnClick(func() { a.editProfile(profile.ID) })
				})
			})
			if item.Chosen() {
				a.selectRoutingProfile(profile.ID)
			}
		}
		m.Separator()
		if m.Item("Новый профиль…").Icon(menuIconPlus).Disabled(len(a.cfg.RoutingProfiles) >= config.MaxRoutingProfiles).Chosen() {
			a.openNewProfile()
		}
	}, a.menuOptions(d, &a.routingMenuOpen))
}

func (a *App) editProfile(id string) {
	if id != a.cfg.ActiveRoutingProfileID {
		a.selectRoutingProfile(id)
	}
	a.openProfileEditor()
}

func (a *App) settingsButton(c *ui.Context, d design) {
	titleBarButton(c, d, "Настройки", iconSettings, a.settingsOpen, false).OnClick(func() {
		a.settingsOpen = !a.settingsOpen
		a.err = ""
		a.invalidate()
	})
}

func (a *App) homeButton(c *ui.Context, d design) {
	titleBarButton(c, d, "На главную", iconArrowLeft, false, false).OnClick(func() {
		a.settingsOpen = false
		a.invalidate()
	})
}

func (a *App) statusBanner(c *ui.Context, d design) {
	t := d.T
	if a.err == "" && a.warn == "" && a.notice == "" && a.workLabel() == "" {
		return
	}
	ui.Column(c).Children(func() {
		if a.err != "" {
			a.bannerRow(c, d, a.err, iconTriangleAlert, t.Danger, func() { a.err = "" })
		}
		if a.warn != "" {
			a.bannerRow(c, d, a.warn, iconTriangleAlert, t.Warning, nil)
		}
		if a.workLabel() != "" {
			a.bannerRow(c, d, a.workTS, iconActivity, t.TextMuted, nil)
		}
		if a.notice != "" {
			a.bannerRow(c, d, a.notice, iconInfo, t.TextMuted, func() { a.notice = "" })
		}
	})
}

func (a *App) bannerRow(c *ui.Context, d design, text string, icon *ui.SVG, color ui.Color, dismiss func()) {
	ui.Row(c).Padding(spaceS, spaceXL).Gap(spaceM).AlignItems(ui.Center).Background(d.T.Surface).Children(func() {
		ui.Icon(c, icon).Size(iconSizeL, iconSizeL).TextColor(color)
		ui.Text(c, text).FontSize(fontSizeBody).TextColor(color).Grow(1).Wrap()
		if dismiss != nil {
			iconAction(c, d, "Скрыть", iconX, smallIconButtonSize, iconSizeM, dismiss)
		}
	})
}
