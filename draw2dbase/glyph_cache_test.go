package draw2dbase_test

import (
	"context"
	"image"
	"sync"
	"testing"
	"time"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/draw2dbase"
	"github.com/llgcode/draw2d/draw2dimg"
)

// countingGC wraps a real image GraphicContext and counts how many times a
// glyph path was rendered through CreateStringPath. It still implements the
// full draw2d.GraphicContext interface via embedding.
type countingGC struct {
	draw2d.GraphicContext
	mu       sync.Mutex
	renders  map[rune]int
	gate     map[rune]chan struct{}
	blocking map[rune]bool
}

func newCountingGC() *countingGC {
	dest := image.NewRGBA(image.Rect(0, 0, 512, 256))
	gc := draw2dimg.NewGraphicContext(dest)
	gc.SetFontSize(18)
	return &countingGC{
		GraphicContext: gc,
		renders:        make(map[rune]int),
		gate:           make(map[rune]chan struct{}),
		blocking:       make(map[rune]bool),
	}
}

func (c *countingGC) gateRune(r rune) chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan struct{})
	c.gate[r] = ch
	c.blocking[r] = true
	return ch
}

func (c *countingGC) renderCount(r rune) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.renders[r]
}

// CreateStringPath is the method renderGlyph uses to build the cached path.
func (c *countingGC) CreateStringPath(s string, x, y float64) float64 {
	r := []rune(s)[0]
	c.mu.Lock()
	c.renders[r]++
	gate := c.gate[r]
	blocking := c.blocking[r]
	c.mu.Unlock()
	if blocking && gate != nil {
		<-gate
	}
	return c.GraphicContext.CreateStringPath(s, x, y)
}

const testFontName = "luxi:0:0:    18.00"

func TestSyncGlyphCacheConcurrentFetchDedup(t *testing.T) {
	cache := draw2dbase.NewSyncGlyphCache(draw2dbase.GlyphCacheConfig{Capacity: 32})
	defer cache.Close()

	const n = 64
	const chr = 'A'
	gcs := make([]*countingGC, 4)
	for i := range gcs {
		gcs[i] = newCountingGC()
	}
	// Gate the first render on every GC so that, whichever goroutine becomes
	// the single-flight leader, the other goroutines park before a result can
	// be cached.
	gate := make(chan struct{})
	for _, gc := range gcs {
		gc.mu.Lock()
		gc.gate[chr] = gate
		gc.blocking[chr] = true
		gc.mu.Unlock()
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	glyphs := make([]*draw2dbase.Glyph, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			glyphs[idx] = cache.Fetch(gcs[idx%len(gcs)], testFontName, chr)
		}(i)
	}
	close(start)
	// Wait until exactly one render has started; every other goroutine is now
	// blocked on the single-flight group.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		started := 0
		for _, gc := range gcs {
			started += gc.renderCount(chr)
		}
		if started == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	totalStarted := 0
	for _, gc := range gcs {
		totalStarted += gc.renderCount(chr)
	}
	if totalStarted != 1 {
		t.Fatalf("expected exactly one render to start, got %d", totalStarted)
	}
	close(gate)
	wg.Wait()

	totalRenders := 0
	for _, gc := range gcs {
		totalRenders += gc.renderCount(chr)
	}
	if totalRenders != 1 {
		t.Fatalf("glyph rendered %d times across GCs, want exactly 1", totalRenders)
	}
	for i, g := range glyphs {
		if g == nil || g.Path == nil {
			t.Fatalf("goroutine %d got nil glyph", i)
		}
		if g.Path.IsEmpty() {
			t.Fatalf("goroutine %d got an empty path", i)
		}
	}
	if cache.Len() != 1 {
		t.Fatalf("Len = %d, want 1", cache.Len())
	}
	s := cache.Stats()
	// Exactly one render; followers either hit the already filled cache or
	// joined the in-flight load (single-flight dedup).
	if s.Loads != 1 {
		t.Fatalf("Loads = %d, want 1", s.Loads)
	}
	if s.Hits+s.LoadDedups != uint64(n-1) {
		t.Fatalf("stats = %+v, want hits+dedups = %d", s, n-1)
	}
}

func TestSyncGlyphCacheReturnsIndependentCopies(t *testing.T) {
	cache := draw2dbase.NewSyncGlyphCache(draw2dbase.GlyphCacheConfig{Capacity: 8})
	defer cache.Close()
	gc := newCountingGC()

	g1 := cache.Fetch(gc, testFontName, 'B')
	g2 := cache.Fetch(gc, testFontName, 'B')
	if g1 == g2 || g1.Path == g2.Path {
		t.Fatal("Fetch returned shared Glyph/Path pointers")
	}
	// Mutating one returned path must not affect later fetches.
	originalLen := len(g1.Path.Components)
	g1.Path.Clear()
	g3 := cache.Fetch(gc, testFontName, 'B')
	if len(g3.Path.Components) != originalLen || g3.Path.IsEmpty() {
		t.Fatal("mutating a returned glyph corrupted the cached glyph path")
	}
}

