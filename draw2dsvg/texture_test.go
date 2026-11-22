// Copyright 2026 The draw2d Authors. All rights reserved.

package draw2dsvg

import (
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/llgcode/draw2d"
)

func checkerTexture() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	img.SetRGBA(1, 0, color.RGBA{G: 255, A: 255})
	img.SetRGBA(0, 1, color.RGBA{B: 255, A: 255})
	img.SetRGBA(1, 1, color.RGBA{R: 255, G: 255, A: 255})
	return img
}

func fillSquare(gc *GraphicContext) {
	gc.MoveTo(0, 0)
	gc.LineTo(20, 0)
	gc.LineTo(20, 20)
	gc.LineTo(0, 20)
	gc.Close()
	gc.Fill()
}

func TestSvgImageTexturePattern(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	fillSquare(gc)

	if len(svg.Patterns) != 1 {
		t.Fatalf("got %d pattern defs, want 1", len(svg.Patterns))
	}
	p := svg.Patterns[0]
	if p.Id != "pattern-1" {
		t.Errorf("id = %q, want pattern-1", p.Id)
	}
	if p.Width != "2" || p.Height != "2" {
		t.Errorf("tile size = %s x %s, want 2 x 2", p.Width, p.Height)
	}
	if p.X != 0 || p.Y != 0 {
		t.Errorf("tile origin = (%v,%v), want (0,0)", p.X, p.Y)
	}
	if p.PatternUnits != "userSpaceOnUse" || p.ContentUnits != "userSpaceOnUse" {
		t.Errorf("units = %q/%q", p.PatternUnits, p.ContentUnits)
	}
	if p.PatternTransform != "" {
		t.Errorf("identity transform should be omitted, got %q", p.PatternTransform)
	}
	if len(p.Images) != 1 {
		t.Fatalf("got %d images, want 1", len(p.Images))
	}
	if got := p.Images[0]; got.Width != "2" || got.Height != "2" || !strings.HasPrefix(got.Href, "data:image/png;base64,") {
		t.Errorf("image = %+v", got)
	}

	if len(svg.Groups) != 1 || svg.Groups[0].Fill != "url(#pattern-1)" {
		t.Fatalf("group fill = %q, want pattern reference", svg.Groups[0].Fill)
	}

	out, err := xml.MarshalIndent(svg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<pattern id="pattern-1" x="0" y="0" width="2" height="2" patternUnits="userSpaceOnUse" patternContentUnits="userSpaceOnUse">`,
		`<image width="2" height="2" href="data:image/png;base64,`,
		`<g fill="url(#pattern-1)" fill-rule="evenodd">`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("SVG output missing %q\n%s", want, out)
		}
	}
}

func TestSvgImageTextureRepeatPolicies(t *testing.T) {
	for _, tc := range []struct {
		name         string
		repeat       draw2d.TextureRepeat
		wantW, wantH string
	}{
		{"both", draw2d.TextureRepeatBoth, "2", "2"},
		{"x only", draw2d.TextureRepeatX, "2", "1048576"},
		{"y only", draw2d.TextureRepeatY, "1048576", "2"},
		{"clamp", draw2d.TextureClamp, "1048576", "1048576"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svg := NewSvg()
			gc := NewGraphicContext(svg)
			gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, tc.repeat))
			fillSquare(gc)
			if len(svg.Patterns) != 1 {
				t.Fatalf("got %d patterns", len(svg.Patterns))
			}
			p := svg.Patterns[0]
			if p.Width != tc.wantW || p.Height != tc.wantH {
				t.Errorf("tile = %s x %s, want %s x %s", p.Width, p.Height, tc.wantW, tc.wantH)
			}
			// The image keeps its own extent regardless of the tile size, so a
			// clamped axis stays transparent rather than stretching.
			if p.Images[0].Width != "2" || p.Images[0].Height != "2" {
				t.Errorf("image size = %s x %s, want 2 x 2", p.Images[0].Width, p.Images[0].Height)
			}
		})
	}
}

func TestSvgImageTextureClampTileScalesWithTransform(t *testing.T) {
	// A clamped axis must stay large in user space even when the texture is
	// scaled down onto the canvas, otherwise the tile would repeat visibly.
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.Scale(0.01, 0.01)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.NewScaleMatrix(0.01, 0.01), draw2d.TextureClamp))
	fillSquare(gc)
	var got float64
	if _, err := fmt.Sscanf(svg.Patterns[0].Width, "%f", &got); err != nil {
		t.Fatalf("width %q is not a number: %v", svg.Patterns[0].Width, err)
	}
	// 1<<20 / (0.01 * 0.01) = 1.048576e10 user units.
	if got < 1e9 {
		t.Errorf("clamp tile width = %v user units, want > 1e9", got)
	}
}

