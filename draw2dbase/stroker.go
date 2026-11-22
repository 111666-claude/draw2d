// Copyright 2010 The draw2d Authors. All rights reserved.
// created: 13/12/2010 by Laurent Le Goff

package draw2dbase

import (
	"math"

	"github.com/llgcode/draw2d"
)

// minMiterDenom guards against nearly reversing segments (turn angle close
// to 180°): below this value of 1+cos(theta) the miter intersection is so
// far away that it is always replaced by a bevel join, keeping every
// generated coordinate bounded.
const minMiterDenom = 1e-9

type LineStroker struct {
	Flattener     Flattener
	HalfLineWidth float64
	// MiterLimit limits the length of miter joins as a multiple of the
	// line width. Joins exceeding it fall back to a bevel join.
	MiterLimit   float64
	Cap          draw2d.LineCap
	Join         draw2d.LineJoin
	vertices     []float64
	rewind       []float64
	x, y, nx, ny float64
	// started is true once the current subpath has at least one segment,
	// so a join can be emitted between it and the next one.
	started bool
}

func NewLineStroker(c draw2d.LineCap, j draw2d.LineJoin, flattener Flattener) *LineStroker {
	l := new(LineStroker)
	l.Flattener = flattener
	l.HalfLineWidth = 0.5
	l.MiterLimit = draw2d.DefaultMiterLimit
	l.Cap = c
	l.Join = j
	return l
}

func (l *LineStroker) MoveTo(x, y float64) {
	l.x, l.y = x, y
	l.started = false
}

func (l *LineStroker) LineTo(x, y float64) {
	l.line(l.x, l.y, x, y)
}

func (l *LineStroker) LineJoin() {

}

func (l *LineStroker) line(x1, y1, x2, y2 float64) {
	dx := (x2 - x1)
	dy := (y2 - y1)
	d := vectorDistance(dx, dy)
	if d != 0 {
		nx := dy * l.HalfLineWidth / d
		ny := -(dx * l.HalfLineWidth / d)
		// offset points of the new segment at the join point (x1, y1)
		vx1, vy1 := x1+nx, y1+ny
		rx1, ry1 := x1-nx, y1-ny
		if l.started && l.Join == draw2d.MiterJoin {
			if ox, oy, ok := l.miterOffset(nx, ny); ok {
				// The sign of the z component of the cross product of the
				// previous and current segment directions tells which side
				// is the outer side of the turn:
				// sign(dx1*dy2 - dy1*dx2) = -sign(nx1*dx2 + ny1*dy2)
				if l.nx*dx+l.ny*dy < 0 {
					// outer side is the +n side (vertices)
					l.vertices[len(l.vertices)-2] = x1 + ox
					l.vertices[len(l.vertices)-1] = y1 + oy
					vx1, vy1 = x1+ox, y1+oy
				} else {
					// outer side is the -n side (rewind)
					l.rewind[len(l.rewind)-2] = x1 - ox
					l.rewind[len(l.rewind)-1] = y1 - oy
					rx1, ry1 = x1-ox, y1-oy
				}
			}
			// otherwise keep the bevel join (the default outline)
		}
		l.appendVertex(vx1, vy1, x2+nx, y2+ny, rx1, ry1, x2-nx, y2-ny)
		l.x, l.y, l.nx, l.ny = x2, y2, nx, ny
		l.started = true
	}
}

// miterOffset computes the offset, from the join point, of the intersection
// of the two offset lines on the +n side, given the normal (nx, ny) of the
// new segment and the normal (l.nx, l.ny) of the previous one. The -n side
// intersection is the opposite of this offset. ok is false when the join
// must fall back to a bevel: degenerate line width, nearly reversing
// segments, or a miter longer than MiterLimit times the line width.
func (l *LineStroker) miterOffset(nx, ny float64) (ox, oy float64, ok bool) {
	h := l.HalfLineWidth
	if !(h > 0) || math.IsInf(h, 0) {
		return 0, 0, false
	}
	// denom = 1 + cos(theta), theta being the angle between the two
	// segments; the miter extends HalfLineWidth / cos(theta/2) from the
	// join point, that is a ratio of sqrt(2/denom) times the half width.
	denom := 1 + (l.nx*nx+l.ny*ny)/(h*h)
	if !(denom >= minMiterDenom) {
		// segments nearly reverse (or normals are not finite):
		// the miter intersection would be (near) unbounded
		return 0, 0, false
	}
	limit := draw2d.EffectiveMiterLimit(l.MiterLimit)
	if denom < 2/(limit*limit) {
		// miter longer than limit times the line width: bevel
		return 0, 0, false
	}
	ox = (l.nx + nx) / denom
	oy = (l.ny + ny) / denom
	return ox, oy, true
}

func (l *LineStroker) Close() {
	if len(l.vertices) > 1 {
		l.appendVertex(l.vertices[0], l.vertices[1], l.rewind[0], l.rewind[1])
	}
}

func (l *LineStroker) End() {
	if len(l.vertices) > 1 {
		l.Flattener.MoveTo(l.vertices[0], l.vertices[1])
		for i, j := 2, 3; j < len(l.vertices); i, j = i+2, j+2 {
			l.Flattener.LineTo(l.vertices[i], l.vertices[j])
		}
	}
	for i, j := len(l.rewind)-2, len(l.rewind)-1; j > 0; i, j = i-2, j-2 {
		l.Flattener.LineTo(l.rewind[i], l.rewind[j])
	}
	if len(l.vertices) > 1 {
		l.Flattener.LineTo(l.vertices[0], l.vertices[1])
	}
	l.Flattener.End()
	// reinit vertices
	l.vertices = l.vertices[0:0]
	l.rewind = l.rewind[0:0]
	l.x, l.y, l.nx, l.ny = 0, 0, 0, 0
	l.started = false

}

func (l *LineStroker) appendVertex(vertices ...float64) {
	s := len(vertices) / 2
	l.vertices = append(l.vertices, vertices[:s]...)
	l.rewind = append(l.rewind, vertices[s:]...)
}

func vectorDistance(dx, dy float64) float64 {
	return float64(math.Sqrt(dx*dx + dy*dy))
}
