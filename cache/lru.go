package cache

import (
	"container/list"
	"sync"
	"time"
)

// Stats holds cache operation counters. Counters are monotonic for the
// lifetime of the cache; take a Snapshot before and after an operation to
// measure its effect.
type Stats struct {
	// Hits counts successful cache lookups.
	Hits uint64
	// Misses counts lookups absent from the cache, expired, or joined into an
	// in-flight single-flight load.
	Misses uint64
	// Evictions counts entries removed to enforce capacity or TTL.
	Evictions uint64
	// Expirations counts entries removed because their TTL elapsed.
	Expirations uint64
	// Loads counts loader executions actually performed. A single-flight
	// wave of N concurrent misses therefore contributes exactly 1 Load.
	Loads uint64
	// LoadErrors counts loader executions that returned an error or panicked.
	LoadErrors uint64
	// LoadDedups counts requests that joined an already in-flight load.
	LoadDedups uint64
}

// lruEntry is the value stored in the LRU list.
type lruEntry struct {
	key   string
	value interface{}
	// expiresAt is zero when the entry has no per-entry TTL.
	expiresAt time.Time
}

// LRU is a thread-safe, capacity-bounded cache with LRU eviction and optional
// per-entry TTL. A zero LRU is not usable; create one with NewLRU.
type LRU struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	ll       *list.List
	stats    Stats
	stopJan  chan struct{}
	janDone  chan struct{}
	closed   bool
	now      func() time.Time
}

// LRUConfig configures an [LRU] cache.
type LRUConfig struct {
	// Capacity is the maximum number of entries kept in the cache.
	// A zero or negative value means unlimited.
	Capacity int
	// TTL is the time after which an entry is considered expired. Zero means
	// entries never expire. Expired entries are removed lazily on lookup and,
	// when TTL is set, periodically by a background janitor goroutine.
	TTL time.Duration
}

// NewLRU creates a bounded thread-safe LRU cache. The returned cache starts a
// janitor goroutine only when TTL is positive; call [LRU.Close] to stop it.
func NewLRU(cfg LRUConfig) *LRU {
	c := &LRU{
		capacity: cfg.Capacity,
		ttl:      cfg.TTL,
		items:    make(map[string]*list.Element),
		ll:       list.New(),
		stopJan:  make(chan struct{}),
		now:      time.Now,
	}
	if cfg.TTL > 0 {
		// Sweep twice per TTL so memory is reclaimed even without lookups.
		interval := cfg.TTL / 2
		if interval <= 0 {
			interval = cfg.TTL
		}
		c.janDone = make(chan struct{})
		go c.janitor(interval)
	}
	return c
}

// janitor periodically evicts expired entries.
func (c *LRU) janitor(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer close(c.janDone)
	for {
		select {
		case <-ticker.C:
			c.deleteExpired()
		case <-c.stopJan:
			return
		}
	}
}

// Get returns the value for key and whether it was present and unexpired.
// A hit moves the entry to the front of the LRU list. Expired entries are
// evicted lazily and reported as a miss.
func (c *LRU) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		c.stats.Misses++
		return nil, false
	}
	entry := el.Value.(*lruEntry)
	if !entry.expiresAt.IsZero() && !c.now().Before(entry.expiresAt) {
		c.removeElement(el, true)
		c.stats.Misses++
		return nil, false
	}
	c.ll.MoveToFront(el)
	c.stats.Hits++
	return entry.value, true
}

// Add stores or replaces a value for key, marking it as most recently used.
// When TTL is configured the entry gets a fresh expiry. Evictions performed
// to enforce capacity are returned. Replacing an existing key does not evict.
func (c *LRU) Add(key string, value interface{}) (evicted int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var expiresAt time.Time
	if c.ttl > 0 {
		expiresAt = c.now().Add(c.ttl)
	}
	if el, ok := c.items[key]; ok {
		entry := el.Value.(*lruEntry)
		entry.value = value
		entry.expiresAt = expiresAt
		c.ll.MoveToFront(el)
		return 0
	}
	entry := &lruEntry{key: key, value: value, expiresAt: expiresAt}
	el := c.ll.PushFront(entry)
	c.items[key] = el
	for c.capacity > 0 && c.ll.Len() > c.capacity {
		c.removeOldest(false)
		evicted++
	}
	return evicted
}

// Remove deletes key from the cache and reports whether it was present.
func (c *LRU) Remove(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.removeElement(el, false)
		return true
	}
	return false
}

// Len returns the current number of entries, excluding expired entries that
// have not yet been swept.
func (c *LRU) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// Clear removes every entry; eviction counters are not updated.
func (c *LRU) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll.Init()
	c.items = make(map[string]*list.Element)
}

// Close stops the janitor goroutine (if any) and clears the cache. It is safe
// to call multiple times.
func (c *LRU) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	stop := c.stopJan
	c.mu.Unlock()
	close(stop)
	if c.janDone != nil {
		<-c.janDone
	}
	c.Clear()
}

// Snapshot returns a copy of the operation counters.
func (c *LRU) Snapshot() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

// AddLoad records one completed loader execution.
func (c *LRU) AddLoad() {
	c.mu.Lock()
	c.stats.Loads++
	c.mu.Unlock()
}

// AddLoadError records one failed loader execution.
func (c *LRU) AddLoadError() {
	c.mu.Lock()
	c.stats.LoadErrors++
	c.mu.Unlock()
}

// AddLoadDedup records one request served by an in-flight load.
func (c *LRU) AddLoadDedup() {
	c.mu.Lock()
	c.stats.LoadDedups++
	c.mu.Unlock()
}

// removeOldest evicts the least recently used entry. Caller holds c.mu.
func (c *LRU) removeOldest(expired bool) {
	el := c.ll.Back()
	if el != nil {
		c.removeElement(el, expired)
	}
}

// removeElement unlinks an entry and updates counters. Caller holds c.mu.
func (c *LRU) removeElement(el *list.Element, expired bool) {
	entry := el.Value.(*lruEntry)
	c.ll.Remove(el)
	delete(c.items, entry.key)
	c.stats.Evictions++
	if expired {
		c.stats.Expirations++
	}
}

// deleteExpired sweeps all expired entries.
func (c *LRU) deleteExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for el := c.ll.Back(); el != nil; {
		prev := el.Prev()
		entry := el.Value.(*lruEntry)
		if !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
			c.removeElement(el, true)
		}
		el = prev
	}
}
