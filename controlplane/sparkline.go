// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"html/template"
	"strings"
)

// sparklineSVG renders a cost-trend series as a minimal line+area chart,
// per the dataviz skill's stat-tile trend spec: 2px line, ~10% opacity
// area fill, an emphasized endpoint (>=8px marker with a surface-color
// ring). A single series needs no legend — the card's own label already
// names what's plotted.
func sparklineSVG(points []CostTrendPoint, w, h int) template.HTML {
	if len(points) == 0 {
		return template.HTML(fmt.Sprintf(
			`<svg width="%d" height="%d" viewBox="0 0 %d %d" class="spark spark-empty"></svg>`, w, h, w, h))
	}

	if len(points) == 1 {
		cx, cy := float64(w)/2, float64(h)/2
		return template.HTML(fmt.Sprintf(
			`<svg width="%d" height="%d" viewBox="0 0 %d %d" class="spark">`+
				`<circle cx="%.1f" cy="%.1f" r="4" class="spark-dot" stroke-width="2"/>`+
				`</svg>`, w, h, w, h, cx, cy))
	}

	lo, hi := points[0].CostHigh, points[0].CostHigh
	for _, p := range points {
		if p.CostHigh < lo {
			lo = p.CostHigh
		}
		if p.CostHigh > hi {
			hi = p.CostHigh
		}
	}
	if hi == lo {
		hi = lo + 1 // avoid divide-by-zero when the series is flat
	}

	const padY = 4.0 // keep the line off the very top/bottom edge
	usableH := float64(h) - 2*padY

	xs := make([]float64, len(points))
	ys := make([]float64, len(points))
	for i, p := range points {
		xs[i] = float64(i) / float64(len(points)-1) * float64(w)
		norm := (p.CostHigh - lo) / (hi - lo)
		ys[i] = padY + (1-norm)*usableH
	}

	var line strings.Builder
	fmt.Fprintf(&line, "M%.1f,%.1f", xs[0], ys[0])
	for i := 1; i < len(xs); i++ {
		fmt.Fprintf(&line, " L%.1f,%.1f", xs[i], ys[i])
	}

	var area strings.Builder
	area.WriteString(line.String())
	fmt.Fprintf(&area, " L%.1f,%d L%.1f,%d Z", xs[len(xs)-1], h, xs[0], h)

	lastX, lastY := xs[len(xs)-1], ys[len(ys)-1]

	return template.HTML(fmt.Sprintf(
		`<svg width="%d" height="%d" viewBox="0 0 %d %d" class="spark" preserveAspectRatio="none">`+
			`<path d="%s" class="spark-area"/>`+
			`<path d="%s" class="spark-line" fill="none" stroke-width="2" vector-effect="non-scaling-stroke"/>`+
			`<circle cx="%.1f" cy="%.1f" r="4" class="spark-dot" stroke-width="2"/>`+
			`</svg>`,
		w, h, w, h, area.String(), line.String(), lastX, lastY,
	))
}
