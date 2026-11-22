// XML fixtures for radial gradient fills on the svg backend.

package draw2dsvg

import (
	"encoding/xml"
	"image/color"
	"testing"

	"github.com/llgcode/draw2d"
)

func fillRectPath(gc *GraphicContext, x0, y0, x1, y1 float64) {
	gc.BeginPath()
	gc.MoveTo(x0, y0)
	gc.LineTo(x1, y0)
	gc.LineTo(x1, y1)
	gc.LineTo(x0, y1)
	gc.Close()
	gc.Fill()
}

func marshal(t *testing.T, svg *Svg) string {
	t.Helper()
	out, err := xml.MarshalIndent(svg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestXml_RadialGradient(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)

	g := draw2d.NewRadialGradient(50, 50, 40, 70, 50)
	g.Spread = draw2d.ReflectSpread
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(1, color.NRGBA{0, 0, 255, 128})
	gc.SetFillGradient(g)
	fillRectPath(gc, 0, 0, 100, 100)

	expectedOut := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs>
    <radialGradient id="radialGradient-1" gradientUnits="userSpaceOnUse" cx="50" cy="50" r="40" fx="70" fy="50" spreadMethod="reflect">
      <stop offset="0" stop-color="#FF0000"></stop>
      <stop offset="1" stop-color="#0000FF" stop-opacity="0.502"></stop>
    </radialGradient>
  </defs>
  <g fill="url(#radialGradient-1)" fill-rule="evenodd">
    <path d="M 0,0 L 100,0 L 100,100 L 0,100 Z"></path>
  </g>
</svg>`

	if out := marshal(t, svg); out != expectedOut {
		t.Errorf("svg output is not as expected\ngot:\n%s\n\nwant:\n%s\n", out, expectedOut)
	}
}

func TestXml_RadialGradientPadConcentric(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)

	// concentric focus and pad spread are the svg defaults:
	// no spreadMethod attribute, fx/fy equal to cx/cy
	g := draw2d.NewRadialGradient(50, 50, 40, 50, 50)
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(1, color.RGBA{0, 0, 255, 255})
	gc.SetFillGradient(g)
	fillRectPath(gc, 0, 0, 100, 100)

	expectedOut := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs>
    <radialGradient id="radialGradient-1" gradientUnits="userSpaceOnUse" cx="50" cy="50" r="40" fx="50" fy="50">
      <stop offset="0" stop-color="#FF0000"></stop>
      <stop offset="1" stop-color="#0000FF"></stop>
    </radialGradient>
  </defs>
  <g fill="url(#radialGradient-1)" fill-rule="evenodd">
    <path d="M 0,0 L 100,0 L 100,100 L 0,100 Z"></path>
  </g>
</svg>`

	if out := marshal(t, svg); out != expectedOut {
		t.Errorf("svg output is not as expected\ngot:\n%s\n\nwant:\n%s\n", out, expectedOut)
	}
}

func TestXml_RadialGradientFocusClamped(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)

	// focus outside the circle is clamped onto it, as in the image backend
	g := draw2d.NewRadialGradient(50, 50, 40, 200, 50)
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(1, color.RGBA{0, 0, 255, 255})
	gc.SetFillGradient(g)
	fillRectPath(gc, 0, 0, 100, 100)

	expectedOut := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs>
    <radialGradient id="radialGradient-1" gradientUnits="userSpaceOnUse" cx="50" cy="50" r="40" fx="90" fy="50">
      <stop offset="0" stop-color="#FF0000"></stop>
      <stop offset="1" stop-color="#0000FF"></stop>
    </radialGradient>
  </defs>
  <g fill="url(#radialGradient-1)" fill-rule="evenodd">
    <path d="M 0,0 L 100,0 L 100,100 L 0,100 Z"></path>
  </g>
</svg>`

	if out := marshal(t, svg); out != expectedOut {
		t.Errorf("svg output is not as expected\ngot:\n%s\n\nwant:\n%s\n", out, expectedOut)
	}
}

func TestXml_RadialGradientStopsNormalized(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)

	// stops added out of order and out of range are sorted and clamped
	g := draw2d.NewRadialGradient(50, 50, 40, 50, 50)
	g.AddColorStop(1.5, color.RGBA{0, 0, 255, 255})
	g.AddColorStop(-0.5, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(0.5, color.RGBA{0, 255, 0, 255})
	gc.SetFillGradient(g)
	fillRectPath(gc, 0, 0, 100, 100)

	expectedOut := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs>
    <radialGradient id="radialGradient-1" gradientUnits="userSpaceOnUse" cx="50" cy="50" r="40" fx="50" fy="50">
      <stop offset="0" stop-color="#FF0000"></stop>
      <stop offset="0.5" stop-color="#00FF00"></stop>
      <stop offset="1" stop-color="#0000FF"></stop>
    </radialGradient>
  </defs>
  <g fill="url(#radialGradient-1)" fill-rule="evenodd">
    <path d="M 0,0 L 100,0 L 100,100 L 0,100 Z"></path>
  </g>
</svg>`

	if out := marshal(t, svg); out != expectedOut {
		t.Errorf("svg output is not as expected\ngot:\n%s\n\nwant:\n%s\n", out, expectedOut)
	}
}

