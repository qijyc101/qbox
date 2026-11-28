package app

import (
	"github.com/egoist/mygo/ui"
)

// buttonKind selects the visual weight of an action.
type buttonKind int

const (
	buttonSecondary buttonKind = iota
	buttonPrimary
	buttonDanger
)

// baseButton is the single entry point for text buttons.
func baseButton(c *ui.Context, label string, primary bool) ui.Element {
	if primary {
		return ui.PrimaryButton(c, label)
	}
	return ui.Button(c, label)
}

// dialogButton applies the dialog-specific danger treatment.
func dialogButton(c *ui.Context, d design, label string, kind buttonKind) ui.Element {
	button := baseButton(c, label, kind == buttonPrimary)
	if kind == buttonDanger {
		button.TextColor(d.T.Danger)
	}
	return button
}

// iconButton is a text button with a leading icon.
func iconButton(c *ui.Context, label string, svg *ui.SVG, primary bool) ui.Element {
	button := baseButton(c, "", primary)
	button.Gap(iconLabelGap).Label(label).Children(func() {
		ui.Icon(c, svg).Size(iconSizeXL, iconSizeXL)
		ui.Text(c, label).FontSize(fontSizeLabel)
	})
	return button
}

// titleBarButton is the shared title-bar navigation and menu trigger.
func titleBarButton(c *ui.Context, d design, label string, svg *ui.SVG, active, chevron bool) ui.Element {
	button := ui.ButtonBase(c).Padding(spaceS, spaceL).Gap(spaceS).Radius(d.RadiusS)
	if active || button.Hovered() {
		button.Background(d.T.SurfaceHover)
	}
	if active {
		button.TextColor(d.T.Accent)
	}
	button.Children(func() {
		ui.Icon(c, svg).Size(iconSizeXL, iconSizeXL).TextColor(d.T.TextMuted)
		ui.Text(c, label).FontSize(fontSizeLabel)
		if chevron {
			ui.Icon(c, iconChevronDown).Size(iconSizeS, iconSizeS).TextColor(d.T.TextMuted)
		}
	})
	return button
}

// menuOptionsAt is the shared popover menu styling.
func menuOptionsAt(d design, open *bool, at, self ui.Anchor) ui.PopoverMenuButtonOptions {
	return ui.PopoverMenuButtonOptions{
		At:   at,
		Self: self,
		Open: open,
		Panel: func(panel ui.Element) {
			panel.Padding(spaceXS).Radius(d.RadiusM).Background(d.T.Background).Border(borderWidth, d.T.Border)
		},
		Item: func(item ui.Element) {
			item.Padding(spaceS, spaceL).Radius(d.RadiusS)
			if item.Highlighted() {
				item.Background(d.T.Accent)
				item.TextColor(d.T.AccentText)
			}
		},
	}
}

// iconAction is an icon-only button with the app's hover treatment.
func iconAction(c *ui.Context, d design, label string, svg *ui.SVG, size, iconSize float32, onClick func()) ui.Element {
	button := ui.ButtonBase(c).Size(size, size).Radius(d.RadiusS).Center().Label(label).Tooltip(label)
	if button.Hovered() {
		button.Background(d.T.SurfaceHover)
	}
	button.Children(func() {
		ui.Icon(c, svg).Size(iconSize, iconSize).TextColor(d.T.TextMuted)
	})
	if onClick != nil {
		button.OnClick(onClick)
	}
	return button
}

// fieldLabel is the caption above a form control.
func fieldLabel(c *ui.Context, d design, label string) {
	ui.Text(c, label).FontSize(fontSizeSmall).TextColor(d.T.TextMuted)
}

// baseInput is a labelled single-line input.
func baseInput(c *ui.Context, d design, label, placeholder string, value *string, onChange func()) ui.Element {
	fieldLabel(c, d, label)
	input := ui.TextInput(c, value).Label(label)
	if placeholder != "" {
		input.Placeholder(placeholder)
	}
	if onChange != nil {
		input.OnChange(onChange)
	}
	return input
}

// baseTokenField is a labelled token field.
func baseTokenField(c *ui.Context, d design, label, placeholder string, values *[]string, onChange func()) ui.Element {
	fieldLabel(c, d, label)
	field := ui.TokenField(c, values, nil).Label(label).FillWidth()
	if placeholder != "" {
		field.Placeholder(placeholder)
	}
	if onChange != nil && field.Changed() {
		onChange()
	}
	return field
}

