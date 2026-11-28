package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"qbox/internal/model"
)

func (a *App) trafficCard(c *ui.Context, d design) {
	t := d.T
	surfaceCard(c, d, cardSpec{
		Height:  trafficCardHeight,
		Padding: contentPadding,
		Gap:     spaceS,
	}, func() {
		ui.Row(c).Gap(spaceM).AlignItems(ui.Center).Children(func() {
			color, stateIcon := stateVisual(t, a.state)
			ui.Row(c).Gap(spaceS).AlignItems(ui.Center).Children(func() {
				if stateIcon != nil {
					ui.Icon(c, stateIcon).Size(iconSizeL, iconSizeL).TextColor(color)
				} else {
					ui.Box(c).Size(activityDotSize, activityDotSize).Radius(activityDotRadius).Background(color)
				}
				ui.Text(c, a.stateText()).FontSize(fontSizeBody).TextColor(color)
			})
			if !a.cfg.TUN.Enabled && a.proxyAddr != "" && a.state == model.StateRunning {
				ui.Text(c, a.proxyAddr).FontSize(fontSizeSmall).TextColor(t.TextMuted)
			}
			ui.Spacer(c)
			trafficMetric(c, iconArrowUp, formatBytes(a.stats.Up), t.TextMuted)
			trafficMetric(c, iconArrowDown, formatBytes(a.stats.Down), t.Accent)
		})
		ui.Box(c).Grow(1).Draw(func(p *ui.Painter, r ui.Rect) {
			a.drawTrafficChart(p, r, d)
		})
		ui.Row(c).Gap(spaceXL).AlignItems(ui.Center).Children(func() {
			trafficLegend(c, iconArrowDown, "принято", t.Accent)
			trafficLegend(c, iconArrowUp, "отдано", t.TextMuted)
			ui.Spacer(c)
			period := fmt.Sprintf("%d секунд", trafficPoints)
			if n := len(a.traffic); n > 1 && n < trafficPoints {
				period = fmt.Sprintf("последние %d с", n)
			}
			ui.Text(c, period).FontSize(fontSizeCaption).TextColor(t.TextMuted)
		})
	})
}

func stateVisual(t *ui.Theme, state model.State) (ui.Color, *ui.SVG) {
	switch state {
	case model.StateRunning:
		return t.Success, iconCircleCheck
	case model.StateError:
		return t.Danger, iconCircleAlert
	case model.StateStarting, model.StateStopping:
		return t.Warning, iconActivity
	default:
		return t.TextMuted, nil
	}
}

func trafficMetric(c *ui.Context, icon *ui.SVG, value string, color ui.Color) {
	ui.Row(c).Gap(spaceXS).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon).Size(iconSizeM, iconSizeM).TextColor(color)
		ui.Text(c, value).FontSize(fontSizeBody).TextColor(color)
	})
}

func trafficLegend(c *ui.Context, icon *ui.SVG, label string, color ui.Color) {
	ui.Row(c).Gap(spaceXS).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon).Size(iconSizeXS, iconSizeXS).TextColor(color)
		ui.Text(c, label).FontSize(fontSizeCaption).TextColor(color)
	})
}

func (a *App) drawTrafficChart(p *ui.Painter, r ui.Rect, d design) {
	t := d.T
	if r.W <= chartMinSize || r.H <= chartMinSize {
		return
	}
	padding := trafficChartPad
	left := r.X + padding
	top := r.Y + padding
	width := r.W - padding*2
	height := r.H - padding*2
	for i := 1; i < trafficGridLines; i++ {
		y := top + height*float32(i)/float32(trafficGridLines)
		p.Line(left, y, left+width, y, 1, t.Border.Alpha(chartGridAlpha))
	}
	if len(a.traffic) < 2 {
		p.Text(left, top+spaceXL, "нет данных", fontSizeCaption, t.TextMuted)
		return
	}
	maxRate := float64(a.trafficPeak())
	if maxRate < trafficMinScale {
		maxRate = trafficMinScale
	}
	maxRate *= trafficHeadroom
	step := width / float32(len(a.traffic)-1)
	down := make([][2]float32, 0, len(a.traffic))
	up := make([][2]float32, 0, len(a.traffic))
	for i, point := range a.traffic {
		x := left + float32(i)*step
		down = append(down, [2]float32{x, top + height - float32(float64(point.Down)/maxRate)*height})
		up = append(up, [2]float32{x, top + height - float32(float64(point.Up)/maxRate)*height})
	}
	p.StrokePolyline(down, chartDownLine, t.Accent)
	p.StrokePolyline(up, chartUpLine, t.TextMuted)
}
