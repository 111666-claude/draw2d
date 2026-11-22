// Copyright 2010 The draw2d Authors. All rights reserved.

package draw2dbase

import (
	"container/list"
	"math"
	"sync"
	"sync/atomic"

	"github.com/llgcode/draw2d"
)

// FlattenPurpose identifies how a flattened path is going to be consumed.
// It is part of the cache key so that fill and stroke flattenings never
// share an entry, even if their event streams happen to be identical today.
type FlattenPurpose uint8

const (
	// FillPurpose marks a flattening used to fill a path
	FillPurpose FlattenPurpose = iota
	// StrokePurpose marks a flattening used to stroke a path
	StrokePurpose
	// FillStrokePurpose marks a flattening used to fill and stroke a path in one pass
	FillStrokePurpose
)

const (
	// DefaultFlattenCacheCapacity is the default maximum number of entries
	// held by the shared flatten cache.
	DefaultFlattenCacheCapacity = 128
	// DefaultFlattenCacheMaxPoints is the default maximum number of float64
	// values a single cached flattened path may hold. Larger results are
	// never cached so one huge path cannot exhaust memory or evict everything.
	DefaultFlattenCacheMaxPoints = 1 << 20
)

// flattenCmd is one recorded event of a flattened path.
type flattenCmd uint8

const (
	cmdMoveTo flattenCmd = iota
	cmdLineTo
	cmdLineJoin
	cmdClose
	cmdEnd
)

// flattenedPath is an immutable record of the event stream Flatten emits
// for a path. It is shared between cache readers and must never be mutated
// after creation.
type flattenedPath struct {
	cmds   []flattenCmd
	points []float64 // x, y pairs consumed by cmdMoveTo and cmdLineTo
}

// replayTo replays the recorded event stream into flattener. It only reads
// the flattenedPath, so a single entry can be replayed concurrently by any
// number of consumers (image, SVG, GL backends, ...).
func (f *flattenedPath) replayTo(flattener Flattener) {
	j := 0
	for _, cmd := range f.cmds {
		switch cmd {
		case cmdMoveTo:
			flattener.MoveTo(f.points[j], f.points[j+1])
			j += 2
		case cmdLineTo:
			flattener.LineTo(f.points[j], f.points[j+1])
			j += 2
		case cmdLineJoin:
			flattener.LineJoin()
		case cmdClose:
			flattener.Close()
		case cmdEnd:
			flattener.End()
		}
	}
}

// flattenRecorder records the event stream of a flattening pass.
type flattenRecorder struct {
	flattened flattenedPath
}

func (r *flattenRecorder) MoveTo(x, y float64) {
	r.flattened.cmds = append(r.flattened.cmds, cmdMoveTo)
	r.flattened.points = append(r.flattened.points, x, y)
}

func (r *flattenRecorder) LineTo(x, y float64) {
	r.flattened.cmds = append(r.flattened.cmds, cmdLineTo)
	r.flattened.points = append(r.flattened.points, x, y)
}

func (r *flattenRecorder) LineJoin() {
	r.flattened.cmds = append(r.flattened.cmds, cmdLineJoin)
}

func (r *flattenRecorder) Close() {
	r.flattened.cmds = append(r.flattened.cmds, cmdClose)
}

func (r *flattenRecorder) End() {
	r.flattened.cmds = append(r.flattened.cmds, cmdEnd)
}

// flattenKey distinguishes cached flattenings by path content, matrix
// scale, curve tolerance and fill/stroke purpose. The full transformation
// matrix is deliberately not part of the key: flattening happens in path
// coordinates and the matrix is applied downstream by Transformer, only
// its scale influences arc subdivision.
type flattenKey struct {
	hash      uint64
	scale     float64
	tolerance float64
	purpose   FlattenPurpose
}

// cacheEntry is an immutable cache entry. The source path content is kept
// so a hash hit can be verified before the entry is used.
type cacheEntry struct {
	key       flattenKey
	flattened flattenedPath
	comps     []draw2d.PathCmp // copy of the path components at record time
	src       []float64        // copy of the path points at record time
}

// FlattenCache caches flattened paths so that repeatedly drawn complex
// paths do not pay the Bézier subdivision cost on every frame. It is safe
// for concurrent use. Entries are immutable once published and are shared
// between readers; they are never handed out as mutable slices.
//
// The zero value is not usable; create a cache with NewFlattenCache.
// A nil *FlattenCache is valid and flattens directly without caching.
type FlattenCache struct {
	mu        sync.Mutex
	capacity  int // maximum number of entries, immutable after creation
	maxPoints int // maximum float64 values per cached entry
	evictList *list.List
	entries   map[flattenKey]*list.Element
	hits      uint64
	misses    uint64
}

// NewFlattenCache creates a cache holding at most capacity entries using
// least-recently-used eviction. A capacity of 0 disables caching.
func NewFlattenCache(capacity int) *FlattenCache {
	if capacity < 0 {
		capacity = 0
	}
	return &FlattenCache{
		capacity:  capacity,
		maxPoints: DefaultFlattenCacheMaxPoints,
		evictList: list.New(),
		entries:   make(map[flattenKey]*list.Element),
	}
}

// SetMaxEntryPoints sets the maximum number of float64 values a single
// cached flattened path may hold. Larger results are not cached.
func (c *FlattenCache) SetMaxEntryPoints(max int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.maxPoints = max
	c.mu.Unlock()
}

// Stats returns the number of cache hits, misses and current entries.
func (c *FlattenCache) Stats() (hits, misses uint64, entries int) {
	if c == nil {
		return 0, 0, 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses, len(c.entries)
}

// Reset drops all entries and statistics.
func (c *FlattenCache) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.evictList.Init()
	c.entries = make(map[flattenKey]*list.Element)
	c.hits, c.misses = 0, 0
	c.mu.Unlock()
}