type cardSpec struct {
	Height    float32
	Padding   float32
	Gap       float32
	FillWidth bool
}

// surfaceCard is the shared card surface.
func surfaceCard(c *ui.Context, d design, spec cardSpec, build func()) {
	padding := spec.Padding
	if padding == 0 {
		padding = cardPadding
	}
	gap := spec.Gap
	if gap == 0 {
		gap = cardGap
	}
	card := ui.Column(c)
	if spec.FillWidth {
		card.FillWidth()
	}
	if spec.Height > 0 {
		card.Height(spec.Height)
	}
	card.Padding(padding).Gap(gap).Background(d.T.Surface).Radius(d.RadiusL).
		Border(borderWidth, d.T.Border).Children(build)
}

// baseCard is the surface shared by settings and the routing editor.
func baseCard(c *ui.Context, d design, icon *ui.SVG, title string, build func()) {
	surfaceCard(c, d, cardSpec{FillWidth: true}, func() {
		ui.Row(c).Gap(spaceS).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon).Size(iconSizeL, iconSizeL).TextColor(d.T.TextMuted)
			ui.Text(c, title).FontSize(fontSizeSmall).Bold().TextColor(d.T.TextMuted)
		})
		build()
	})
}

// hintText is a muted explanatory paragraph.
func hintText(c *ui.Context, d design, text string) ui.Element {
	return ui.Text(c, text).FontSize(fontSizeSmall).TextColor(d.T.TextMuted).Wrap()
}

// errorText is a wrapped validation or operation error.
func errorText(c *ui.Context, d design, text string) ui.Element {
	return ui.Text(c, text).FontSize(fontSizeBody).TextColor(d.T.Danger).Wrap()
}

// warningText is a wrapped warning paragraph.
func warningText(c *ui.Context, d design, text string) ui.Element {
	return ui.Text(c, text).FontSize(fontSizeSmall).TextColor(d.T.Warning).Wrap()
}

// restartHint explains that changed settings wait for a restart.
func (a *App) restartHint(c *ui.Context, d design) {
	if a.needsRestart() {
		warningText(c, d, "Настройки изменены: перезапустите подключение, чтобы применить их.")
	}
}

// bodyHint is a wider muted explanatory paragraph.
func bodyHint(c *ui.Context, d design, text string) ui.Element {
	return ui.Text(c, text).FontSize(fontSizeBody).TextColor(d.T.TextMuted).Wrap()
}

// switchRow is the shared title/hint/switch row.
func switchRow(c *ui.Context, d design, title, hint string, value *bool, onChange func()) {
	ui.Row(c).Gap(spaceXL).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(spaceXXS).Grow(1).Children(func() {
			ui.Text(c, title)
			if hint != "" {
				hintText(c, d, hint)
			}
		})
		toggle := ui.Switch(c, value).Label(title)
		if onChange != nil {
			toggle.OnChange(onChange)
		}
	})
}

// emptyState is the centered placeholder used by list panels.
func emptyState(c *ui.Context, d design, icon *ui.SVG, title, body string, action func()) {
	ui.Column(c).Grow(1).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(spaceS).Padding(space3XL).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon).Size(iconSizeEmpty, iconSizeEmpty).TextColor(d.T.TextMuted)
			ui.Text(c, title).FontSize(fontSizeEmptyTitle).Bold()
			if body != "" {
				hintText(c, d, body)
			}
			if action != nil {
				action()
			}
		})
	})
}

// dialogFrame lays out the common column, header and body of a child window.
func (a *App) dialogFrame(c *ui.Context, d design, title string, close func(), body func()) {
	ui.Column(c).Fill().Padding(dialogPadding).Gap(dialogGap).Children(func() {
		a.dialogHeader(c, d, title, close)
		body()
	})
}

// dialogFooter is the shared spacer and action row at the bottom of a dialog.
func dialogFooter(c *ui.Context, d design, label string, kind buttonKind, disabled bool, action, cancel func()) {
	ui.Spacer(c)
	ui.Row(c).Gap(spaceM).Children(func() {
		ui.Spacer(c)
		dialogButton(c, d, label, kind).Disabled(disabled).OnClick(action)
		dialogButton(c, d, "Отмена", buttonSecondary).OnClick(cancel)
	})
}
