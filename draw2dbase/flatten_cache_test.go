package draw2dbase

import (
	"math"
	"sync"
	"testing"

	"github.com/llgcode/draw2d"
)

// event is one recorded Flattener call used to compare event streams.
type event struct {
	cmd  string
	x, y float64
}

type eventRecorder struct {
	events []event
}

func (r *eventRecorder) MoveTo(x, y float64) { r.events = append(r.events, event{"M", x, y}) }
func (r *eventRecorder) LineTo(x, y float64) { r.events = append(r.events, event{"L", x, y}) }
func (r *eventRecorder) LineJoin()           { r.events = append(r.events, event{"J", 0, 0}) }
func (r *eventRecorder) Close()              { r.events = append(r.events, event{"C", 0, 0}) }
func (r *eventRecorder) End()                { r.events = append(r.events, event{"E", 0, 0}) }

func recordDirect(path *draw2d.Path, scale, tolerance float64) []event {
	rec := &eventRecorder{}
	flatten(path, rec, scale, tolerance)
	return rec.events
}

func recordCached(c *FlattenCache, path *draw2d.Path, scale, tolerance float64, purpose FlattenPurpose) []event {
	rec := &eventRecorder{}
	c.Flatten(path, rec, scale, tolerance, purpose)
	return rec.events
}

func eventsEqual(a, b []event) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// complexTestPath builds a deterministic path mixing every curve kind,
// sub paths and closes.
func complexTestPath() *draw2d.Path {
	p := new(draw2d.Path)
	p.MoveTo(10, 10)
	for i := 0; i < 25; i++ {
		f := float64(i)
		p.CubicCurveTo(20+f*7, 5+f*3, 40+f*2, 60-f, 30+f*9, 30+f*4)
		p.QuadCurveTo(50+f, 80-f*2, 60+f*5, 20+f*6)
		p.ArcTo(100+f*3, 100, 30+f, 20+f/2, f*0.1, math.Pi*1.5)
	}
	p.Close()
	p.MoveTo(200, 200)
	p.CubicCurveTo(220, 180, 260, 260, 300, 200)
	p.LineTo(300, 240)
	p.Close()
	return p
}

func TestFlattenCacheConsistentWithFlatten(t *testing.T) {
	path := complexTestPath()
	want := recordDirect(path, 1.5, CurveFlatteningThreshold)
	if len(want) == 0 {
		t.Fatal("expected a non empty event stream")
	}

	cache := NewFlattenCache(DefaultFlattenCacheCapacity)
	for i := 0; i < 3; i++ { // first call misses, following calls hit
		got := recordCached(cache, path, 1.5, CurveFlatteningThreshold, FillPurpose)
		if !eventsEqual(want, got) {
			t.Fatalf("call %d: cached event stream differs from direct flattening", i)
		}
	}
	hits, misses, entries := cache.Stats()
	if misses != 1 || hits != 2 || entries != 1 {
		t.Fatalf("unexpected stats: hits=%d misses=%d entries=%d", hits, misses, entries)
	}
}

func TestFlattenCacheInvalidation(t *testing.T) {
	cache := NewFlattenCache(DefaultFlattenCacheCapacity)
	path := complexTestPath()

	recordCached(cache, path, 1, CurveFlatteningThreshold, FillPurpose)
	_, misses, _ := cache.Stats()
	if misses != 1 {
		t.Fatalf("expected 1 miss, got %d", misses)
	}

	// Same content, same parameters: hit.
	recordCached(cache, path, 1, CurveFlatteningThreshold, FillPurpose)
	// Different purpose: miss.
	recordCached(cache, path, 1, CurveFlatteningThreshold, StrokePurpose)
	// Different scale: miss.
	recordCached(cache, path, 2, CurveFlatteningThreshold, FillPurpose)
	// Different tolerance: miss.
	recordCached(cache, path, 1, 0.25, FillPurpose)

	hits, misses, entries := cache.Stats()
	if hits != 1 || misses != 4 || entries != 4 {
		t.Fatalf("unexpected stats: hits=%d misses=%d entries=%d", hits, misses, entries)
	}

	// Path mutation must not be served from the stale entry.
	path.CubicCurveTo(1, 2, 3, 4, 5, 6)
	got := recordCached(cache, path, 1, CurveFlatteningThreshold, FillPurpose)
	want := recordDirect(path, 1, CurveFlatteningThreshold)
	if !eventsEqual(want, got) {
		t.Fatal("mutated path was served stale cache content")
	}
	hits, misses, _ = cache.Stats()
	if hits != 1 || misses != 5 {
		t.Fatalf("mutation did not invalidate: hits=%d misses=%d", hits, misses)
	}

	// In-place content change (same length) must also invalidate.
	p2 := complexTestPath()
	recordCached(cache, p2, 1, CurveFlatteningThreshold, FillPurpose)
	p2.Points[0] += 1 // mutate a coordinate without changing lengths
	got = recordCached(cache, p2, 1, CurveFlatteningThreshold, FillPurpose)
	want = recordDirect(p2, 1, CurveFlatteningThreshold)
	if !eventsEqual(want, got) {
		t.Fatal("in-place mutated path was served stale cache content")
	}
}

