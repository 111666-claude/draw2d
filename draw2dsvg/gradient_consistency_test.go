// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dsvg

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dimg"
)

// modelSvgGradient samples an emitted SVG linearGradient the way an SVG
// renderer would: map the device point to user space with the group transform,
// project it on the gradient axis and interpolate the stops with the declared
// spread method. It is an independent implementation used to cross check the
// image backend.
func modelSvgGradient(t *testing.T, g *LinearGradient, groupTransform string, deviceX, deviceY float64) color.NRGBA {
	t.Helper()
	x0 := mustParseSvgFloat(t, g.X1)
	y0 := mustParseSvgFloat(t, g.Y1)
	x1 := mustParseSvgFloat(t, g.X2)
	y1 := mustParseSvgFloat(t, g.Y2)
	tr := parseSimpleTransform(t, groupTransform)
	ux, uy := tr.InverseTransformPoint(deviceX, deviceY)
	tp := svgGradientParam(x0, y0, x1, y1, ux, uy)
	switch g.SpreadMethod {
	case "repeat":
		tp -= math.Floor(tp)
	case "reflect":
		tp -= 2 * math.Floor(tp/2)
		if tp > 1 {
			tp = 2 - tp
		}
	}
	type stop struct {
		off float64
		c   color.NRGBA
	}
	stops := make([]stop, 0, len(g.Stops))
	for _, s := range g.Stops {
		c := parseHex(t, s.StopColor)
		if s.StopOpacity != "" {
			c.A = uint8(math.Round(mustParseSvgFloat(t, s.StopOpacity) * 255))
		}
		stops = append(stops, stop{mustParseSvgFloat(t, s.Offset), c})
	}
	if tp <= stops[0].off {
		return stops[0].c
	}
	if tp >= stops[len(stops)-1].off {
		return stops[len(stops)-1].c
	}
	for i := 1; i < len(stops); i++ {
		if tp <= stops[i].off {
			a, b := stops[i-1].c, stops[i].c
			u := (tp - stops[i-1].off) / (stops[i].off - stops[i-1].off)
			aa := float64(a.A) / 255
			ba := float64(b.A) / 255
			pa := aa + (ba-aa)*u
			if pa == 0 {
				return color.NRGBA{}
			}
			mix := func(ca, cb uint8) uint8 {
				pa0 := float64(ca) * aa
				pb0 := float64(cb) * ba
				return uint8(math.Round((pa0 + (pb0-pa0)*u) / pa))
			}
			return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: uint8(math.Round(pa * 255))}
		}
	}
	return color.NRGBA{}
}

func svgGradientParam(x0, y0, x1, y1, x, y float64) float64 {
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Inf(1)
	}
	return ((x-x0)*dx + (y-y0)*dy) / l2
}

func mustParseSvgFloat(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("cannot parse %q: %v", s, err)
	}
	return v
}

