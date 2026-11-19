// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dsvg

import (
	"encoding/xml"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/llgcode/draw2d"
)

func newGradientContext() (*Svg, *GraphicContext) {
	svg := NewSvg()
	return svg, NewGraphicContext(svg)
}

func fillTriangle(gc *GraphicContext) {
	gc.MoveTo(0, 0)
	gc.LineTo(100, 0)
	gc.LineTo(100, 100)
	gc.Close()
	gc.Fill()
}

func TestSvgLinearGradientDefinition(t *testing.T) {
	svg, gc := newGradientContext()
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(10, 20, 30, 40, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 0, G: 0, B: 255, A: 255}},
		{Offset: 0.5, Color: color.NRGBA{R: 128, G: 0, B: 128, A: 128}},
		{Offset: 1, Color: color.NRGBA{R: 255, G: 128, B: 0, A: 255}},
	}))
	fillTriangle(gc)

	if len(svg.LinearGradients) != 1 {
		t.Fatalf("got %d gradient defs, want 1", len(svg.LinearGradients))
	}
	g := svg.LinearGradients[0]
	if g.Id != "gradient-1" {
		t.Errorf("id = %q, want gradient-1", g.Id)
	}
	if g.X1 != "10" || g.Y1 != "20" || g.X2 != "30" || g.Y2 != "40" {
		t.Errorf("axis = (%s,%s)-(%s,%s)", g.X1, g.Y1, g.X2, g.Y2)
	}
	if g.GradientUnits != "userSpaceOnUse" {
		t.Errorf("gradientUnits = %q", g.GradientUnits)
	}
	if g.SpreadMethod != "" {
		t.Errorf("pad should omit spreadMethod, got %q", g.SpreadMethod)
	}
	if len(g.Stops) != 3 {
		t.Fatalf("got %d stops, want 3", len(g.Stops))
	}
	if g.Stops[0].Offset != "0" || g.Stops[0].StopColor != "#0000FF" || g.Stops[0].StopOpacity != "" {
		t.Errorf("stop 0 = %+v", g.Stops[0])
	}
	if g.Stops[1].StopColor != "#800080" {
		t.Errorf("semi transparent stop color = %q, want straight #800080", g.Stops[1].StopColor)
	}
	if g.Stops[1].StopOpacity != "0.501961" {
		t.Errorf("semi transparent stop opacity = %q", g.Stops[1].StopOpacity)
	}
	if len(g.Stops[2].StopColor) != 7 || g.Stops[2].StopColor != "#FF8000" {
		t.Errorf("stop 2 color = %q", g.Stops[2].StopColor)
	}
}