func TestFlattenCacheSaveRestoreCopy(t *testing.T) {
	// A copy of a path has identical content and must hit the same entry,
	// which is what makes Save/Restore of graphic contexts cheap.
	cache := NewFlattenCache(DefaultFlattenCacheCapacity)
	path := complexTestPath()
	recordCached(cache, path, 1, CurveFlatteningThreshold, FillPurpose)

	dup := path.Copy()
	got := recordCached(cache, dup, 1, CurveFlatteningThreshold, FillPurpose)
	if want := recordDirect(path, 1, CurveFlatteningThreshold); !eventsEqual(want, got) {
		t.Fatal("copied path produced a different event stream")
	}
	hits, _, entries := cache.Stats()
	if hits != 1 || entries != 1 {
		t.Fatalf("copy did not share the entry: hits=%d entries=%d", hits, entries)
	}

	// Mutating the copy afterwards must not affect the original entry.
	dup.LineTo(500, 500)
	recordCached(cache, dup, 1, CurveFlatteningThreshold, FillPurpose)
	got = recordCached(cache, path, 1, CurveFlatteningThreshold, FillPurpose)
	if want := recordDirect(path, 1, CurveFlatteningThreshold); !eventsEqual(want, got) {
		t.Fatal("mutating a copy corrupted the original cache entry")
	}
}

func TestFlattenCacheEmptyAndLineOnly(t *testing.T) {
	cache := NewFlattenCache(DefaultFlattenCacheCapacity)

	empty := new(draw2d.Path)
	got := recordCached(cache, empty, 1, CurveFlatteningThreshold, FillPurpose)
	want := recordDirect(empty, 1, CurveFlatteningThreshold)
	if !eventsEqual(want, got) {
		t.Fatal("empty path: cached stream differs")
	}

	lines := new(draw2d.Path)
	lines.MoveTo(0, 0)
	lines.LineTo(10, 10)
	lines.LineTo(20, 0)
	lines.Close()
	got = recordCached(cache, lines, 1, CurveFlatteningThreshold, FillPurpose)
	want = recordDirect(lines, 1, CurveFlatteningThreshold)
	if !eventsEqual(want, got) {
		t.Fatal("line only path: cached stream differs")
	}

	if _, _, entries := cache.Stats(); entries != 0 {
		t.Fatalf("empty and line only paths must not be cached, got %d entries", entries)
	}
}

