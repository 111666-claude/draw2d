// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dbase

import (
	"image/color"
	"math"

	"github.com/llgcode/draw2d"
)

// MarkerVertex is a path vertex together with the direction of the path at
// that vertex. Angle is in radians, clockwise, in the y-down coordinate
// system used by draw2d and SVG (0 points in +x direction).
type MarkerVertex struct {
	X, Y  float64
	Angle float64
}

// MarkerVertices holds the vertices of a path at which start, mid and end
// markers are placed.
type MarkerVertices struct {
	Start, End       MarkerVertex
	HasStart, HasEnd bool
	Mids             []MarkerVertex
}

// tangentEpsilon is the squared length below which a segment is considered
// zero-length, and below which the sum of the incoming and outgoing unit
// tangents is considered zero (a 180° hairpin turn), where the bisector
// direction is indeterminate.
const tangentEpsilon = 1e-12

// ComputeMarkerVertices flattens path and computes the position and tangent
// direction of every marker vertex, following the SVG marker placement rules:
// every subpath gets a start and an end marker vertex, and every interior
// vertex gets a mid marker whose direction is the bisector of the incoming
// and outgoing segments. For closed subpaths the (coincident) first vertex
// carries both the start marker, oriented along the first segment, and the
// end marker, oriented along the closing segment.
//
// Degenerate cases are resolved deterministically: zero-length segments are
// collapsed before tangents are computed, subpaths without a single well
// defined direction (e.g. a lone MoveTo) produce no vertices at all, and at
// a 180° hairpin turn, where the bisector is indeterminate, the incoming
// direction is used.
func ComputeMarkerVertices(path *draw2d.Path) MarkerVertices {
	var c subpathCollector
	Flatten(path, &c, 1)
	var result MarkerVertices
	for _, sp := range c.subpaths {
		pts := collapsePoints(sp.pts)
		if sp.closed && len(pts) > 1 {
			// the closing point duplicates the first one
			pts = pts[:len(pts)-1]
		}
		if len(pts) < 2 {
			continue
		}
		n := len(pts)
		if !sp.closed {
			result.Start = MarkerVertex{pts[0][0], pts[0][1], segmentDir(pts[0], pts[1])}
			result.HasStart = true
			result.End = MarkerVertex{pts[n-1][0], pts[n-1][1], segmentDir(pts[n-2], pts[n-1])}
			result.HasEnd = true
			for i := 1; i < n-1; i++ {
				result.Mids = append(result.Mids, bisectorVertex(pts[i-1], pts[i], pts[i+1]))
			}
			continue
		}
		// closed subpath: start and end markers coincide at pts[0]
		result.Start = MarkerVertex{pts[0][0], pts[0][1], segmentDir(pts[0], pts[1])}
		result.HasStart = true
		result.End = MarkerVertex{pts[0][0], pts[0][1], segmentDir(pts[n-1], pts[0])}
		result.HasEnd = true
		for i := 1; i < n; i++ {
			result.Mids = append(result.Mids, bisectorVertex(pts[i-1], pts[i], pts[(i+1)%n]))
		}
	}
	return result
}

// bisectorVertex returns the mid marker vertex at b, oriented along the
// bisector of the segments a->b and b->c. At a 180° hairpin turn the
// bisector is indeterminate and the incoming direction is used.
func bisectorVertex(a, b, c [2]float64) MarkerVertex {
	in := segmentDir(a, b)
	out := segmentDir(b, c)
	sinIn, cosIn := math.Sincos(in)
	sinOut, cosOut := math.Sincos(out)
	sx, sy := cosIn+cosOut, sinIn+sinOut
	angle := in
	if sx*sx+sy*sy >= tangentEpsilon {
		angle = math.Atan2(sy, sx)
	}
	return MarkerVertex{b[0], b[1], angle}
}

// segmentDir returns the angle of the segment from a to b. Callers must
// ensure a != b (consecutive duplicates are collapsed beforehand).
func segmentDir(a, b [2]float64) float64 {
	return math.Atan2(b[1]-a[1], b[0]-a[0])
}

// collapsePoints removes consecutive duplicate points.
func collapsePoints(pts [][2]float64) [][2]float64 {
	out := pts[:0]
	for _, p := range pts {
		if len(out) == 0 || out[len(out)-1] != p {
			out = append(out, p)
		}
	}
	return out
}

// subpathCollector flattens a path into polygonal subpaths.
type subpathCollector struct {
	subpaths []*subpath
	cur      *subpath
}

type subpath struct {
	pts    [][2]float64
	closed bool
}

func (c *subpathCollector) flush() {
	if c.cur != nil && len(c.cur.pts) > 0 {
		c.subpaths = append(c.subpaths, c.cur)
	}
	c.cur = nil
}

func (c *subpathCollector) MoveTo(x, y float64) {
	c.flush()
	c.cur = &subpath{pts: [][2]float64{{x, y}}}
}

