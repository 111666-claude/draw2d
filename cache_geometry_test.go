package draw2d_test

import (
	"encoding/xml"
	"image"
	"strings"
	"testing"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dbase"
	"github.com/llgcode/draw2d/draw2dimg"
	"github.com/llgcode/draw2d/draw2dpdf"
	"github.com/llgcode/draw2d/draw2dsvg"
)

// TestGlyphGeometryUnchanged requires the new concurrent caches to produce the
// exact same glyph widths and path geometry as the legacy non-thread-safe
// implementations.
func TestGlyphGeometryUnchanged(t *testing.T) {
	draw2d.SetFontFolder("resource/font")
	withGlobalFontCache(t, draw2d.NewDefaultSyncFontCache("resource/font"))
	const text = "The quick brown fox jumps 0123456789"

	// Image backend: compare width and rendered pixels between a context using
	// the legacy glyph cache and one using the new concurrent cache.
	renderImg := func(useLegacy bool) (float64, *image.RGBA) {
		dest := image.NewRGBA(image.Rect(0, 0, 400, 120))
		gc := draw2dimg.NewGraphicContext(dest)
		if useLegacy {
			gc.SetGlyphCache(draw2dbase.NewGlyphCache())
		}
		gc.SetFontSize(20)
		width := gc.FillStringAt(text, 8, 40)
		return width, dest
	}
	legacyWidth, legacyImg := renderImg(true)
	newWidth, newImg := renderImg(false)
	if legacyWidth != newWidth {
		t.Fatalf("image glyph width changed: legacy=%v new=%v", legacyWidth, newWidth)
	}
	if ok, at := pixelsEqual(legacyImg, newImg); !ok {
		t.Fatalf("image pixels differ at offset %d", at)
	}

	// SVG backend: the serialized glyph path descriptions must be identical.
	renderSvg := func(useLegacy bool) string {
		svg := draw2dsvg.NewSvg()
		gc := draw2dsvg.NewGraphicContext(svg)
		if useLegacy {
			gc.SetGlyphCache(draw2dbase.NewGlyphCache())
		}
		gc.SetFontSize(20)
		gc.FillStringAt(text, 8, 40)
		out, err := xml.MarshalIndent(svg, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	if a, b := renderSvg(true), renderSvg(false); a != b {
		t.Fatal("SVG glyph path descriptions differ between legacy and new caches")
	}

	// Direct glyph-level check: widths and path component counts match for a
	// range of runes through both cache implementations.
	imgGC := draw2dimg.NewGraphicContext(image.NewRGBA(image.Rect(0, 0, 400, 120)))
	imgGC.SetFontSize(24)
	fontName := imgGC.GetFontName()
	legacy := draw2dbase.NewGlyphCache()
	concurrent := draw2dbase.NewDefaultSyncGlyphCache()
	defer concurrent.Close()
	for _, r := range "draw2d LRU+TTL 0123456789 !?" {
		g1 := legacy.Fetch(imgGC, fontName, r)
		g2 := concurrent.Fetch(imgGC, fontName, r)
		if g1.Width != g2.Width {
			t.Fatalf("rune %q width: legacy=%v new=%v", r, g1.Width, g2.Width)
		}
		if g1.Path.String() != g2.Path.String() {
			t.Fatalf("rune %q path geometry differs", r)
		}
	}
}

// TestPDFBackendUnaffected renders through the PDF backend, which has no glyph
// cache of its own but consumes the global font cache; output must succeed and
// stay deterministic.
func TestPDFBackendUnaffected(t *testing.T) {
	draw2d.SetFontFolder("resource/font")
	withGlobalFontCache(t, draw2d.NewDefaultSyncFontCache("resource/font"))

	render := func() string {
		pdf := draw2dpdf.NewPdf("L", "mm", "A4")
		gc := draw2dpdf.NewGraphicContext(pdf)
		// luximbi is the font shipped with its gofpdf json/.z metrics.
		gc.SetFontData(draw2d.FontData{
			Name:   "luxi",
			Family: draw2d.FontFamilyMono,
			Style:  draw2d.FontStyleBold | draw2d.FontStyleItalic,
		})
		gc.SetFontSize(14)
		gc.FillStringAt("PDF backend font cache check 0123456789", 10, 20)
		var sb strings.Builder
		if err := pdf.Output(&sb); err != nil {
			t.Fatal(err)
		}
		return sb.String()
	}
	first := render()
	second := render()
	if first != second {
		t.Fatal("PDF output is not deterministic with the bounded font cache")
	}
	if len(first) == 0 {
		t.Fatal("PDF output was empty")
	}
}

// TestMemoryBoundGlyphCache asserts that under a steady stream of distinct
// glyphs the cache never exceeds its configured capacity.
func TestMemoryBoundGlyphCache(t *testing.T) {
	const capacity = 32
	cache := draw2dbase.NewSyncGlyphCache(draw2dbase.GlyphCacheConfig{Capacity: capacity})
	defer cache.Close()
	gc := draw2dimg.NewGraphicContext(image.NewRGBA(image.Rect(0, 0, 256, 256)))
	gc.SetFontSize(16)

	for i := 0; i < 5000; i++ {
		r := rune('a' + i%200) // 200 distinct glyphs, capacity 32
		if g := cache.Fetch(gc, gc.GetFontName(), r); g == nil {
			t.Fatalf("nil glyph for rune %d", r)
		}
		if cache.Len() > capacity {
			t.Fatalf("Len = %d exceeds capacity %d at iteration %d", cache.Len(), capacity, i)
		}
	}
	s := cache.Stats()
	if s.Evictions == 0 {
		t.Fatalf("expected evictions, stats=%+v", s)
	}
	t.Logf("glyph memory bound: final Len=%d evictions=%d loads=%d hits=%d",
		cache.Len(), s.Evictions, s.Loads, s.Hits)
}

// TestMemoryBoundFontCache asserts the font cache never exceeds its capacity
// even when many distinct FontData values are requested.
func TestMemoryBoundFontCache(t *testing.T) {
	const capacity = 4
	loader := newCountingLoader()
	font := mustParseTTF(t, "resource/font/luxisr.ttf")
	const distinct = 40
	for i := 0; i < distinct; i++ {
		loader.addFont(testFontData("font-"+string(rune('A'+i))), font)
	}
	cache, err := draw2d.NewSyncFontCache(loader, draw2d.FontCacheConfig{Capacity: capacity})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	for i := 0; i < 2000; i++ {
		fd := testFontData("font-" + string(rune('A'+i%distinct)))
		if _, err := cache.Load(fd); err != nil {
			t.Fatal(err)
		}
		if cache.Len() > capacity {
			t.Fatalf("Len = %d exceeds capacity %d", cache.Len(), capacity)
		}
	}
	s := cache.Stats()
	if s.Evictions == 0 {
		t.Fatalf("expected font evictions, stats=%+v", s)
	}
	t.Logf("font memory bound: final Len=%d evictions=%d loads=%d",
		cache.Len(), s.Evictions, s.Loads)
}
