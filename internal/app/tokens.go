package app

import (
	"hash/fnv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"qbox/internal/config"
)

const (
	colorWhite = "#ffffff"
	colorBlack = "#000000"

	lightBackground  = "#f5f6f8"
	darkBackground   = "#141416"
	windowBackground = "light-dark(#f5f6f8, #141416)"

	lightSurface        = "#ffffff"
	lightSurfaceHover   = "#eef0f4"
	lightSurfacePressed = "#e4e7ed"
	lightBorder         = "#e2e5ea"
	lightText           = "#18181b"
	lightTextMuted      = "#71717a"
	lightDanger         = "#dc2626"
	lightWarning        = "#d97706"
	lightSuccess        = "#16a34a"

	darkSurface        = "#1d1e21"
	darkSurfaceHover   = "#26272b"
	darkSurfacePressed = "#303136"
	darkBorder         = "#2e3035"
	darkText           = "#f4f4f5"
	darkTextMuted      = "#9a9aa3"
	darkDanger         = "#ff6369"
	darkWarning        = "#fbbf24"
	darkSuccess        = "#4ade80"

	titleBarMinHeight  float32 = 40
	windowControlWidth float32 = 40
	dividerSize        float32 = 1
	borderWidth        float32 = 1
	selectionBorder    float32 = 2
	chartMinSize       float32 = 2
	numberStep         float64 = 1
	windowCloseHover           = "#e81123"

	mainWindowWidth        = 1120
	mainWindowHeight       = 760
	mainWindowMinWidth     = 1000
	mainWindowMinHeight    = 600
	mainWindowCornerRadius = 12

	trayIconMaxDimension = 1024
	trayIconMaxBytes     = 1 << 20
	configViewerLimit    = 4 << 20
	logViewerTail        = 256 << 10

	accentTintAlpha  float32 = 0.10
	accentHoverMix   float32 = 0.10
	accentPressedMix float32 = 0.18
	selectionAlpha   float32 = 0.30
	mutedTintAlpha   float32 = 0.18
	chartGridAlpha   float32 = 0.55
	chartDownLine    float32 = 1.6
	chartUpLine      float32 = 1.2

	spaceXXS float32 = 2
	spaceXS  float32 = 4
	spaceS   float32 = 6
	spaceM   float32 = 8
	spaceL   float32 = 10
	spaceXL  float32 = 12
	space2XL float32 = 16
	space3XL float32 = 20

	fontSizeCaption     float32 = 10
	fontSizeSmall       float32 = 11
	fontSizeBody        float32 = 12
	fontSizeLabel       float32 = 13
	fontSizeAvatar      float32 = 14
	fontSizeEmptyTitle  float32 = 15
	fontSizeDialogTitle float32 = 18
	fontSizePageTitle   float32 = 20

	iconLabelGap  float32 = 7
	iconSizeXS    float32 = 10
	iconSizeS     float32 = 11
	iconSizeM     float32 = 12
	iconSizeL     float32 = 13
	iconSizeXL    float32 = 14
	iconSize2XL   float32 = 15
	iconSizeEmpty float32 = 24

	sidebarWidth      float32 = 270
	contentPadding    float32 = 12
	cardPadding       float32 = 14
	cardGap           float32 = 10
	dialogPadding     float32 = 14
	dialogGap         float32 = 10
	trafficCardHeight float32 = 170
	trafficChartPad   float32 = 4
	trafficGridLines  int     = 4
	trafficMinScale   float64 = 1024
	trafficHeadroom   float64 = 1.12
	logLevelWidth     float32 = 48

	avatarSize          float32 = 32
	statusDotSize       float32 = 6
	activityDotSize     float32 = 7
	statusDotRadius     float32 = 3
	activityDotRadius   float32 = 4
	nodeAccentWidth     float32 = 3
	nodeAccentHeight    float32 = 24
	closeButtonSize     float32 = 28
	smallIconButtonSize float32 = 22
	menuButtonSize      float32 = 26

	dateTimeLayout = "02.01.2006 15:04:05"
	dateLayout     = "2006-01-02"
	clockLayout    = "15:04:05"
	monoFont       = "Consolas"
)

// design carries the theme and the design tokens of a frame.
type design struct {
	T *ui.Theme

	RadiusXS float32
	RadiusS  float32
	RadiusM  float32
	RadiusL  float32
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
		t.Surface = ui.Hex(darkSurface)
		t.SurfaceHover = ui.Hex(darkSurfaceHover)
		t.SurfacePressed = ui.Hex(darkSurfacePressed)
		t.Border = ui.Hex(darkBorder)
		t.Text = ui.Hex(darkText)
		t.TextMuted = ui.Hex(darkTextMuted)
		t.Danger = ui.Hex(darkDanger)
		t.Warning = ui.Hex(darkWarning)
		t.Success = ui.Hex(darkSuccess)
	} else {
		t.Background = ui.Hex(lightBackground)
		t.Surface = ui.Hex(lightSurface)
		t.SurfaceHover = ui.Hex(lightSurfaceHover)
		t.SurfacePressed = ui.Hex(lightSurfacePressed)
		t.Border = ui.Hex(lightBorder)
		t.Text = ui.Hex(lightText)
		t.TextMuted = ui.Hex(lightTextMuted)
		t.Danger = ui.Hex(lightDanger)
		t.Warning = ui.Hex(lightWarning)
		t.Success = ui.Hex(lightSuccess)
	}
	t.Dark = dark
	t.Accent = accent
	t.AccentText = ui.Hex(colorWhite)
	mix := ui.Hex(colorBlack)
	if dark {
		mix = ui.Hex(colorWhite)
	}
	t.AccentHover = accent.Mix(mix, accentHoverMix)
	t.AccentPressed = accent.Mix(mix, accentPressedMix)
	t.InactiveAccent = accent
	t.Selection = accent.Alpha(selectionAlpha)
	t.Focus = accent
	t.FocusRingWidth = spaceXXS
	t.FocusRingOffset = dividerSize
	t.Spacing = spaceXS
	t.FontSize = fontSizeLabel
	radii := a.radiusPreset()
	t.Radius = radii.S
	t.Scrollbar = t.TextMuted.Alpha(0.35)

	return design{
		T:        &t,
		RadiusXS: radii.XS,
		RadiusS:  radii.S,
		RadiusM:  radii.M,
		RadiusL:  radii.L,
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