func TestSyncGlyphCacheLRUEvictionAndRerender(t *testing.T) {
	cache := draw2dbase.NewSyncGlyphCache(draw2dbase.GlyphCacheConfig{Capacity: 2})
	defer cache.Close()
	gc := newCountingGC()

	cache.Fetch(gc, testFontName, 'a')
	cache.Fetch(gc, testFontName, 'b')
	cache.Fetch(gc, testFontName, 'c') // evicts 'a'
	if cache.Len() != 2 {
		t.Fatalf("Len = %d, want 2", cache.Len())
	}
	if s := cache.Stats(); s.Evictions != 1 {
		t.Fatalf("Evictions = %d, want 1", s.Evictions)
	}
	cache.Fetch(gc, testFontName, 'a') // re-rendered on demand
	if gc.renderCount('a') != 2 {
		t.Fatalf("'a' rendered %d times, want 2 after eviction", gc.renderCount('a'))
	}
	if gc.renderCount('b') != 1 || gc.renderCount('c') != 1 {
		t.Fatalf("cached glyphs re-rendered: b=%d c=%d", gc.renderCount('b'), gc.renderCount('c'))
	}
}

func TestSyncGlyphCacheTTLExpiry(t *testing.T) {
	cache := draw2dbase.NewSyncGlyphCache(draw2dbase.GlyphCacheConfig{
		Capacity: 8,
		TTL:      25 * time.Millisecond,
	})
	defer cache.Close()
	gc := newCountingGC()

	cache.Fetch(gc, testFontName, 'T')
	time.Sleep(55 * time.Millisecond)
	cache.Fetch(gc, testFontName, 'T')
	if gc.renderCount('T') != 2 {
		t.Fatalf("'T' rendered %d times, want 2 after TTL expiry", gc.renderCount('T'))
	}
	if s := cache.Stats(); s.Expirations == 0 {
		t.Fatalf("stats = %+v, want expiration recorded", s)
	}
}

func TestSyncGlyphCacheCancellation(t *testing.T) {
	cache := draw2dbase.NewSyncGlyphCache(draw2dbase.GlyphCacheConfig{Capacity: 8})
	defer cache.Close()
	gc := newCountingGC()
	gate := gc.gateRune('X')

	leaderDone := make(chan struct{})
	go func() {
		_ = cache.Fetch(gc, testFontName, 'X')
		close(leaderDone)
	}()
	time.Sleep(30 * time.Millisecond) // let the leader enter the blocked render

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g, err := cache.FetchContext(ctx, gc, testFontName, 'X')
	if err == nil {
		t.Fatal("cancelled FetchContext returned nil error")
	}
	if g != nil {
		t.Fatal("cancelled FetchContext returned a non-nil glyph")
	}

	close(gate)
	<-leaderDone
	// Subsequent fetch succeeds and never started a second concurrent render.
	g = cache.Fetch(gc, testFontName, 'X')
	if g == nil || g.Path.IsEmpty() {
		t.Fatal("fetch after leader completion failed")
	}
	if gc.renderCount('X') != 1 {
		t.Fatalf("'X' rendered %d times, want 1", gc.renderCount('X'))
	}
}

func TestSyncGlyphCacheHighConcurrencyBounded(t *testing.T) {
	cache := draw2dbase.NewSyncGlyphCache(draw2dbase.GlyphCacheConfig{Capacity: 16})
	defer cache.Close()

	// Each goroutine owns its graphic context, mirroring the real backends:
	// a GC (and its freetype GlyphBuf) is single-goroutine state, while the
	// glyph cache is the shared, concurrency-safe component.
	const workers = 32
	const runesN = 48

	var wg sync.WaitGroup
	for g := 0; g < workers; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			gc := newCountingGC()
			for i := 0; i < 300; i++ {
				r := rune('a' + (seed+i)%runesN)
				glyph := cache.Fetch(gc, testFontName, r)
				if glyph == nil || glyph.Path == nil {
					t.Errorf("nil glyph for %q", r)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	if cache.Len() > 16 {
		t.Fatalf("Len = %d exceeds capacity 16", cache.Len())
	}
	s := cache.Stats()
	if s.Loads == 0 || s.Evictions == 0 {
		t.Fatalf("expected loads and evictions, got %+v", s)
	}
	// Loads counts only actual renders; hits + dedups + loads account for
	// every served request.
	requests := s.Hits + s.LoadDedups + s.Loads
	if requests == 0 {
		t.Fatalf("no requests recorded: %+v", s)
	}
	t.Logf("glyph high concurrency: requests=%d loads=%d hits=%d dedups=%d evictions=%d",
		requests, s.Loads, s.Hits, s.LoadDedups, s.Evictions)
}
