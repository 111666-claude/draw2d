package cache

import (
	"runtime"
	"testing"
	"time"
)

// TestLRUJanitorStoppedOnClose verifies that caches created with a TTL do not
// leak their janitor goroutine after Close.
func TestLRUJanitorStoppedOnClose(t *testing.T) {
	before := runtime.NumGoroutine()
	const n = 25
	caches := make([]*LRU, n)
	for i := range caches {
		caches[i] = NewLRU(LRUConfig{Capacity: 4, TTL: 50 * time.Millisecond})
		caches[i].Add("k", i)
	}
	// Give janitors time to start.
	time.Sleep(80 * time.Millisecond)
	started := runtime.NumGoroutine()
	if started-before < n/2 {
		t.Fatalf("expected ~%d janitor goroutines, got delta %d", n, started-before)
	}
	for _, c := range caches {
		c.Close()
	}
	// GC and let schedulers reap the exited goroutines.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		if runtime.NumGoroutine() <= before+1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("janitor goroutines leaked: before=%d after=%d", before, runtime.NumGoroutine())
}

// TestLRUNoJanitorWithoutTTL verifies a TTL-free cache starts no janitor.
func TestLRUNoJanitorWithoutTTL(t *testing.T) {
	before := runtime.NumGoroutine()
	c := NewLRU(LRUConfig{Capacity: 4})
	defer c.Close()
	time.Sleep(50 * time.Millisecond)
	if got := runtime.NumGoroutine(); got > before {
		t.Fatalf("TTL-free cache started a janitor: before=%d after=%d", before, got)
	}
}
