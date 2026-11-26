package app

import (
	"hash/fnv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"qbox/internal/config"
)

const (
	lightBackground  = "#f5f6f8"
	darkBackground   = "#141416"
	windowBackground = "light-dark(#f5f6f8, #141416)"

	titleBarMinHeight  = 40
	windowControlWidth = 40
)

// design carries the theme and the design tokens of a frame.
type design struct {
	T *ui.Theme

	RadiusXS float32
	RadiusS  float32
	RadiusM  float32
	RadiusL  float32

	Space1 float32
	Space2 float32
	Space3 float32
	Space4 float32
	Space5 float32
}

type accentDef struct {
	Name  string
	Label string
	Light ui.Color
	Dark  ui.Color
}

var accentDefs = []accentDef{
	{Name: config.AccentBlue, Label: "Синий", Light: ui.Hex("#2563eb"), Dark: ui.Hex("#60a5fa")},
	{Name: config.AccentViolet, Label: "Фиолетовый", Light: ui.Hex("#7c3aed"), Dark: ui.Hex("#a78bfa")},
	{Name: config.AccentEmerald, Label: "Изумрудный", Light: ui.Hex("#059669"), Dark: ui.Hex("#34d399")},
	{Name: config.AccentAmber, Label: "Янтарный", Light: ui.Hex("#d97706"), Dark: ui.Hex("#fbbf24")},
	{Name: config.AccentRose, Label: "Розовый", Light: ui.Hex("#e11d48"), Dark: ui.Hex("#fb7185")},
	{Name: config.AccentTeal, Label: "Бирюзовый", Light: ui.Hex("#0d9488"), Dark: ui.Hex("#2dd4bf")},
}

var avatarPalette = []ui.Color{
	ui.Hex("#2563eb"),
	ui.Hex("#7c3aed"),
	ui.Hex("#059669"),
	ui.Hex("#d97706"),
	ui.Hex("#e11d48"),
	ui.Hex("#0d9488"),
	ui.Hex("#4f46e5"),
	ui.Hex("#b45309"),
}

func (a *App) prepare(c *ui.Context) design {
	d := a.designFor(c)
	c.SetTheme(d.T)
	return d
}

func (a *App) designFor(c *ui.Context) design {
	dark := a.cfg.Appearance.Theme == config.ThemeDark
	if a.cfg.Appearance.Theme != config.ThemeLight && a.cfg.Appearance.Theme != config.ThemeDark {
		dark = c.Theme().Dark
	}
	var t ui.Theme
	if dark {
		t = *ui.DarkTheme()
	} else {
		t = *ui.LightTheme()
	}

	accent := accentColor(a.cfg.Appearance.Accent, dark)
	if dark {
		t.Background = ui.Hex(darkBackground)
		t.Surface = ui.Hex("#1d1e21")
		t.SurfaceHover = ui.Hex("#26272b")
		t.SurfacePressed = ui.Hex("#303136")
		t.Border = ui.Hex("#2e3035")
		t.Text = ui.Hex("#f4f4f5")
		t.TextMuted = ui.Hex("#9a9aa3")
		t.Danger = ui.Hex("#ff6369")
		t.Warning = ui.Hex("#fbbf24")
		t.Success = ui.Hex("#4ade80")
	} else {
		t.Background = ui.Hex(lightBackground)
		t.Surface = ui.Hex("#ffffff")
		t.SurfaceHover = ui.Hex("#eef0f4")
		t.SurfacePressed = ui.Hex("#e4e7ed")
		t.Border = ui.Hex("#e2e5ea")
		t.Text = ui.Hex("#18181b")
		t.TextMuted = ui.Hex("#71717a")
		t.Danger = ui.Hex("#dc2626")
		t.Warning = ui.Hex("#d97706")
		t.Success = ui.Hex("#16a34a")
	}
	t.Dark = dark
	t.Accent = accent
	t.AccentText = ui.Hex("#ffffff")
	mix := ui.Hex("#000000")
	if dark {
		mix = ui.Hex("#ffffff")
	}
	t.AccentHover = accent.Mix(mix, 0.10)
	t.AccentPressed = accent.Mix(mix, 0.18)
	t.InactiveAccent = accent
	t.Selection = accent.Alpha(0.30)
	t.Focus = accent
	t.FocusRingWidth = 2
	t.FocusRingOffset = 1
	t.Spacing = 4
	t.FontSize = 13
	radii := a.radiusPreset()
	t.Radius = radii.S
	t.Scrollbar = t.TextMuted.Alpha(0.35)

	return design{
		T:        &t,
		RadiusXS: radii.XS,
		RadiusS:  radii.S,
		RadiusM:  radii.M,
		RadiusL:  radii.L,
		Space1:   4,
		Space2:   8,
		Space3:   12,
		Space4:   16,
		Space5:   24,
	}
}

type radiusSet struct {
	XS, S, M, L float32
}

func (a *App) radiusPreset() radiusSet {
	switch a.cfg.Appearance.Corners {
	case config.CornersCompact:
		return radiusSet{XS: 4, S: 6, M: 8, L: 12}
	case config.CornersRound:
		return radiusSet{XS: 8, S: 12, M: 16, L: 20}
	default:
		return radiusSet{XS: 6, S: 8, M: 12, L: 16}
	}
}

func accentColor(name string, dark bool) ui.Color {
	for _, def := range accentDefs {
		if def.Name == name {
			if dark {
				return def.Dark
			}
			return def.Light
		}
	}
	if dark {
		return accentDefs[0].Dark
	}
	return accentDefs[0].Light
}

func avatarColor(id string) ui.Color {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return avatarPalette[int(h.Sum32())%len(avatarPalette)]
}

func avatarLetter(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "?"
	}
	for _, r := range name {
		if r == ' ' || r == '\t' {
			continue
		}
		return string(r)
	}
	return "?"
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

// applyWindowTheme updates the native window chrome for the chosen theme.
func (a *App) applyWindowTheme() {
	if a.win == nil {
		return
	}
	source := mygo.ThemeSystem
	switch a.cfg.Appearance.Theme {
	case config.ThemeLight:
		source = mygo.ThemeLight
	case config.ThemeDark:
		source = mygo.ThemeDark
	}
	mygo.Theme.SetSource(source)
	_ = a.win.SetBackgroundColor(windowBackground)
	a.win.SetWindowButtonsTheme(source)
	for _, w := range a.children {
		_ = w.SetBackgroundColor(windowBackground)
	}
}
