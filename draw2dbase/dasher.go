// Copyright 2010 The draw2d Authors. All rights reserved.
// created: 13/12/2010 by Laurent Le Goff

package draw2dbase

import (
	"math"
)

type DashVertexConverter struct {
	next           Flattener
	x, y, distance float64
	dash           []float64
	dashTotal      float64
	currentDash    int
	dashOffset     float64
	budget         int
}

// NewDashConverter creates a dasher emitting dash on/off transitions to
// flattener. An invalid dash pattern (empty, or containing non-finite,
// zero or negative lengths) is rejected: the converter then behaves like a
// pass-through and draws solid lines instead of looping forever.
func NewDashConverter(dash []float64, dashOffset float64, flattener Flattener) *DashVertexConverter {
	var dasher DashVertexConverter
	if validDashPattern(dash) {
		dasher.dash = dash
		for _, d := range dash {
			dasher.dashTotal += d
		}
	}
	dasher.currentDash = 0
	dasher.dashOffset = dashOffset
	dasher.next = flattener
	dasher.budget = maxDashTransitions
	return &dasher
}

func (dasher *DashVertexConverter) LineTo(x, y float64) {
	dasher.lineTo(x, y)
}

func (dasher *DashVertexConverter) MoveTo(x, y float64) {
	if !isFinite(x, y) {
		return
	}
	dasher.next.MoveTo(x, y)
	dasher.x, dasher.y = x, y
	dasher.distance = dasher.dashOffset
	dasher.currentDash = 0
	if dasher.dash != nil {
		dasher.distance = reduceDashDistance(dasher.distance, dasher.dashTotal)
	}
}

// reduceDashDistance reduces a consumed dash distance modulo the dash
// pattern length in constant time. Walking it down dash by dash would take
// O(distance) time, which hangs on huge offsets or segment lengths.
// Negative distances (negative dash offsets) are kept as-is.
func reduceDashDistance(distance, total float64) float64 {
	if !isFinite(distance) {
		return 0
	}
	if distance > 0 && total > 0 && !math.IsInf(total, 0) {
		return math.Mod(distance, total)
	}
	return distance
}

func (dasher *DashVertexConverter) LineJoin() {
	dasher.next.LineJoin()
}

func (dasher *DashVertexConverter) Close() {
	dasher.next.Close()
}

func (dasher *DashVertexConverter) End() {
	dasher.next.End()
}

func (dasher *DashVertexConverter) lineTo(x, y float64) {
	if !isFinite(x, y) {
		return
	}
	if dasher.dash == nil {
		// No (valid) dash pattern: solid line.
		dasher.next.LineTo(x, y)
		return
	}
	dasher.distance = reduceDashDistance(dasher.distance, dasher.dashTotal)
	rest := dasher.dash[dasher.currentDash] - dasher.distance
	for rest < 0 {
		dasher.distance = dasher.distance - dasher.dash[dasher.currentDash]
		dasher.currentDash = (dasher.currentDash + 1) % len(dasher.dash)
		rest = dasher.dash[dasher.currentDash] - dasher.distance
	}
	d := distance(dasher.x, dasher.y, x, y)
	if !isFinite(d) {
		// Non-finite segment length: skip dashing, keep a stable state.
		dasher.x, dasher.y = x, y
		return
	}
	for d >= rest && d > 0 && dasher.budget > 0 {
		dasher.budget--
		k := rest / d
		lx := dasher.x + k*(x-dasher.x)
		ly := dasher.y + k*(y-dasher.y)
		if dasher.currentDash%2 == 0 {
			// line
			dasher.next.LineTo(lx, ly)
		} else {
			// gap
			dasher.next.End()
			dasher.next.MoveTo(lx, ly)
		}
		d = d - rest
		dasher.x, dasher.y = lx, ly
		dasher.currentDash = (dasher.currentDash + 1) % len(dasher.dash)
		rest = dasher.dash[dasher.currentDash]
	}
	dasher.distance = d
	if dasher.currentDash%2 == 0 {
		// line
		dasher.next.LineTo(x, y)
	} else {
		// gap
		dasher.next.End()
		dasher.next.MoveTo(x, y)
	}
	if dasher.distance >= dasher.dash[dasher.currentDash] {
		dasher.distance = dasher.distance - dasher.dash[dasher.currentDash]
		dasher.currentDash = (dasher.currentDash + 1) % len(dasher.dash)
	}
	dasher.x, dasher.y = x, y
}

func distance(x1, y1, x2, y2 float64) float64 {
	return vectorDistance(x2-x1, y2-y1)
}
