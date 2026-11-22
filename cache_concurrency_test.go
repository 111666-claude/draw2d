package draw2d_test

import (
	"context"
	"encoding/xml"
	"errors"
	"image"
	"sync"
	"testing"
	"time"

	"github.com/golang/freetype/truetype"
	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dbase"
	"github.com/llgcode/draw2d/draw2dimg"
	"github.com/llgcode/draw2d/draw2dkit"
	"github.com/llgcode/draw2d/draw2dsvg"
)

// draw2dbaseDefaultFontData returns the package's default FontData without the
// test package having to import draw2dbase solely for the variable.
func draw2dbaseDefaultFontData() draw2d.FontData {
	return draw2dbase.DefaultFontData
}

// renderPoster renders the same fixed-font text scene into a fresh RGBA image.
// The output is deterministic and used for pixel-level comparison across cache
// implementations and goroutines.
func renderPoster(t *testing.T) image.Image {
	t.Helper()
	dest := image.NewRGBA(image.Rect(0, 0, 320, 240))
	gc := draw2dimg.NewGraphicContext(dest)

	draw2dkit.RoundedRectangle(gc, 5, 5, 300, 220, 12, 12)
	gc.SetFillColor(image.White)
	gc.SetStrokeColor(image.Black)
	gc.FillStroke()

	gc.SetFillColor(image.Black)
	gc.SetFontSize(24)
	gc.FillStringAt("Concurrent glyph cache", 16, 40)

	gc.SetFontSize(14)
	for i, line := range []string{
		"bounded LRU + TTL eviction",
		"single-flight font parsing",
		"identical pixels, no races",
		"0123456789 !?&@#",
	} {
		gc.FillStringAt(line, 16, float64(80+i*24))
	}
	return dest
}

func pixelsEqual(a, b *image.RGBA) (bool, int) {
	if len(a.Pix) != len(b.Pix) {
		return false, -1
	}
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			return false, i
		}
	}
	return true, 0
}

// withGlobalFontCache temporarily installs cache as the global font cache and
// restores the previous one, including on test failure.
func withGlobalFontCache(t *testing.T, cache draw2d.FontCache) {
	t.Helper()
	previous := draw2d.GetGlobalFontCache()
	draw2d.SetFontCache(cache)
	t.Cleanup(func() { draw2d.SetFontCache(previous) })
}

// TestPosterPixelsIdenticalAcrossCaches requires the new bounded concurrent
// cache to produce byte-identical output to the legacy folder cache.
func TestPosterPixelsIdenticalAcrossCaches(t *testing.T) {
	draw2d.SetFontFolder("resource/font")

	// Reference image with the legacy, unsynchronized implementation.
	withGlobalFontCache(t, draw2d.NewSyncFolderFontCache("resource/font"))
	reference := renderPoster(t).(*image.RGBA)

	withGlobalFontCache(t, draw2d.NewDefaultSyncFontCache("resource/font"))
	got := renderPoster(t).(*image.RGBA)

	if ok, at := pixelsEqual(reference, got); !ok {
		t.Fatalf("pixel mismatch at offset %d: reference=%d new=%d",
			at, reference.Pix[at], got.Pix[at])
	}
}

