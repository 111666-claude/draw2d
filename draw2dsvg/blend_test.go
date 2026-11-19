// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dsvg

import (
	"encoding/xml"
	"errors"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dkit"
)

// marshal renders svg to indented XML for assertions.
func marshal(t *testing.T, svg *Svg) string {
	t.Helper()
	out, err := xml.MarshalIndent(svg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(out)
}

func TestBlendNormalEmitsNoStyle(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()

	got := marshal(t, svg)
	if strings.Contains(got, "mix-blend-mode") || strings.Contains(got, "style=") {
		t.Errorf("normal mode emitted a style:\n%s", got)
	}
	want := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs></defs>
  <g fill="#FFFFFF" fill-rule="evenodd">
    <path d="M 0,0 L 10,0 L 10,10 L 0,10 Z"></path>
  </g>
</svg>`
	if got != want {
		t.Errorf("normal output changed\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBlendModePerDrawing(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()
	if err := gc.SetBlendMode(draw2d.BlendMultiply); err != nil {
		t.Fatal(err)
	}
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()
	if err := gc.SetBlendMode(draw2d.BlendScreen); err != nil {
		t.Fatal(err)
	}
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Stroke()

	got := marshal(t, svg)
	want := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs></defs>
  <g fill="#FFFFFF" fill-rule="evenodd">
    <path d="M 0,0 L 10,0 L 10,10 L 0,10 Z"></path>
  </g>
  <g fill="#FFFFFF" fill-rule="evenodd" style="mix-blend-mode:multiply">
    <path d="M 0,0 L 10,0 L 10,10 L 0,10 Z"></path>
  </g>
  <g stroke="#000000" stroke-width="1" stroke-linecap="round" stroke-linejoin="round" style="mix-blend-mode:screen">
    <path d="M 0,0 L 10,0 L 10,10 L 0,10 Z"></path>
  </g>
</svg>`
	if got != want {
		t.Errorf("svg output is not as expected\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBlendModeRestore(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.Save()
	if err := gc.SetBlendMode(draw2d.BlendMultiply); err != nil {
		t.Fatal(err)
	}
	gc.Restore()
	if gc.GetBlendMode() != draw2d.BlendNormal {
		t.Errorf("Restore did not restore the blend mode: %v", gc.GetBlendMode())
	}
}

func TestBlendModeInvalid(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	if err := gc.SetBlendMode(draw2d.BlendMode(-3)); !errors.Is(err, draw2d.ErrInvalidBlendMode) {
		t.Errorf("SetBlendMode(invalid) = %v, want ErrInvalidBlendMode", err)
	}
	if got := gc.GetBlendMode(); got != draw2d.BlendNormal {
		t.Errorf("invalid mode changed state to %v", got)
	}
	var _ draw2d.GraphicContext = gc
}

func TestLayerEmitsBlendGroup(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill() // outside the layer, no style

	if err := gc.BeginLayer(draw2d.BlendMultiply, 0.5); err != nil {
		t.Fatal(err)
	}
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()
	draw2dkit.Rectangle(gc, 10, 10, 20, 20)
	gc.Stroke()
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}

	got := marshal(t, svg)
	want := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs></defs>
  <g fill="#FFFFFF" fill-rule="evenodd">
    <path d="M 0,0 L 10,0 L 10,10 L 0,10 Z"></path>
  </g>
  <g style="mix-blend-mode:multiply;opacity:0.5">
    <g fill="#FFFFFF" fill-rule="evenodd">
      <path d="M 0,0 L 10,0 L 10,10 L 0,10 Z"></path>
    </g>
    <g stroke="#000000" stroke-width="1" stroke-linecap="round" stroke-linejoin="round">
      <path d="M 10,10 L 20,10 L 20,20 L 10,20 Z"></path>
    </g>
  </g>
</svg>`
	if got != want {
		t.Errorf("svg layer output is not as expected\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLayerNormalOpaqueHasNoStyle(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
		t.Fatal(err)
	}
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	got := marshal(t, svg)
	if strings.Contains(got, "style=") {
		t.Errorf("normal opaque layer emitted a style:\n%s", got)
	}
	if !strings.Contains(got, "<g>\n") {
		t.Errorf("normal opaque layer did not emit a bare group:\n%s", got)
	}
}

func TestLayerOpacities(t *testing.T) {
	cases := []struct {
		opacity float64
		want    string
	}{
		{0, "opacity:0"},
		{0.25, "opacity:0.25"},
		{1, ""},
		{1.5, ""}, // clamped to 1
		{-0.5, "opacity:0"},
	}
	for _, c := range cases {
		svg := NewSvg()
		gc := NewGraphicContext(svg)
		if err := gc.BeginLayer(draw2d.BlendNormal, c.opacity); err != nil {
			t.Fatalf("opacity %v: %v", c.opacity, err)
		}
		draw2dkit.Rectangle(gc, 0, 0, 10, 10)
		gc.Fill()
		if err := gc.EndLayer(); err != nil {
			t.Fatal(err)
		}
		got := marshal(t, svg)
		if c.want == "" {
			if strings.Contains(got, "opacity") {
				t.Errorf("opacity %v emitted a declaration:\n%s", c.opacity, got)
			}
			continue
		}
		if !strings.Contains(got, c.want) {
			t.Errorf("opacity %v: missing %q in\n%s", c.opacity, c.want, got)
		}
	}
}

func TestLayerEmptyProducesNothing(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	if err := gc.BeginLayer(draw2d.BlendMultiply, 1); err != nil {
		t.Fatal(err)
	}
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	got := marshal(t, svg)
	want := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs></defs>
</svg>`
	if got != want {
		t.Errorf("empty layer produced output\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLayerNestedEmitsNestedGroups(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	if err := gc.BeginLayer(draw2d.BlendScreen, 1); err != nil {
		t.Fatal(err)
	}
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()
	if err := gc.BeginLayer(draw2d.BlendMultiply, 1); err != nil {
		t.Fatal(err)
	}
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	got := marshal(t, svg)
	want := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs></defs>
  <g style="mix-blend-mode:screen">
    <g fill="#FFFFFF" fill-rule="evenodd">
      <path d="M 0,0 L 10,0 L 10,10 L 0,10 Z"></path>
    </g>
    <g style="mix-blend-mode:multiply">
      <g fill="#FFFFFF" fill-rule="evenodd">
        <path d="M 0,0 L 10,0 L 10,10 L 0,10 Z"></path>
      </g>
    </g>
  </g>
</svg>`
	if got != want {
		t.Errorf("nested layers output is not as expected\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLayerErrors(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)

	if err := gc.EndLayer(); !errors.Is(err, draw2d.ErrLayerNotOpen) {
		t.Errorf("EndLayer without layer = %v, want ErrLayerNotOpen", err)
	}
	if err := gc.BeginLayer(draw2d.BlendMode(12), 1); !errors.Is(err, draw2d.ErrInvalidBlendMode) {
		t.Errorf("BeginLayer invalid mode = %v, want ErrInvalidBlendMode", err)
	}
	if err := gc.BeginLayer(draw2d.BlendMultiply, math.NaN()); !errors.Is(err, draw2d.ErrInvalidOpacity) {
		t.Errorf("BeginLayer NaN = %v, want ErrInvalidOpacity", err)
	}
	if err := gc.BeginLayer(draw2d.BlendMultiply, math.Inf(-1)); !errors.Is(err, draw2d.ErrInvalidOpacity) {
		t.Errorf("BeginLayer -Inf = %v, want ErrInvalidOpacity", err)
	}
	if err := gc.EndLayer(); !errors.Is(err, draw2d.ErrLayerNotOpen) {
		t.Errorf("EndLayer after rejected BeginLayer = %v, want ErrLayerNotOpen", err)
	}

	// A nil SVG is rejected instead of panicking.
	nilGC := NewGraphicContext(nil)
	if err := nilGC.BeginLayer(draw2d.BlendMultiply, 1); !errors.Is(err, draw2d.ErrBlendUnsupported) {
		t.Errorf("BeginLayer on nil svg = %v, want ErrBlendUnsupported", err)
	}
}

func TestLayerDepthLimit(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	for i := 0; i < draw2d.MaxLayerDepth; i++ {
		if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
			t.Fatalf("BeginLayer %d: %v", i, err)
		}
	}
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); !errors.Is(err, draw2d.ErrLayerDepthExceeded) {
		t.Errorf("BeginLayer past limit = %v, want ErrLayerDepthExceeded", err)
	}
	for i := 0; i < draw2d.MaxLayerDepth; i++ {
		if err := gc.EndLayer(); err != nil {
			t.Fatalf("EndLayer %d: %v", i, err)
		}
	}
}

func TestLayerStateRestoredOnEnd(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.SetFillColor(nil)
	if err := gc.BeginLayer(draw2d.BlendMultiply, 0.5); err != nil {
		t.Fatal(err)
	}
	gc.SetBlendMode(draw2d.BlendScreen)
	gc.Translate(5, 5)
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	if !gc.Current.Tr.IsIdentity() {
		t.Errorf("transform not restored: %v", gc.Current.Tr)
	}
	if gc.GetBlendMode() != draw2d.BlendNormal {
		t.Errorf("blend mode not restored: %v", gc.GetBlendMode())
	}
}

func TestClearInsideLayer(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()
	if err := gc.BeginLayer(draw2d.BlendMultiply, 1); err != nil {
		t.Fatal(err)
	}
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()
	gc.Clear()
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	got := marshal(t, svg)
	want := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs></defs>
</svg>`
	if got != want {
		t.Errorf("Clear did not clear layers\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestClearRectInsideLayer(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	if err := gc.BeginLayer(draw2d.BlendMultiply, 1); err != nil {
		t.Fatal(err)
	}
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()
	gc.ClearRect(0, 0, 5, 5)
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	got := marshal(t, svg)
	// The layer group must contain the ClearRect mask group wrapping the path.
	want := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs>
    <mask id="mask-1">
      <rect width="100%" height="100%" fill="#fff"></rect>
      <rect width="5" height="5" fill="#000"></rect>
    </mask>
  </defs>
  <g style="mix-blend-mode:multiply">
    <g mask="url(#mask-1)">
      <g fill="#FFFFFF" fill-rule="evenodd">
        <path d="M 0,0 L 10,0 L 10,10 L 0,10 Z"></path>
      </g>
    </g>
  </g>
</svg>`
	if got != want {
		t.Errorf("ClearRect output is not as expected\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestDrawImageBlendMode(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	if err := gc.SetBlendMode(draw2d.BlendScreen); err != nil {
		t.Fatal(err)
	}
	gc.DrawImage(solidImage(2, 2))
	got := marshal(t, svg)
	if !strings.Contains(got, `style="mix-blend-mode:screen"`) {
		t.Errorf("image draw lacks blend style:\n%s", got)
	}
}

// solidImage builds an opaque image used by DrawImage tests.
func solidImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+3] = 255, 255
	}
	return img
}

func TestBlendModeText(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	if err := gc.SetBlendMode(draw2d.BlendMultiply); err != nil {
		t.Fatal(err)
	}
	gc.SetFontSize(12)
	gc.FillStringAt("A", 0, 0)
	got := marshal(t, svg)
	if !strings.Contains(got, `style="mix-blend-mode:multiply"`) {
		t.Errorf("text draw lacks blend style:\n%s", got)
	}
}

func TestBlendModePersistsAcrossLayers(t *testing.T) {
	// A layer opened while a blend mode is active must leave the outer mode
	// untouched: inside the layer drawing is normal, outside it stays screen.
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	if err := gc.SetBlendMode(draw2d.BlendScreen); err != nil {
		t.Fatal(err)
	}
	if err := gc.BeginLayer(draw2d.BlendMultiply, 1); err != nil {
		t.Fatal(err)
	}
	if got := gc.GetBlendMode(); got != draw2d.BlendNormal {
		t.Errorf("inside layer blend mode = %v, want normal", got)
	}
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	if got := gc.GetBlendMode(); got != draw2d.BlendScreen {
		t.Errorf("after layer blend mode = %v, want screen", got)
	}
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()

	got := marshal(t, svg)
	if strings.Count(got, "mix-blend-mode:screen") != 1 {
		t.Errorf("want exactly one screen group, got:\n%s", got)
	}
	if strings.Count(got, "mix-blend-mode:multiply") != 1 {
		t.Errorf("want exactly one multiply group, got:\n%s", got)
	}
}

func TestLayerWithTransform(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.Translate(3, 4)
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
		t.Fatal(err)
	}
	draw2dkit.Rectangle(gc, 0, 0, 1, 1)
	gc.Fill()
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	got := marshal(t, svg)
	// The transform from before the layer is restored and applies to each
	// element inside the group, exactly as it would without a layer.
	want := `<svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="none">
  <defs></defs>
  <g>
    <g fill="#FFFFFF" fill-rule="evenodd" transform="translate(3,4)">
      <path d="M 0,0 L 1,0 L 1,1 L 0,1 Z"></path>
    </g>
  </g>
</svg>`
	if got != want {
		t.Errorf("layer with transform output is not as expected\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLayerIsolatesBlendedChildren(t *testing.T) {
	// A plain (normal, opaque) layer whose children use a blend mode would let
	// those children blend with the backdrop unless the layer is isolated.
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
		t.Fatal(err)
	}
	if err := gc.SetBlendMode(draw2d.BlendMultiply); err != nil {
		t.Fatal(err)
	}
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	got := marshal(t, svg)
	if !strings.Contains(got, `style="isolation:isolate"`) {
		t.Errorf("layer with blended child is not isolated:\n%s", got)
	}
}

func TestLayerNoIsolationWithoutBlendedChildren(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	if err := gc.BeginLayer(draw2d.BlendNormal, 1); err != nil {
		t.Fatal(err)
	}
	draw2dkit.Rectangle(gc, 0, 0, 10, 10)
	gc.Fill()
	if err := gc.EndLayer(); err != nil {
		t.Fatal(err)
	}
	got := marshal(t, svg)
	if strings.Contains(got, "isolation") {
		t.Errorf("plain layer was isolated unnecessarily:\n%s", got)
	}
}
