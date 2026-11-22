package draw2dimg

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dbase"
)

// scenePath builds a deterministic complex path with many curves and arcs.
func scenePath() *draw2d.Path {
	p := new(draw2d.Path)
	p.MoveTo(20, 20)
	x, y := 20.0, 20.0
	for i := 0; i < 60; i++ {
		f := float64(i%13) + 1
		p.CubicCurveTo(x+f*7, y-f*3, x+f*2, y+f*9, x+f*5, y+f*4)
		x, y = x+f*5, y+f*4
		p.QuadCurveTo(x+f*6, y+f, x-f*2, y+f*3)
		x, y = x-f*2, y+f*3
		p.ArcTo(x, y, f*4, f*2, 0.3*f, math.Pi*1.2)
		x += f
	}
	p.Close()
	return p
}

// renderScene renders a repeated complex scene, optionally with the shared
// flatten cache replaced by cache (nil disables caching).
func renderScene(cache *draw2dbase.FlattenCache, frames int, mutate func(gc *GraphicContext, frame int)) *image.RGBA {
	old := draw2dbase.SharedFlattenCache()
	draw2dbase.SetSharedFlattenCache(cache)
	defer draw2dbase.SetSharedFlattenCache(old)

	dest := image.NewRGBA(image.Rect(0, 0, 400, 400))
	gc := NewGraphicContext(dest)
	gc.SetFillColor(color.RGBA{0x40, 0x80, 0xcc, 0xff})
	gc.SetStrokeColor(color.RGBA{0x10, 0x10, 0x10, 0xff})
	gc.SetLineWidth(2)
	path := scenePath()
	for i := 0; i < frames; i++ {
		if mutate != nil {
			mutate(gc, i)
		}
		gc.FillStroke(path)
	}
	return dest
}

func TestFlattenCachePixelIdentity(t *testing.T) {
	reference := renderScene(nil, 3, nil) // no cache at all
	cached := renderScene(draw2dbase.NewFlattenCache(draw2dbase.DefaultFlattenCacheCapacity), 3, nil)
	if !bytes.Equal(reference.Pix, cached.Pix) {
		t.Fatal("cached rendering differs pixel by pixel from uncached rendering")
	}
}

func TestFlattenCacheLineCapStyles(t *testing.T) {
	// Line caps are applied by the stroker downstream of the flattening, so
	// a cached flattening shared between cap styles must still produce the
	// exact uncached output for each style. All styles share one cache here,
	// so later styles are served by entries stored for earlier ones.
	cache := draw2dbase.NewFlattenCache(draw2dbase.DefaultFlattenCacheCapacity)
	for _, cap := range []draw2d.LineCap{draw2d.RoundCap, draw2d.ButtCap, draw2d.SquareCap} {
		setCap := func(gc *GraphicContext, frame int) {
			if frame == 0 {
				gc.SetLineCap(cap)
			}
		}
		reference := renderScene(nil, 3, setCap)
		cached := renderScene(cache, 3, setCap)
		if !bytes.Equal(reference.Pix, cached.Pix) {
			t.Fatalf("cap %v: cached rendering differs from uncached rendering", cap)
		}
	}
	hits, misses, _ := cache.Stats()
	if hits == 0 || misses == 0 {
		t.Fatalf("expected the cache to be shared across cap styles: hits=%d misses=%d", hits, misses)
	}

	// Sanity check: the comparison is sensitive to stroke state that does
	// affect rasterization, e.g. the line width.
	thin := renderScene(nil, 1, func(gc *GraphicContext, frame int) { gc.SetLineWidth(2) })
	thick := renderScene(nil, 1, func(gc *GraphicContext, frame int) { gc.SetLineWidth(8) })
	if bytes.Equal(thin.Pix, thick.Pix) {
		t.Fatal("line widths 2 and 8 produced identical images, test is not sensitive")
	}
}