func TestSvgLinearGradientFillReferenceAndXML(t *testing.T) {
	svg, gc := newGradientContext()
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 100, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 0, G: 0, B: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{R: 255, G: 128, B: 0, A: 255}},
	}))
	fillTriangle(gc)

	if len(svg.Groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(svg.Groups))
	}
	if got := svg.Groups[0].Fill; got != "url(#gradient-1)" {
		t.Errorf("group fill = %q, want gradient reference", got)
	}
	out, err := xml.MarshalIndent(svg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<linearGradient id="gradient-1" x1="0" y1="0" x2="100" y2="0" gradientUnits="userSpaceOnUse">`,
		`<stop offset="0" stop-color="#0000FF"></stop>`,
		`<stop offset="1" stop-color="#FF8000"></stop>`,
		`<g fill="url(#gradient-1)" fill-rule="evenodd">`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("SVG output missing %q\n%s", want, out)
		}
	}
}

func TestSvgLinearGradientSpreadMethod(t *testing.T) {
	for mode, want := range map[draw2d.ExtendMode]string{
		draw2d.ExtendRepeat:  `spreadMethod="repeat"`,
		draw2d.ExtendReflect: `spreadMethod="reflect"`,
		draw2d.ExtendPad:     `spreadMethod=""`,
	} {
		svg, gc := newGradientContext()
		g := draw2d.NewLinearGradient(0, 0, 10, 0, []draw2d.GradientStop{{Offset: 0, Color: color.White}, {Offset: 1, Color: color.Black}})
		g.Extend = mode
		gc.SetFillLinearGradient(g)
		fillTriangle(gc)
		out, err := xml.MarshalIndent(svg, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		has := strings.Contains(string(out), want)
		if mode == draw2d.ExtendPad {
			has = !strings.Contains(string(out), "spreadMethod")
		}
		if !has {
			t.Errorf("mode %v: output missing %q\n%s", mode, want, out)
		}
	}
}

func TestSvgLinearGradientSortedStops(t *testing.T) {
	// Out of order stops must be emitted sorted by offset, so an SVG renderer
	// and the image backend agree.
	svg, gc := newGradientContext()
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 10, 0, []draw2d.GradientStop{
		{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
	}))
	fillTriangle(gc)
	stops := svg.LinearGradients[0].Stops
	if stops[0].StopColor != "#FF0000" || stops[1].StopColor != "#0000FF" {
		t.Errorf("stops not sorted: %q then %q", stops[0].StopColor, stops[1].StopColor)
	}
}

func TestSvgLinearGradientTransform(t *testing.T) {
	// The gradient definition stays in user space and the group carries the
	// transform, exactly like an SVG userSpaceOnUse gradient on a rotated
	// shape.
	svg, gc := newGradientContext()
	gc.Translate(10, 20)
	gc.Scale(2, 3)
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 100, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.White},
		{Offset: 1, Color: color.Black},
	}))
	fillTriangle(gc)
	g := svg.LinearGradients[0]
	if g.X2 != "100" {
		t.Errorf("gradient axis should be in user space, x2 = %q", g.X2)
	}
	if got := svg.Groups[0].Transform; !strings.HasPrefix(got, "matrix(") {
		t.Errorf("group transform = %q, want matrix(...)", got)
	}
}

func TestSvgLinearGradientFallbackToSolid(t *testing.T) {
	solidFill := "#112233"
	cases := map[string]*draw2d.LinearGradient{
		"no stops":        draw2d.NewLinearGradient(0, 0, 10, 0, nil),
		"non-finite axis": {X0: math.NaN(), X1: 10, Stops: []draw2d.GradientStop{{Offset: 0, Color: color.White}}},
	}
	for name, g := range cases {
		t.Run(name, func(t *testing.T) {
			svg, gc := newGradientContext()
			gc.SetFillColor(color.NRGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xFF})
			gc.SetFillLinearGradient(g)
			if got := svg.Groups; len(got) != 0 {
				t.Fatal("draw happened before fill")
			}
			fillTriangle(gc)
			if len(svg.LinearGradients) != 0 {
				t.Errorf("unsupported gradient should not emit a def, got %d", len(svg.LinearGradients))
			}
			if got := svg.Groups[0].Fill; got != solidFill {
				t.Errorf("fill = %q, want solid %q", got, solidFill)
			}
		})
	}
}

func TestSvgSetFillColorClearsGradient(t *testing.T) {
	svg, gc := newGradientContext()
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 10, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.White},
		{Offset: 1, Color: color.Black},
	}))
	gc.SetFillColor(color.White)
	fillTriangle(gc)
	if len(svg.LinearGradients) != 0 {
		t.Error("SetFillColor should clear the gradient")
	}
	if got := svg.Groups[0].Fill; got != "#FFFFFF" {
		t.Errorf("fill = %q, want solid white", got)
	}
}

func TestSvgLinearGradientPerFillDefinition(t *testing.T) {
	// Two fills with different gradients must reference two distinct defs.
	svg, gc := newGradientContext()
	g1 := draw2d.NewLinearGradient(0, 0, 10, 0, []draw2d.GradientStop{{Offset: 0, Color: color.White}, {Offset: 1, Color: color.Black}})
	g2 := draw2d.NewLinearGradient(0, 0, 20, 0, []draw2d.GradientStop{{Offset: 0, Color: color.Black}, {Offset: 1, Color: color.White}})
	gc.SetFillLinearGradient(g1)
	fillTriangle(gc)
	gc.SetFillLinearGradient(g2)
	fillTriangle(gc)
	if len(svg.LinearGradients) != 2 {
		t.Fatalf("got %d defs, want 2", len(svg.LinearGradients))
	}
	if svg.LinearGradients[0].Id == svg.LinearGradients[1].Id {
		t.Error("gradient ids must be unique")
	}
	if svg.Groups[0].Fill == svg.Groups[1].Fill {
		t.Error("each fill must reference its own gradient")
	}
}

func TestSvgLinearGradientXMLRoundTrip(t *testing.T) {
	svg, gc := newGradientContext()
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 100, 100, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 0, G: 0, B: 128, A: 255}},
		{Offset: 1, Color: color.NRGBA{R: 255, G: 140, B: 0, A: 255}},
	}))
	fillTriangle(gc)
	out, err := xml.Marshal(svg)
	if err != nil {
		t.Fatal(err)
	}
	var parsed Svg
	if err := xml.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	if len(parsed.LinearGradients) != 1 || len(parsed.LinearGradients[0].Stops) != 2 {
		t.Fatalf("round trip lost gradient data: %+v", parsed.LinearGradients)
	}
	if parsed.Groups[0].Fill != "url(#gradient-1)" {
		t.Errorf("round trip fill = %q", parsed.Groups[0].Fill)
	}
}

func TestSvgLinearGradientClampedOffsetsAndNaN(t *testing.T) {
	// Offsets outside [0, 1] are clamped and non-finite offsets dropped, so
	// the emitted SVG stays valid.
	svg, gc := newGradientContext()
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 10, 0, []draw2d.GradientStop{
		{Offset: 2, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: math.NaN(), Color: color.NRGBA{G: 255, A: 255}},
		{Offset: -1, Color: color.NRGBA{B: 255, A: 255}},
	}))
	fillTriangle(gc)
	stops := svg.LinearGradients[0].Stops
	if len(stops) != 2 {
		t.Fatalf("got %d stops, want 2", len(stops))
	}
	if stops[0].Offset != "0" || stops[0].StopColor != "#0000FF" {
		t.Errorf("clamped low stop = %+v", stops[0])
	}
	if stops[1].Offset != "1" || stops[1].StopColor != "#FF0000" {
		t.Errorf("clamped high stop = %+v", stops[1])
	}
}

func TestSvgLinearGradientFullyTransparentStop(t *testing.T) {
	svg, gc := newGradientContext()
	gc.SetFillLinearGradient(draw2d.NewLinearGradient(0, 0, 10, 0, []draw2d.GradientStop{
		{Offset: 0, Color: color.NRGBA{R: 255, A: 255}},
		{Offset: 1, Color: color.NRGBA{}},
	}))
	fillTriangle(gc)
	stop := svg.LinearGradients[0].Stops[1]
	if stop.StopOpacity != "0" {
		t.Errorf("fully transparent stop-opacity = %q, want 0", stop.StopOpacity)
	}
	if stop.StopColor != "#000000" {
		t.Errorf("transparent stop color = %q, want #000000", stop.StopColor)
	}
}