func TestXml_RadialGradientSaveRestore(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)

	g := draw2d.NewRadialGradient(50, 50, 40, 50, 50)
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(1, color.RGBA{0, 0, 255, 255})

	gc.SetFillGradient(g)
	gc.Save()
	gc.SetFillColor(color.RGBA{0, 255, 0, 255}) // solid replaces gradient
	fillRectPath(gc, 0, 0, 10, 10)
	gc.Save()
	gc.SetFillGradient(g)
	fillRectPath(gc, 20, 0, 30, 10)
	gc.Restore()
	gc.Restore()
	fillRectPath(gc, 40, 0, 50, 10) // gradient is back

	expectedOut := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs>
    <radialGradient id="radialGradient-1" gradientUnits="userSpaceOnUse" cx="50" cy="50" r="40" fx="50" fy="50">
      <stop offset="0" stop-color="#FF0000"></stop>
      <stop offset="1" stop-color="#0000FF"></stop>
    </radialGradient>
    <radialGradient id="radialGradient-2" gradientUnits="userSpaceOnUse" cx="50" cy="50" r="40" fx="50" fy="50">
      <stop offset="0" stop-color="#FF0000"></stop>
      <stop offset="1" stop-color="#0000FF"></stop>
    </radialGradient>
  </defs>
  <g fill="#00FF00" fill-rule="evenodd">
    <path d="M 0,0 L 10,0 L 10,10 L 0,10 Z"></path>
  </g>
  <g fill="url(#radialGradient-1)" fill-rule="evenodd">
    <path d="M 20,0 L 30,0 L 30,10 L 20,10 Z"></path>
  </g>
  <g fill="url(#radialGradient-2)" fill-rule="evenodd">
    <path d="M 40,0 L 50,0 L 50,10 L 40,10 Z"></path>
  </g>
</svg>`

	if out := marshal(t, svg); out != expectedOut {
		t.Errorf("svg output is not as expected\ngot:\n%s\n\nwant:\n%s\n", out, expectedOut)
	}
}

func TestXml_RadialGradientInvalid(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)

	// a gradient without stops is invalid and paints nothing
	gc.SetFillGradient(draw2d.NewRadialGradient(50, 50, 40, 50, 50))
	fillRectPath(gc, 0, 0, 10, 10)

	expectedOut := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs></defs>
  <g fill="none" fill-rule="evenodd">
    <path d="M 0,0 L 10,0 L 10,10 L 0,10 Z"></path>
  </g>
</svg>`

	if out := marshal(t, svg); out != expectedOut {
		t.Errorf("svg output is not as expected\ngot:\n%s\n\nwant:\n%s\n", out, expectedOut)
	}
}

func TestXml_RadialGradientZeroRadius(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)

	// r="0" is valid svg: the shape is painted with the last stop color,
	// which is also what the image backend produces
	g := draw2d.NewRadialGradient(50, 50, 0, 50, 50)
	g.AddColorStop(0, color.RGBA{255, 0, 0, 255})
	g.AddColorStop(1, color.RGBA{0, 0, 255, 255})
	gc.SetFillGradient(g)
	fillRectPath(gc, 0, 0, 10, 10)

	expectedOut := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs>
    <radialGradient id="radialGradient-1" gradientUnits="userSpaceOnUse" cx="50" cy="50" r="0" fx="50" fy="50">
      <stop offset="0" stop-color="#FF0000"></stop>
      <stop offset="1" stop-color="#0000FF"></stop>
    </radialGradient>
  </defs>
  <g fill="url(#radialGradient-1)" fill-rule="evenodd">
    <path d="M 0,0 L 10,0 L 10,10 L 0,10 Z"></path>
  </g>
</svg>`

	if out := marshal(t, svg); out != expectedOut {
		t.Errorf("svg output is not as expected\ngot:\n%s\n\nwant:\n%s\n", out, expectedOut)
	}
}
