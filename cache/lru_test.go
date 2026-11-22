package cache

import (
	"sync"
	"testing"
	"time"
)

func TestLRUBasicGetAdd(t *testing.T) {
	c := NewLRU(LRUConfig{Capacity: 2})
	defer c.Close()

	if _, ok := c.Get("missing"); ok {
		t.Fatal("empty cache returned a hit")
	}
	if got := c.Snapshot().Misses; got != 1 {
		t.Fatalf("Misses = %d, want 1", got)
	}

	c.Add("a", 1)
	c.Add("b", 2)
	if v, ok := c.Get("a"); !ok || v.(int) != 1 {
		t.Fatalf("Get(a) = %v, %v", v, ok)
	}
	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2", c.Len())
	}
	s := c.Snapshot()
	if s.Hits != 1 || s.Misses != 1 {
		t.Fatalf("stats = %+v", s)
	}
}

func TestLRUCapacityEvictionOrder(t *testing.T) {
	c := NewLRU(LRUConfig{Capacity: 3})
	defer c.Close()

	c.Add("a", 1)
	c.Add("b", 2)
	c.Add("c", 3)
	// Touch "a" so "b" becomes the least recently used.
	if _, ok := c.Get("a"); !ok {
		t.Fatal("missing a")
	}
	evicted := c.Add("d", 4)
	if evicted != 1 {
		t.Fatalf("Add returned %d evictions, want 1", evicted)
	}
	if c.Len() != 3 {
		t.Fatalf("Len = %d, want 3", c.Len())
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("b should have been evicted as LRU entry")
	}
	for _, k := range []string{"a", "c", "d"} {
		if _, ok := c.Get(k); !ok {
			t.Fatalf("%s should still be cached", k)
		}
	}
	s := c.Snapshot()
	if s.Evictions != 1 {
		t.Fatalf("Evictions = %d, want 1", s.Evictions)
	}
}

func TestLRUReplaceDoesNotEvict(t *testing.T) {
	c := NewLRU(LRUConfig{Capacity: 1})
	defer c.Close()

	c.Add("a", 1)
	if evicted := c.Add("a", 2); evicted != 0 {
		t.Fatalf("replacement evicted %d entries", evicted)
	}
	if v, ok := c.Get("a"); !ok || v.(int) != 2 {
		t.Fatalf("Get(a) = %v, %v", v, ok)
	}
	if c.Len() != 1 {
		t.Fatalf("Len = %d, want 1", c.Len())
	}
}

func TestLRUUnlimitedCapacity(t *testing.T) {
	// Zero capacity means unbounded.
	c := NewLRU(LRUConfig{Capacity: 0})
	defer c.Close()

	for i := 0; i < 1000; i++ {
		c.Add(string(rune('A'+i%26))+string(rune('a'+i/26)), i)
	}
	if c.Len() != 1000 {
		t.Fatalf("Len = %d, want 1000 (unbounded)", c.Len())
	}
	if s := c.Snapshot(); s.Evictions != 0 {
		t.Fatalf("Evictions = %d, want 0", s.Evictions)
	}
}

func TestLRURemoveAndClear(t *testing.T) {
	c := NewLRU(LRUConfig{Capacity: 10})
	defer c.Close()

	c.Add("a", 1)
	c.Add("b", 2)
	if !c.Remove("a") {
		t.Fatal("Remove(a) reported absent")
	}
	if c.Remove("a") {
		t.Fatal("second Remove(a) reported present")
	}
	if _, ok := c.Get("a"); ok {
		t.Fatal("a still present after Remove")
	}
	c.Clear()
	if c.Len() != 0 {
		t.Fatalf("Len = %d after Clear, want 0", c.Len())
	}
}

func TestLRUTTLExpiryLazy(t *testing.T) {
	now := time.Unix(1000, 0)
	c := NewLRU(LRUConfig{Capacity: 10, TTL: 5 * time.Second})
	c.now = func() time.Time { return now }
	defer c.Close()

	c.Add("a", 1)
	now = now.Add(4 * time.Second)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("entry expired before TTL")
	}
	now = now.Add(2 * time.Second) // 6s after insert
	if _, ok := c.Get("a"); ok {
		t.Fatal("entry should be expired")
	}
	s := c.Snapshot()
	if s.Expirations != 1 || s.Evictions != 1 {
		t.Fatalf("stats = %+v, want 1 expiration/eviction", s)
	}
	if s.Misses < 1 {
		t.Fatalf("Misses = %d, want >= 1", s.Misses)
	}
}

func TestLRUTTLRefreshedOnReplace(t *testing.T) {
	now := time.Unix(1000, 0)
	c := NewLRU(LRUConfig{Capacity: 10, TTL: 5 * time.Second})
	c.now = func() time.Time { return now }
	defer c.Close()

	c.Add("a", 1)
	now = now.Add(4 * time.Second)
	c.Add("a", 2)                  // fresh expiry
	now = now.Add(4 * time.Second) // 8s after first add, 4s after replace
	if _, ok := c.Get("a"); !ok {
		t.Fatal("entry should survive because expiry was refreshed")
	}
}

func TestLRUJanitorSweepsExpired(t *testing.T) {
	c := NewLRU(LRUConfig{Capacity: 10, TTL: 20 * time.Millisecond})
	defer c.Close()

	c.Add("a", 1)
	c.Add("b", 2)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c.Len() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if c.Len() != 0 {
		t.Fatalf("janitor did not sweep entries: Len = %d", c.Len())
	}
	s := c.Snapshot()
	if s.Expirations != 2 {
		t.Fatalf("Expirations = %d, want 2", s.Expirations)
	}
}

func TestLRUCloseIdempotent(t *testing.T) {
	c := NewLRU(LRUConfig{Capacity: 10, TTL: 10 * time.Millisecond})
	c.Add("a", 1)
	c.Close()
	c.Close() // must not panic or block
}

func TestLRUConcurrent(t *testing.T) {
	c := NewLRU(LRUConfig{Capacity: 64})
	defer c.Close()

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				key := string(rune('a'+i%32)) + "-" + string(rune('A'+n))
				c.Add(key, i)
				_, _ = c.Get(key)
				if i%50 == 0 {
					_ = c.Snapshot()
				}
				if i%100 == 0 {
					c.Remove(key)
				}
			}
		}(g)
	}
	wg.Wait()
	if c.Len() > 64 {
		t.Fatalf("Len = %d exceeds capacity 64", c.Len())
	}
}