func TestFlattenCacheScaleChange(t *testing.T) {
	// Growing the transform between frames must not reuse the flattening
	// computed for another scale.
	grow := func(gc *GraphicContext, frame int) {
		gc.Scale(1+0.1*float64(frame), 1+0.1*float64(frame))
	}
	reference := renderScene(nil, 4, grow)
	cached := renderScene(draw2dbase.NewFlattenCache(draw2dbase.DefaultFlattenCacheCapacity), 4, grow)
	if !bytes.Equal(reference.Pix, cached.Pix) {
		t.Fatal("cached rendering with scale changes differs from uncached rendering")
	}
}

func TestFlattenCacheSaveRestoreClear(t *testing.T) {
	// Save/Restore copy the path and Clear/BeginPath reset it; none of these
	// may be served stale cache content.
	render := func(cache *draw2dbase.FlattenCache) *image.RGBA {
		old := draw2dbase.SharedFlattenCache()
		draw2dbase.SetSharedFlattenCache(cache)
		defer draw2dbase.SetSharedFlattenCache(old)

		dest := image.NewRGBA(image.Rect(0, 0, 400, 400))
		gc := NewGraphicContext(dest)
		gc.SetStrokeColor(color.RGBA{0, 0, 0, 0xff})
		path := scenePath()
		for i := 0; i < 3; i++ {
			gc.Save()
			gc.Translate(float64(i)*3, float64(i)*2)
			gc.Stroke(path)
			gc.Restore()
			gc.BeginPath()
			gc.MoveTo(10+float64(i), 10)
			gc.CubicCurveTo(50, 90, 150, 30, 200, 100)
			gc.Stroke()
			gc.Clear()
		}
		return dest
	}
	reference := render(nil)
	cached := render(draw2dbase.NewFlattenCache(draw2dbase.DefaultFlattenCacheCapacity))
	if !bytes.Equal(reference.Pix, cached.Pix) {
		t.Fatal("cached rendering with Save/Restore/Clear differs from uncached rendering")
	}
}

func BenchmarkComplexSceneUncached(b *testing.B) {
	benchmarkComplexScene(b, nil)
}

func BenchmarkComplexSceneCached(b *testing.B) {
	benchmarkComplexScene(b, draw2dbase.NewFlattenCache(draw2dbase.DefaultFlattenCacheCapacity))
}

func BenchmarkTextUncached(b *testing.B) {
	benchmarkText(b, nil)
}

func BenchmarkTextCached(b *testing.B) {
	benchmarkText(b, draw2dbase.NewFlattenCache(draw2dbase.DefaultFlattenCacheCapacity))
}

// benchmarkText renders the same strings every frame; each glyph outline is
// re-flattened per frame without the cache.
func benchmarkText(b *testing.B, cache *draw2dbase.FlattenCache) {
	initFontCache()
	old := draw2dbase.SharedFlattenCache()
	draw2dbase.SetSharedFlattenCache(cache)
	defer draw2dbase.SetSharedFlattenCache(old)

	dest := image.NewRGBA(image.Rect(0, 0, 400, 400))
	gc := NewGraphicContext(dest)
	gc.SetFontData(draw2d.FontData{Name: "goregular"})
	gc.SetFontSize(14)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for line := 0; line < 10; line++ {
			gc.FillStringAt("The quick brown fox jumps over the lazy dog", 10, 20+float64(line)*30)
		}
	}
}

func benchmarkComplexScene(b *testing.B, cache *draw2dbase.FlattenCache) {
	old := draw2dbase.SharedFlattenCache()
	draw2dbase.SetSharedFlattenCache(cache)
	defer draw2dbase.SetSharedFlattenCache(old)

	dest := image.NewRGBA(image.Rect(0, 0, 400, 400))
	gc := NewGraphicContext(dest)
	gc.SetLineWidth(2)
	path := scenePath()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gc.FillStroke(path)
	}
}