func TestFlattenCacheNonFinite(t *testing.T) {
	cache := NewFlattenCache(DefaultFlattenCacheCapacity)
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		path := new(draw2d.Path)
		path.MoveTo(0, 0)
		path.CubicCurveTo(bad, 10, 20, 30, 40, 50)
		got := recordCached(cache, path, 1, CurveFlatteningThreshold, FillPurpose)
		want := recordDirect(path, 1, CurveFlatteningThreshold)
		if len(got) != len(want) {
			t.Fatalf("non finite coordinate: stream length differs (%d != %d)", len(got), len(want))
		}
		for i := range got {
			a, b := got[i], want[i]
			if a.cmd != b.cmd || !sameFloat(a.x, b.x) || !sameFloat(a.y, b.y) {
				t.Fatalf("non finite coordinate: event %d differs: %v != %v", i, a, b)
			}
		}
	}
	if _, _, entries := cache.Stats(); entries != 0 {
		t.Fatalf("non finite results must not be cached, got %d entries", entries)
	}

	// A non finite scale flattens directly without caching and without panic.
	path := complexTestPath()
	_ = recordCached(cache, path, math.NaN(), CurveFlatteningThreshold, FillPurpose)
	_ = recordCached(cache, path, math.Inf(1), CurveFlatteningThreshold, FillPurpose)
	if _, _, entries := cache.Stats(); entries != 0 {
		t.Fatalf("non finite scale must not be cached, got %d entries", entries)
	}
}

func sameFloat(a, b float64) bool {
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	return a == b
}

func TestFlattenCacheCapacityAndLRU(t *testing.T) {
	const capacity = 4
	cache := NewFlattenCache(capacity)
	paths := make([]*draw2d.Path, 2*capacity)
	for i := range paths {
		p := new(draw2d.Path)
		p.MoveTo(float64(i), 0)
		p.CubicCurveTo(float64(i)+1, 2, 3, 4, 5, 6)
		paths[i] = p
		recordCached(cache, p, 1, CurveFlatteningThreshold, FillPurpose)
	}
	_, misses, entries := cache.Stats()
	if entries != capacity {
		t.Fatalf("cache grew beyond capacity: %d entries", entries)
	}
	if misses != uint64(len(paths)) {
		t.Fatalf("expected %d misses, got %d", len(paths), misses)
	}

	// The oldest entries were evicted: replaying the first path misses,
	// replaying the most recent one hits.
	recordCached(cache, paths[0], 1, CurveFlatteningThreshold, FillPurpose)
	recordCached(cache, paths[len(paths)-1], 1, CurveFlatteningThreshold, FillPurpose)
	hits, misses, entries := cache.Stats()
	if hits != 1 || misses != uint64(len(paths))+1 || entries != capacity {
		t.Fatalf("unexpected LRU behavior: hits=%d misses=%d entries=%d", hits, misses, entries)
	}
}

func TestFlattenCacheOversizedEntry(t *testing.T) {
	cache := NewFlattenCache(DefaultFlattenCacheCapacity)
	cache.SetMaxEntryPoints(16)
	path := complexTestPath() // flattens to far more than 16 floats
	got := recordCached(cache, path, 1, CurveFlatteningThreshold, FillPurpose)
	want := recordDirect(path, 1, CurveFlatteningThreshold)
	if !eventsEqual(want, got) {
		t.Fatal("oversized path: cached stream differs")
	}
	if _, _, entries := cache.Stats(); entries != 0 {
		t.Fatalf("oversized entry must not be cached, got %d entries", entries)
	}
}

func TestFlattenCacheNilAndZeroCapacity(t *testing.T) {
	path := complexTestPath()
	want := recordDirect(path, 1, CurveFlatteningThreshold)

	var nilCache *FlattenCache
	if got := recordCached(nilCache, path, 1, CurveFlatteningThreshold, FillPurpose); !eventsEqual(want, got) {
		t.Fatal("nil cache must flatten directly")
	}

	zero := NewFlattenCache(0)
	if got := recordCached(zero, path, 1, CurveFlatteningThreshold, FillPurpose); !eventsEqual(want, got) {
		t.Fatal("zero capacity cache must flatten directly")
	}
	if _, _, entries := zero.Stats(); entries != 0 {
		t.Fatalf("zero capacity cache stored %d entries", entries)
	}
}

func TestFlattenCacheImmutableSharedEntry(t *testing.T) {
	// Two consumers (as the image and SVG backends would be) must receive
	// the same immutable entry without interfering with each other.
	cache := NewFlattenCache(DefaultFlattenCacheCapacity)
	path := complexTestPath()

	first := recordCached(cache, path, 1, CurveFlatteningThreshold, FillPurpose)
	second := recordCached(cache, path, 1, CurveFlatteningThreshold, FillPurpose)
	if !eventsEqual(first, second) {
		t.Fatal("two consumers received different event streams")
	}
	// Replaying again after both consumers ran must still be identical.
	third := recordCached(cache, path, 1, CurveFlatteningThreshold, FillPurpose)
	if !eventsEqual(first, third) {
		t.Fatal("shared entry was mutated by a consumer")
	}
}