func parseHex(t *testing.T, s string) color.NRGBA {
	t.Helper()
	if len(s) != 7 || s[0] != '#' {
		t.Fatalf("bad hex color %q", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		t.Fatalf("bad hex color %q: %v", s, err)
	}
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

// parseSimpleTransform understands the two shapes emitted by toSvgTransform:
// "translate(x,y)" and "matrix(a,b,c,d,e,f)".
func parseSimpleTransform(t *testing.T, s string) draw2d.Matrix {
	t.Helper()
	if s == "" {
		return draw2d.NewIdentityMatrix()
	}
	if len(s) > len("translate()") && s[:10] == "translate(" {
		parts := splitFloats(t, s[10:len(s)-1])
		if len(parts) != 2 {
			t.Fatalf("bad translate %q", s)
		}
		return draw2d.NewTranslationMatrix(parts[0], parts[1])
	}
	if len(s) > 7 && s[:7] == "matrix(" {
		parts := splitFloats(t, s[7:len(s)-1])
		if len(parts) != 6 {
			t.Fatalf("bad matrix %q", s)
		}
		return draw2d.Matrix{parts[0], parts[1], parts[2], parts[3], parts[4], parts[5]}
	}
	t.Fatalf("unsupported transform %q", s)
	return draw2d.NewIdentityMatrix()
}

func splitFloats(t *testing.T, s string) []float64 {
	t.Helper()
	var out []float64
	for _, part := range splitComma(s) {
		if part == "" {
			continue
		}
		out = append(out, mustParseSvgFloat(t, part))
	}
	return out
}

func splitComma(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func TestSvgAndImageGradientAgree(t *testing.T) {
	stops := []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 0, G: 0, B: 255, A: 255}},
		{Offset: 0.5, Color: color.NRGBA{R: 255, G: 0, B: 0, A: 128}},
		{Offset: 1, Color: color.NRGBA{R: 255, G: 128, B: 0, A: 255}},
	}
	for _, mode := range []draw2d.ExtendMode{draw2d.ExtendPad, draw2d.ExtendRepeat, draw2d.ExtendReflect} {
		t.Run(mode.String(), func(t *testing.T) {
			// Same user space geometry, rotation and scale on both backends.
			tr := draw2d.NewTranslationMatrix(40, 30)
			tr.Compose(draw2d.NewRotationMatrix(math.Pi / 5))
			tr.Compose(draw2d.NewScaleMatrix(1.7, 0.8))
			tr.Compose(draw2d.NewTranslationMatrix(-20, -10))

			grad := draw2d.NewLinearGradient(-10, 0, 60, 25, stops)
			grad.Extend = mode

			img := image.NewRGBA(image.Rect(0, 0, 80, 60))
			igc := draw2dimg.NewGraphicContext(img)
			igc.SetFillColor(color.NRGBA{R: 9, G: 9, B: 9, A: 255})
			igc.Clear()
			igc.SetFillLinearGradient(grad)
			igc.SetMatrixTransform(tr)
			igc.MoveTo(-30, -30)
			igc.LineTo(90, -30)
			igc.LineTo(90, 90)
			igc.LineTo(-30, 90)
			igc.Close()
			igc.Fill()

			svg := NewSvg()
			sgc := NewGraphicContext(svg)
			sgc.SetFillLinearGradient(grad)
			sgc.SetMatrixTransform(tr)
			sgc.MoveTo(-30, -30)
			sgc.LineTo(90, -30)
			sgc.LineTo(90, 90)
			sgc.LineTo(-30, 90)
			sgc.Close()
			sgc.Fill()

			if len(svg.LinearGradients) != 1 || len(svg.Groups) != 1 {
				t.Fatalf("unexpected svg structure: %d defs, %d groups", len(svg.LinearGradients), len(svg.Groups))
			}
			def := svg.LinearGradients[0]
			groupTr := svg.Groups[0].Transform

			samples := [][2]int{{5, 5}, {20, 30}, {40, 30}, {55, 10}, {70, 50}, {39, 31}}
			for _, p := range samples {
				want := modelSvgGradient(t, def, groupTr, float64(p[0])+0.5, float64(p[1])+0.5)
				// The image fill is composited over the opaque background; do
				// the same to the SVG model before comparing.
				want = compositeOver(want, color.NRGBA{R: 9, G: 9, B: 9, A: 255})
				got := toStraight(img.RGBAAt(p[0], p[1]))
				if !nearColor(got, want, 3) {
					t.Errorf("pixel (%d,%d): image %v, svg model %v", p[0], p[1], got, want)
				}
			}
		})
	}
}

func toStraight(c color.RGBA) color.NRGBA {
	return color.NRGBAModel.Convert(c).(color.NRGBA)
}

// compositeOver composites src over dst with straight alpha and returns the
// opaque result. Both colors must have alpha 255 in dst.
func compositeOver(src, dst color.NRGBA) color.NRGBA {
	if src.A == 255 {
		return src
	}
	sa := float64(src.A) / 255
	blend := func(s, d uint8) uint8 {
		return uint8(math.Round(float64(s)*sa + float64(d)*(1-sa)))
	}
	return color.NRGBA{
		R: blend(src.R, dst.R),
		G: blend(src.G, dst.G),
		B: blend(src.B, dst.B),
		A: 255,
	}
}

func nearColor(a, b color.NRGBA, tol int) bool {
	diff := func(x, y uint8) bool {
		d := int(x) - int(y)
		return d <= tol && d >= -tol
	}
	return diff(a.R, b.R) && diff(a.G, b.G) && diff(a.B, b.B) && diff(a.A, b.A)
}
