// Package cache provides concurrency-safe, bounded, LRU/TTL evicting caches
// used by draw2d for fonts and glyphs.
//
// The building blocks in this package are generic and backend independent:
//
//   - [Group] deduplicates concurrent loads of the same key so that only one
//     goroutine performs the expensive work (disk read, font parsing, glyph
//     rendering) while the others wait for its result.
//   - [LRU] is a thread-safe, capacity-bounded map with LRU eviction and
//     optional per-entry TTL.
//
// The font and glyph specific caches live in the draw2d and draw2dbase
// packages and are built on top of these primitives.
package cache

import (
	"context"
	"sync"
	"time"
)

// call represents an in-flight or completed Do call for a given key.
type call struct {
	done chan struct{}
	val  interface{}
	err  error
	// panicked holds a value recovered from a panicking loader. It is
	// re-panicked in every waiter so a broken loader cannot deadlock callers.
	panicked interface{}
}

// Group deduplicates concurrent calls for the same key.
//
// Only one execution of the loader function is in flight for a given key at
// any time; concurrent callers receive the same result and error. A zero
// Group is ready to use and may be reused concurrently.
//
// The loader always runs with the context of the goroutine that started it.
// Waiters whose context is cancelled while the load is in flight return
// context.Cause(ctx) without affecting the shared load: its result is still
// stored by the caller once the loader completes, so the first request always
// pays the cost exactly once.
type Group struct {
	mu sync.Mutex
	m  map[string]*call
}

// Do executes fn once for the given key, sharing its result with concurrent
// callers for the same key. The boolean result reports whether this caller
// executed fn (true, the "leader") or joined an in-flight call (false, a
// "follower"). The provided context only affects this caller: cancellation
// unblocks this caller but never cancels the underlying load.
func (g *Group) Do(ctx context.Context, key string, fn func() (interface{}, error)) (interface{}, bool, error) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*call)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		v, err := g.wait(ctx, c)
		return v, false, err
	}
	c := &call{done: make(chan struct{})}
	g.m[key] = c
	g.mu.Unlock()

	g.run(key, c, fn)
	v, err := g.wait(ctx, c)
	return v, true, err
}

// run executes the loader, captures panics and removes the in-flight entry.
func (g *Group) run(key string, c *call, fn func() (interface{}, error)) {
	defer func() {
		if r := recover(); r != nil {
			c.panicked = r
		}
		close(c.done)
		g.mu.Lock()
		// Only delete the entry we own; a new call for the same key may have
		// been started by another goroutine after close but before this lock.
		if g.m[key] == c {
			delete(g.m, key)
		}
		g.mu.Unlock()
	}()
	c.val, c.err = fn()
}

// wait blocks until the shared call completes or ctx is done.
func (g *Group) wait(ctx context.Context, c *call) (interface{}, error) {
	select {
	case <-c.done:
		if c.panicked != nil {
			panic(c.panicked)
		}
		return c.val, c.err
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

// InFlight reports whether a call for key is currently executing. It is mainly
// useful in tests that need to wait for a completed wave to clean up.
func (g *Group) InFlight(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.m[key]
	return ok
}

// WaitUntilIdle blocks until no call for key is in flight or the deadline
// expires, reporting whether the group became idle.
func (g *Group) WaitUntilIdle(key string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !g.InFlight(key) {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return !g.InFlight(key)
}