func (c *subpathCollector) LineTo(x, y float64) {
	if c.cur == nil {
		c.cur = &subpath{}
	}
	c.cur.pts = append(c.cur.pts, [2]float64{x, y})
}

func (c *subpathCollector) LineJoin() {}

func (c *subpathCollector) Close() {
	if c.cur != nil {
		c.cur.closed = true
	}
}

func (c *subpathCollector) End() {
	c.flush()
}

// MarkerTransform returns the matrix that maps marker artwork coordinates
// into the user space of the referencing path: the artwork is scaled so the
// viewBox fits the Width x Height viewport (scaled by strokeWidth when the
// marker uses strokeWidth units), shifted so the reference point sits on the
// vertex, and rotated to the vertex tangent when the marker orients
// automatically.
func MarkerTransform(m *draw2d.Marker, v MarkerVertex, strokeWidth float64) draw2d.Matrix {
	swf := 1.0
	if m.ScaleWithStroke {
		swf = strokeWidth
	}
	scale := swf
	if m.ViewBox != ([4]float64{}) {
		scale = math.Min(m.Width*swf/m.ViewBox[2], m.Height*swf/m.ViewBox[3])
	}
	angle := m.OrientAngle * math.Pi / 180
	if m.OrientAuto {
		angle = v.Angle
	}
	tr := draw2d.NewTranslationMatrix(v.X, v.Y)
	tr.Rotate(angle)
	tr.Scale(scale, scale)
	tr.Translate(-m.RefX, -m.RefY)
	return tr
}

// transformMarkerPath flattens the marker artwork (resolving arcs and curves)
// and returns it transformed by tr.
func transformMarkerPath(m *draw2d.Marker, tr draw2d.Matrix) *draw2d.Path {
	builder := &pathBuilder{}
	Flatten(m.Path, Transformer{Tr: tr, Flattener: builder}, 1)
	return builder.path
}

// pathBuilder rebuilds a draw2d.Path from flattened segments.
type pathBuilder struct {
	path *draw2d.Path
}

func (b *pathBuilder) ensure() {
	if b.path == nil {
		b.path = new(draw2d.Path)
	}
}

func (b *pathBuilder) MoveTo(x, y float64) { b.ensure(); b.path.MoveTo(x, y) }
func (b *pathBuilder) LineTo(x, y float64) { b.ensure(); b.path.LineTo(x, y) }
func (b *pathBuilder) LineJoin()           {}
func (b *pathBuilder) Close()              { b.ensure(); b.path.Close() }
func (b *pathBuilder) End()                {}

// DrawMarkers rasterizes the given markers at the vertices of paths onto gc.
// It is the fallback for backends without native marker support. defaultFill
// is used for markers that do not set their own fill (usually the stroke
// color of the referencing path). The current transformation matrix of gc
// applies to the markers, exactly as it does to the paths themselves.
//
// It returns ErrMarkerCircular if a marker's path is among the decorated
// paths, and the validation error of the first invalid marker. When no
// markers are set it does nothing and returns nil.
func DrawMarkers(gc draw2d.GraphicContext, markers map[draw2d.MarkerPosition]*draw2d.Marker, defaultFill color.Color, strokeWidth float64, paths ...*draw2d.Path) error {
	if len(markers) == 0 {
		return nil
	}
	for _, m := range markers {
		if m == nil {
			continue
		}
		if err := m.Validate(); err != nil {
			return err
		}
		for _, p := range paths {
			if p == m.Path {
				return draw2d.ErrMarkerCircular
			}
		}
	}
	gc.Save()
	defer gc.Restore()
	// prevent recursion when a backend draws markers through its own
	// Stroke/FillStroke implementation
	if cm, ok := gc.(interface{ ClearMarkers() }); ok {
		cm.ClearMarkers()
	}
	for _, path := range paths {
		if path == nil || path.IsEmpty() {
			continue
		}
		verts := ComputeMarkerVertices(path)
		for pos, m := range markers {
			if m == nil {
				continue
			}
			var vs []MarkerVertex
			switch pos {
			case draw2d.MarkerStart:
				if verts.HasStart {
					vs = []MarkerVertex{verts.Start}
				}
			case draw2d.MarkerMid:
				vs = verts.Mids
			case draw2d.MarkerEnd:
				if verts.HasEnd {
					vs = []MarkerVertex{verts.End}
				}
			}
			for _, v := range vs {
				mp := transformMarkerPath(m, MarkerTransform(m, v, strokeWidth))
				fill := m.Fill
				if fill == nil {
					fill = defaultFill
				}
				if fill != nil {
					gc.SetFillColor(fill)
					gc.Fill(mp)
				}
				if m.Stroke != nil {
					w := m.StrokeWidth
					if w <= 0 {
						w = 1
					}
					gc.SetLineWidth(w)
					gc.SetStrokeColor(m.Stroke)
					gc.Stroke(mp)
				}
			}
		}
	}
	return nil
}