func TestFlattenCacheConcurrent(t *testing.T) {
	cache := NewFlattenCache(16)
	paths := []*draw2d.Path{complexTestPath(), complexTestPath(), complexTestPath()}
	paths[1].LineTo(400, 400)
	paths[2].CubicCurveTo(1, 1, 2, 2, 3, 3)
	purposes := []FlattenPurpose{FillPurpose, StrokePurpose, FillStrokePurpose}
	scales := []float64{1, 1.5, 2}

	want := make([][]event, len(paths))
	for i, p := range paths {
		want[i] = recordDirect(p, scales[i], CurveFlatteningThreshold)
	}

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				idx := (g + i) % len(paths)
				got := recordCached(cache, paths[idx], scales[idx], CurveFlatteningThreshold, purposes[idx])
				if !eventsEqual(want[idx], got) {
					t.Errorf("goroutine %d: corrupted event stream for path %d", g, idx)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestFlattenCachedSharedCache(t *testing.T) {
	old := SharedFlattenCache()
	defer SetSharedFlattenCache(old)

	cache := NewFlattenCache(DefaultFlattenCacheCapacity)
	SetSharedFlattenCache(cache)

	path := complexTestPath()
	want := recordDirect(path, 1, CurveFlatteningThreshold)
	for i := 0; i < 2; i++ {
		rec := &eventRecorder{}
		FlattenCached(path, rec, 1, FillPurpose)
		if !eventsEqual(want, rec.events) {
			t.Fatalf("call %d: FlattenCached stream differs", i)
		}
	}
	hits, misses, _ := cache.Stats()
	if hits != 1 || misses != 1 {
		t.Fatalf("unexpected shared cache stats: hits=%d misses=%d", hits, misses)
	}

	// Disabling the cache restores direct flattening.
	SetSharedFlattenCache(nil)
	rec := &eventRecorder{}
	FlattenCached(path, rec, 1, FillPurpose)
	if !eventsEqual(want, rec.events) {
		t.Fatal("disabled cache must flatten directly")
	}
}

// benchmarkPath builds a complex path with n curve segments.
func benchmarkPath(n int) *draw2d.Path {
	p := new(draw2d.Path)
	p.MoveTo(10, 10)
	x, y := 10.0, 10.0
	for i := 0; i < n; i++ {
		f := float64(i%97) + 1
		p.CubicCurveTo(x+f*3, y-f*2, x+f, y+f*4, x+f*2, y+f)
		x, y = x+f*2, y+f
		p.QuadCurveTo(x+f, y+f*2, x-f, y+f)
		x, y = x-f, y+f
		p.ArcTo(x, y, f, f/2, 0.1*f, math.Pi)
	}
	p.Close()
	return p
}

// countingFlattener is a minimal Flattener that only counts events, so the
// benchmark measures flattening rather than rasterization.
type countingFlattener struct{ n int }

func (c *countingFlattener) MoveTo(x, y float64) { c.n++ }
func (c *countingFlattener) LineTo(x, y float64) { c.n++ }
func (c *countingFlattener) LineJoin()           { c.n++ }
func (c *countingFlattener) Close()              { c.n++ }
func (c *countingFlattener) End()                { c.n++ }

func BenchmarkFlattenDirect(b *testing.B) {
	path := benchmarkPath(100)
	flattener := &countingFlattener{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Flatten(path, flattener, 1)
	}
}

func BenchmarkFlattenCached(b *testing.B) {
	path := benchmarkPath(100)
	cache := NewFlattenCache(DefaultFlattenCacheCapacity)
	flattener := &countingFlattener{}
	cache.Flatten(path, flattener, 1, CurveFlatteningThreshold, FillPurpose) // warm up
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Flatten(path, flattener, 1, CurveFlatteningThreshold, FillPurpose)
	}
}