func TestSvgImageTextureNonZeroBoundsPhase(t *testing.T) {
	src := image.NewRGBA(image.Rect(10, 20, 12, 22))
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.SetFillImageTexture(draw2d.NewImageTexture(src, draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	fillSquare(gc)
	p := svg.Patterns[0]
	if p.X != 10 || p.Y != 20 {
		t.Errorf("tile origin = (%v,%v), want Bounds.Min (10,20)", p.X, p.Y)
	}
	if p.Images[0].X != 10 || p.Images[0].Y != 20 {
		t.Errorf("image origin = (%v,%v), want Bounds.Min", p.Images[0].X, p.Images[0].Y)
	}
	if p.Width != "2" || p.Height != "2" {
		t.Errorf("tile = %s x %s, want the Bounds extent", p.Width, p.Height)
	}
}

func TestSvgImageTextureLocalTransform(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	local := draw2d.NewRotationMatrix(math.Pi / 4)
	local.Translate(5, 6)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), local, draw2d.TextureRepeatBoth))
	fillSquare(gc)
	p := svg.Patterns[0]
	want := toSvgTransform(local)
	if p.PatternTransform != want {
		t.Errorf("patternTransform = %q, want %q", p.PatternTransform, want)
	}
	out, err := xml.MarshalIndent(svg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `patternTransform="matrix(`) {
		t.Errorf("SVG output missing patternTransform\n%s", out)
	}

	// The local transform must not be folded into the group matrix, which
	// carries the CTM only.
	if got := svg.Groups[0].Transform; got != "" {
		t.Errorf("group transform = %q, want identity", got)
	}
}

func TestSvgImageTextureTranslatesWithCTM(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.Translate(3, 4)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	fillSquare(gc)
	// The CTM belongs to the group, so the pattern phase follows the path.
	if got := svg.Groups[0].Transform; got != "translate(3,4)" {
		t.Errorf("group transform = %q", got)
	}
	if got := svg.Patterns[0].PatternTransform; got != "" {
		t.Errorf("pattern transform = %q, want identity", got)
	}
}

func TestSvgImageTextureFallbacks(t *testing.T) {
	nilTex := draw2d.NewImageTexture(nil, draw2d.Matrix{}, draw2d.TextureClamp)
	empty := draw2d.NewImageTexture(image.NewRGBA(image.Rect(0, 0, 0, 0)), draw2d.Matrix{}, draw2d.TextureClamp)
	nan := math.NaN()
	for _, tc := range []struct {
		name string
		tex  *draw2d.ImageTexture
	}{
		{"nil", nil},
		{"nil image", nilTex},
		{"empty", empty},
		{"non-finite", draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{nan, 0, 0, 1, 0, 0}, draw2d.TextureClamp)},
		{"singular", draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{1, 2, 2, 4, 0, 0}, draw2d.TextureClamp)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svg := NewSvg()
			gc := NewGraphicContext(svg)
			solid := color.RGBA{R: 10, G: 20, B: 30, A: 255}
			gc.SetFillColor(solid)
			gc.SetFillImageTexture(tc.tex)
			fillSquare(gc)
			if len(svg.Patterns) != 0 {
				t.Errorf("got %d patterns, want none", len(svg.Patterns))
			}
			if got, want := svg.Groups[0].Fill, toSvgRGBA(solid); got != want {
				t.Errorf("fill = %q, want solid %q", got, want)
			}
		})
	}
}

func TestSvgImageTextureInvalidRepeatEmitsClampTile(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeat(42)))
	fillSquare(gc)
	// Normalize turns an unknown policy into clamp, so the tile is large on
	// both axes and the SVG stays valid.
	p := svg.Patterns[0]
	if p.Width != "1048576" || p.Height != "1048576" {
		t.Errorf("tile = %s x %s, want clamp extents", p.Width, p.Height)
	}
}

func TestSvgImageTextureSamplingMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		samp draw2d.TextureSampling
		want string
	}{
		{"bilinear", draw2d.TextureSamplingBilinear, ""},
		{"nearest", draw2d.TextureSamplingNearest, "pixelated"},
		{"unknown falls back", draw2d.TextureSampling(77), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svg := NewSvg()
			gc := NewGraphicContext(svg)
			gc.SetFillImageTexture(draw2d.NewImageTextureWithSampling(
				checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth, tc.samp))
			fillSquare(gc)
			if got := svg.Patterns[0].Images[0].ImageRendering; got != tc.want {
				t.Errorf("image-rendering = %q, want %q", got, tc.want)
			}
			out, err := xml.Marshal(svg)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" && strings.Contains(string(out), "image-rendering") {
				t.Errorf("bilinear output should omit image-rendering\n%s", out)
			}
			if tc.want != "" && !strings.Contains(string(out), `image-rendering="`+tc.want+`"`) {
				t.Errorf("output missing image-rendering\n%s", out)
			}
		})
	}
}