// Flatten flattens path into flattener, reusing a cached flattening when
// the path content, scale, tolerance and purpose match a previous call.
// The event stream delivered to flattener is identical to the one Flatten
// would produce. A nil cache flattens directly.
//
// Deterministic policies:
//   - empty paths and paths without curves are flattened directly (the
//     flattening cost is linear and caching would only add overhead);
//   - a non-finite scale or tolerance disables caching for the call;
//   - results containing non-finite coordinates or more points than the
//     per-entry limit are never cached;
//   - the cache holds at most capacity entries, evicting the least
//     recently used one.
//
// Like every other drawing operation, Flatten must not be called on a path
// that another goroutine is mutating.
func (c *FlattenCache) Flatten(path *draw2d.Path, flattener Flattener, scale, tolerance float64, purpose FlattenPurpose) {
	if c == nil || c.capacity == 0 || !cacheablePath(path) ||
		!isFinite(scale) || !isFinite(tolerance) {
		flatten(path, flattener, scale, tolerance)
		return
	}
	hash, finite := hashPath(path)
	if !finite {
		// Paths with non-finite coordinates are flattened directly and
		// never cached, so a NaN coordinate can neither poison an entry
		// nor defeat the content verification.
		flatten(path, flattener, scale, tolerance)
		return
	}
	key := flattenKey{
		hash:      hash,
		scale:     scale,
		tolerance: tolerance,
		purpose:   purpose,
	}
	c.mu.Lock()
	if el, ok := c.entries[key]; ok {
		e := el.Value.(*cacheEntry)
		if pathMatches(e, path) {
			c.evictList.MoveToFront(el)
			c.hits++
			c.mu.Unlock()
			e.flattened.replayTo(flattener)
			return
		}
		// Hash collision with different content: replace the entry.
		c.removeElement(el)
	}
	c.misses++
	c.mu.Unlock()

	// Record the flattening outside the lock so concurrent readers are not
	// blocked behind an expensive curve subdivision.
	rec := &flattenRecorder{}
	rec.flattened.points = make([]float64, 0, 2*len(path.Points))
	flatten(path, rec, scale, tolerance)
	rec.flattened.replayTo(flattener)

	if !allFinite(rec.flattened.points) {
		return // never cache non-finite results
	}

	e := &cacheEntry{
		key:       key,
		flattened: rec.flattened,
		comps:     append([]draw2d.PathCmp(nil), path.Components...),
		src:       append([]float64(nil), path.Points...),
	}
	c.mu.Lock()
	if len(e.flattened.points) <= c.maxPoints {
		if el, ok := c.entries[key]; ok {
			// Another goroutine stored the same key meanwhile.
			c.removeElement(el)
		}
		c.entries[key] = c.evictList.PushFront(e)
		for c.evictList.Len() > c.capacity {
			c.removeElement(c.evictList.Back())
		}
	}
	c.mu.Unlock()
}

func (c *FlattenCache) removeElement(el *list.Element) {
	c.evictList.Remove(el)
	delete(c.entries, el.Value.(*cacheEntry).key)
}

// cacheablePath reports whether flattening the path does enough work
// (curve or arc subdivision) to be worth caching.
func cacheablePath(p *draw2d.Path) bool {
	for _, cmp := range p.Components {
		switch cmp {
		case draw2d.QuadCurveToCmp, draw2d.CubicCurveToCmp, draw2d.ArcToCmp:
			return true
		}
	}
	return false
}

// hashPath computes a FNV-1a hash over the path content and reports
// whether all path points are finite.
func hashPath(p *draw2d.Path) (hash uint64, finite bool) {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	h ^= uint64(len(p.Components))
	h *= prime64
	for _, cmp := range p.Components {
		h ^= uint64(cmp)
		h *= prime64
	}
	h ^= uint64(len(p.Points))
	h *= prime64
	finite = true
	for _, v := range p.Points {
		h ^= math.Float64bits(v)
		h *= prime64
		finite = finite && isFinite(v)
	}
	return h, finite
}

// pathMatches verifies that a cached entry was recorded from exactly the
// current path content, so a stale or colliding entry is never replayed.
func pathMatches(e *cacheEntry, p *draw2d.Path) bool {
	if len(e.comps) != len(p.Components) || len(e.src) != len(p.Points) {
		return false
	}
	for i, cmp := range e.comps {
		if cmp != p.Components[i] {
			return false
		}
	}
	for i, v := range e.src {
		if v != p.Points[i] {
			return false
		}
	}
	return true
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func allFinite(values []float64) bool {
	for _, v := range values {
		if !isFinite(v) {
			return false
		}
	}
	return true
}

var sharedFlattenCache atomic.Pointer[FlattenCache]

func init() {
	sharedFlattenCache.Store(NewFlattenCache(DefaultFlattenCacheCapacity))
}

// SharedFlattenCache returns the process-wide flatten cache used by
// FlattenCached and the graphic context backends.
func SharedFlattenCache() *FlattenCache {
	return sharedFlattenCache.Load()
}

// SetSharedFlattenCache replaces the shared flatten cache. Passing nil
// disables caching, which restores the historical behavior of flattening
// every path on every draw.
func SetSharedFlattenCache(c *FlattenCache) {
	sharedFlattenCache.Store(c)
}

// FlattenCached flattens path into flattener using the shared flatten
// cache and the default curve tolerance. See FlattenCache.Flatten.
func FlattenCached(path *draw2d.Path, flattener Flattener, scale float64, purpose FlattenPurpose) {
	SharedFlattenCache().Flatten(path, flattener, scale, CurveFlatteningThreshold, purpose)
}