// TestConcurrentPostersFixedFont renders the same scene from many goroutines
// sharing the global bounded cache and requires pixel-identical output.
func TestConcurrentPostersFixedFont(t *testing.T) {
	cache := draw2d.NewDefaultSyncFontCache("resource/font")
	withGlobalFontCache(t, cache)

	const n = 32
	var wg sync.WaitGroup
	images := make([]*image.RGBA, n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			defer func() {
				if r := recover(); r != nil {
					errs[idx] = errors.New("panic during concurrent render")
				}
			}()
			images[idx] = renderPoster(t).(*image.RGBA)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
	reference := images[0]
	for i := 1; i < n; i++ {
		if ok, at := pixelsEqual(reference, images[i]); !ok {
			t.Fatalf("goroutine %d differs at offset %d", i, at)
		}
	}

	stats := cache.Stats()
	// Only the handful of fonts used by the poster are ever parsed.
	if cache.Len() == 0 {
		t.Fatal("font cache is empty after rendering posters")
	}
	if stats.Loads == 0 || stats.Hits == 0 {
		t.Fatalf("expected loads and hits, got %+v", stats)
	}
	t.Logf("concurrent posters: fonts cached=%d loads=%d hits=%d dedups=%d",
		cache.Len(), stats.Loads, stats.Hits, stats.LoadDedups)
}

// flakyLoader fails the first failureBudget requests per font, then succeeds.
type flakyLoader struct {
	mu             sync.Mutex
	remainingFails map[string]int
	font           *truetype.Font
	parses         map[string]int
}

func newFlakyLoader(font *truetype.Font, fails int) *flakyLoader {
	return &flakyLoader{
		remainingFails: map[string]int{"luxi": fails},
		font:           font,
		parses:         map[string]int{},
	}
}

func (l *flakyLoader) LoadFont(fd draw2d.FontData) (*truetype.Font, error) {
	l.mu.Lock()
	l.parses[fd.Name]++
	if l.remainingFails[fd.Name] > 0 {
		l.remainingFails[fd.Name]--
		l.mu.Unlock()
		return nil, errors.New("injected transient I/O failure")
	}
	l.mu.Unlock()
	return l.font, nil
}

// TestFaultInjectionTransientFailures drives concurrent loads against a loader
// that fails repeatedly: every caller must eventually get the font, failed
// results must never be cached, and the success must parse exactly once more.
func TestFaultInjectionTransientFailures(t *testing.T) {
	font := mustParseTTF(t, "resource/font/luxisr.ttf")
	loader := newFlakyLoader(font, 5)
	cache, err := draw2d.NewSyncFontCache(loader, draw2d.FontCacheConfig{Capacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	withGlobalFontCache(t, cache)

	fd := draw2dbaseDefaultFontData()
	const n = 24
	var wg sync.WaitGroup
	ok := make(chan struct{}, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for attempt := 0; attempt < 50; attempt++ {
				if font, err := cache.Load(fd); err == nil && font != nil {
					ok <- struct{}{}
					return
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(ok)
	successes := 0
	for range ok {
		successes++
	}
	if successes != n {
		t.Fatalf("only %d/%d goroutines eventually loaded the font", successes, n)
	}
	loader.mu.Lock()
	parses := loader.parses[fd.Name]
	loader.mu.Unlock()
	// Five failed waves plus at least one successful wave.
	if parses < 6 {
		t.Fatalf("loader ran %d times, expected >= 6", parses)
	}
	// Each failed wave is one loader execution shared by its waiters.
	if s := cache.Stats(); s.LoadErrors < 5 {
		t.Fatalf("LoadErrors = %d, want >= 5", s.LoadErrors)
	}
	// After recovery, no more parses: the successful font is cached.
	before := loader.parses[fd.Name]
	for i := 0; i < n; i++ {
		if _, err := cache.Load(fd); err != nil {
			t.Fatal(err)
		}
	}
	if loader.parses[fd.Name] != before {
		t.Fatal("recovered font was parsed again despite being cached")
	}
}

// TestFaultInjectionCancelledLoadRetries verifies a cancelled request neither
// poisons the cache nor blocks later successful loads. The loader blocks on a
// gate so the cancellation happens while the single-flight load is genuinely
// in flight.
func TestFaultInjectionCancelledLoadRetries(t *testing.T) {
	loader := newCountingLoader()
	fd := draw2dbaseDefaultFontData()
	loader.addFont(fd, mustParseTTF(t, "resource/font/luxisr.ttf"))
	gate := loader.gateFont(fd)

	cache, err := draw2d.NewSyncFontCache(loader, draw2d.FontCacheConfig{Capacity: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	// Start the leader and let it enter the blocked loader.
	leaderErr := make(chan error, 1)
	go func() {
		_, e := cache.Load(fd)
		leaderErr <- e
	}()
	time.Sleep(30 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.LoadContext(ctx, fd); err == nil {
		t.Fatal("cancelled load returned nil error")
	}
	if cache.Len() != 0 {
		t.Fatal("cancelled load populated the cache")
	}

	// Unblock the leader; its result gets cached and later loads succeed.
	close(gate)
	if err := <-leaderErr; err != nil {
		t.Fatalf("leader load failed: %v", err)
	}
	if cache.Len() != 1 {
		t.Fatalf("Len = %d, want 1 after leader completed", cache.Len())
	}
	if _, err := cache.Load(fd); err != nil {
		t.Fatalf("load after cancellation failed: %v", err)
	}
	if got := loader.callCount(fd); got != 1 {
		t.Fatalf("loader ran %d times, want 1", got)
	}
}

// TestSVGGlyphPathsStable renders the same text through two SVG graphic
// contexts sharing one glyph cache and requires identical glyph path
// descriptions: the svg backend's glyph geometry must not change.
func TestSVGGlyphPathsStable(t *testing.T) {
	cache := draw2d.NewDefaultSyncFontCache("resource/font")
	withGlobalFontCache(t, cache)

	const text = "Hello draw2d 0123456789"
	render := func() string {
		svg := draw2dsvg.NewSvg()
		gc := draw2dsvg.NewGraphicContext(svg)
		gc.SetFontSize(24)
		gc.FillStringAt(text, 10, 40)
		out, err := xml.MarshalIndent(svg, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}

	first := render()
	second := render()
	if first != second {
		t.Fatal("SVG output for identical text differs between renders")
	}

	// Concurrent renders sharing the global caches must stay identical.
	const n = 16
	var wg sync.WaitGroup
	outs := make([]string, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			outs[idx] = render()
		}(i)
	}
	close(start)
	wg.Wait()
	for i := 1; i < n; i++ {
		if outs[i] != first {
			t.Fatalf("concurrent SVG render %d differs from the reference", i)
		}
	}
}