func TestSvgImageTextureNonFiniteCTM(t *testing.T) {
	// A non-finite CTM cannot be represented in SVG, so the fill falls back to
	// the solid colour and the document stays well formed.
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	solid := color.RGBA{R: 4, G: 5, B: 6, A: 255}
	gc.SetFillColor(solid)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	gc.SetMatrixTransform(draw2d.Matrix{math.NaN(), 0, 0, 1, 0, 0})
	fillSquare(gc)
	if len(svg.Patterns) != 0 {
		t.Errorf("got %d patterns for non-finite CTM", len(svg.Patterns))
	}
	if got, want := svg.Groups[0].Fill, toSvgRGBA(solid); got != want {
		t.Errorf("fill = %q, want solid %q", got, want)
	}
	if out, err := xml.Marshal(svg); err != nil || strings.Contains(string(out), "NaN") {
		t.Errorf("invalid XML: %v\n%s", err, out)
	}
}

func TestSvgImageTextureSharesDefsWithMask(t *testing.T) {
	// Patterns, masks and font glyphs all live under a single <defs>; adding a
	// pattern must not duplicate or corrupt the defs block.
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	svg.Masks = append(svg.Masks, &Mask{})
	gc.ClearRect(0, 0, 10, 10)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	fillSquare(gc)

	out, err := xml.MarshalIndent(svg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if n := strings.Count(s, "<defs>"); n != 1 {
		t.Errorf("got %d <defs> blocks, want 1\n%s", n, s)
	}
	for _, want := range []string{"<mask", "<pattern", "</defs>"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q\n%s", want, s)
		}
	}
	var v any
	if err := xml.Unmarshal(out, &v); err != nil {
		t.Errorf("output is not valid XML: %v\n%s", err, s)
	}
}

func TestSvgSolidFillUnchanged(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	solid := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	gc.SetFillColor(solid) // must drop the texture
	fillSquare(gc)
	if len(svg.Patterns) != 0 {
		t.Errorf("SetFillColor left %d patterns", len(svg.Patterns))
	}
	if got, want := svg.Groups[0].Fill, toSvgRGBA(solid); got != want {
		t.Errorf("fill = %q, want %q", got, want)
	}
}

func TestSvgImageTextureSaveRestore(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	gc.Save()
	gc.SetFillColor(color.Black)
	gc.Restore()
	fillSquare(gc)
	if len(svg.Patterns) != 1 {
		t.Fatalf("restore lost the texture: %d patterns", len(svg.Patterns))
	}
	if got := svg.Groups[0].Fill; !strings.HasPrefix(got, "url(#pattern-") {
		t.Errorf("fill = %q, want pattern reference", got)
	}
}

func TestSvgImageTextureTwoFillsIndependent(t *testing.T) {
	// Two fills with different textures must each get their own definition and
	// reference, so draw order does not alias them.
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	fillSquare(gc)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.NewScaleMatrix(3, 3), draw2d.TextureClamp))
	fillSquare(gc)
	if len(svg.Patterns) != 2 {
		t.Fatalf("got %d patterns, want 2", len(svg.Patterns))
	}
	if svg.Groups[0].Fill == svg.Groups[1].Fill {
		t.Errorf("both groups reference %q", svg.Groups[0].Fill)
	}
	if svg.Patterns[0].PatternTransform != "" || svg.Patterns[1].PatternTransform == "" {
		t.Errorf("pattern transforms are not independent")
	}
}

func TestSvgImageTextureStrokeOnlyHasNoPattern(t *testing.T) {
	svg := NewSvg()
	gc := NewGraphicContext(svg)
	gc.SetFillImageTexture(draw2d.NewImageTexture(checkerTexture(), draw2d.Matrix{}, draw2d.TextureRepeatBoth))
	gc.MoveTo(0, 0)
	gc.LineTo(10, 10)
	gc.Stroke()
	if len(svg.Patterns) != 0 {
		t.Errorf("stroke-only group emitted %d patterns", len(svg.Patterns))
	}
	if got := svg.Groups[0].Fill; got != "" {
		t.Errorf("stroke-only group fill = %q, want empty", got)
	}
}
