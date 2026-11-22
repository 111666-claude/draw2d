package draw2dbase

import (
	"context"
	"strconv"
	"time"

	"github.com/llgcode/draw2d"
	"github.com/llgcode/draw2d/cache"
)

// DefaultGlyphCacheCapacity bounds how many rendered glyphs are retained.
// Glyphs are keyed by font name (which includes the font size) and rune, so
// a typical application keeps a few hundred entries.
const DefaultGlyphCacheCapacity = 4096

// glyphCacheKey identifies a glyph: font name (already encoding family, style
// and font size) plus the rune.
type glyphCacheKey struct {
	fontName string
	chr      rune
}

func (k glyphCacheKey) string() string {
	// A rune cannot contain a NUL byte, so '\x00' is an unambiguous
	// separator and avoids fmt allocation overhead.
	return k.fontName + "\x00" + strconv.FormatInt(int64(k.chr), 16)
}

// GlyphCacheConfig configures a [SyncGlyphCache].
type GlyphCacheConfig struct {
	// Capacity bounds the number of rendered glyphs kept in memory. Zero or
	// negative means unlimited.
	Capacity int
	// TTL, when positive, expires rendered glyphs after the given duration.
	TTL time.Duration
}

// SyncGlyphCache is a concurrency-safe, bounded GlyphCache with single-flight
// rendering, LRU and optional TTL eviction.
//
// Concurrent Fetch calls for the same font name and rune render the glyph at
// most once. Every Fetch returns an independent copy: callers never receive a
// shared mutable Path, so concurrent fills and strokes cannot corrupt each
// other's geometry. Evicted glyphs are rendered again on demand.
//
// SyncGlyphCache implements the GlyphCache interface and can be installed on
// any graphic context with SetGlyphCache where available.
type SyncGlyphCache struct {
	lru   *cache.LRU
	group cache.Group
}

// NewSyncGlyphCache builds a bounded concurrent glyph cache.
func NewSyncGlyphCache(cfg GlyphCacheConfig) *SyncGlyphCache {
	return &SyncGlyphCache{
		lru: cache.NewLRU(cache.LRUConfig{
			Capacity: cfg.Capacity,
			TTL:      cfg.TTL,
		}),
	}
}

// NewDefaultSyncGlyphCache builds a glyph cache with the default capacity and
// no TTL; this is the cache used by the standard graphic context
// constructors.
func NewDefaultSyncGlyphCache() *SyncGlyphCache {
	return NewSyncGlyphCache(GlyphCacheConfig{Capacity: DefaultGlyphCacheCapacity})
}

// Fetch implements GlyphCache: it returns a copy of a cached glyph or renders
// it once, sharing the render with concurrent callers for the same key.
func (g *SyncGlyphCache) Fetch(gc draw2d.GraphicContext, fontName string, chr rune) *Glyph {
	glyph, err := g.FetchContext(context.Background(), gc, fontName, chr)
	if err != nil {
		// A background-context call can only fail if rendering panics, which
		// renderGlyph does not; keep the legacy error-free signature.
		return glyph
	}
	return glyph
}

// FetchContext is Fetch with cancellation. A cancelled context returns the
// context error and a nil glyph without cancelling a render already in
// flight for other callers. On success the returned Glyph is an independent
// copy and is safe to mutate.
func (g *SyncGlyphCache) FetchContext(ctx context.Context, gc draw2d.GraphicContext, fontName string, chr rune) (*Glyph, error) {
	key := glyphCacheKey{fontName: fontName, chr: chr}
	keyStr := key.string()
	if v, ok := g.lru.Get(keyStr); ok {
		return v.(*Glyph).Copy(), nil
	}
	v, leader, err := g.group.Do(ctx, keyStr, func() (interface{}, error) {
		// Re-check: another goroutine may have rendered this glyph while we
		// were waiting for the single-flight slot.
		if cached, ok := g.lru.Get(keyStr); ok {
			return cached, nil
		}
		// renderGlyph saves/restores the gc and leaves the cached glyph's
		// path untouched by later drawing.
		glyph := renderGlyph(gc, fontName, chr)
		g.lru.Add(keyStr, glyph)
		return glyph, nil
	})
	if err != nil {
		// A follower whose own context ended while waiting did not join a
		// completed load, so it is not counted as a dedup.
		if !leader && ctx.Err() == nil {
			g.lru.AddLoadDedup()
		}
		return nil, err
	}
	if leader {
		g.lru.AddLoad()
	} else {
		g.lru.AddLoadDedup()
	}
	// Copy on the way out: the cached value must never escape as mutable.
	return v.(*Glyph).Copy(), nil
}

// Len returns the number of cached glyphs.
func (g *SyncGlyphCache) Len() int {
	return g.lru.Len()
}

// Stats returns a snapshot of the cache counters.
func (g *SyncGlyphCache) Stats() cache.Stats {
	return g.lru.Snapshot()
}

// Close stops background TTL eviction and clears the cache.
func (g *SyncGlyphCache) Close() {
	g.lru.Close()
}
