package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGroupSingleFlight(t *testing.T) {
	var g Group
	var calls int32

	const waiters = 64
	var wg sync.WaitGroup
	results := make([]interface{}, waiters)
	errs := make([]error, waiters)
	leaders := make([]bool, waiters)
	start := make(chan struct{})

	for i := 0; i < waiters; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx], leaders[idx], errs[idx] = g.Do(context.Background(), "k", func() (interface{}, error) {
				atomic.AddInt32(&calls, 1)
				time.Sleep(20 * time.Millisecond)
				return "value", nil
			})
		}(i)
	}
	close(start)
	wg.Wait()

	if calls != 1 {
		t.Fatalf("loader ran %d times, want exactly 1", calls)
	}
	leaderCount := 0
	for i := 0; i < waiters; i++ {
		if errs[i] != nil || results[i] != "value" {
			t.Fatalf("waiter %d got %v, %v", i, results[i], errs[i])
		}
		if leaders[i] {
			leaderCount++
		}
	}
	if leaderCount != 1 {
		t.Fatalf("leader count = %d, want 1", leaderCount)
	}
}

func TestGroupDistinctKeysRunSeparately(t *testing.T) {
	var g Group
	var calls int32
	var wg sync.WaitGroup
	for _, k := range []string{"a", "b", "c"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			v, _, err := g.Do(context.Background(), key, func() (interface{}, error) {
				atomic.AddInt32(&calls, 1)
				return key, nil
			})
			if err != nil || v != key {
				t.Errorf("key %s -> %v, %v", key, v, err)
			}
		}(k)
	}
	wg.Wait()
	if calls != 3 {
		t.Fatalf("loader ran %d times, want 3", calls)
	}
}

// TestGroupErrorSharedInWave verifies a gated failing loader executes exactly
// once while concurrent callers wait, and every waiter receives the error.
func TestGroupErrorSharedInWave(t *testing.T) {
	var g Group
	var calls int32
	wantErr := errors.New("boom")
	gate := make(chan struct{})

	const waiters = 8
	var wg sync.WaitGroup
	errs := make([]error, waiters)
	start := make(chan struct{})
	for i := 0; i < waiters; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, _, errs[idx] = g.Do(context.Background(), "k", func() (interface{}, error) {
				<-gate
				// Count only after the gate opens so the increment is fully
				// ordered with the test's unblock and later assertions.
				atomic.AddInt32(&calls, 1)
				return nil, wantErr
			})
		}(i)
	}
	close(start)
	// Give every goroutine time to park on the single in-flight call.
	time.Sleep(30 * time.Millisecond)
	close(gate)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("failing loader ran %d times, want 1 for the wave", got)
	}
	for i, err := range errs {
		if err != wantErr {
			t.Fatalf("waiter %d err = %v, want %v", i, err, wantErr)
		}
	}

	// The wave's cleanup (delete from the in-flight map) happens after the
	// waiters are released, so wg.Wait alone is not enough; wait explicitly.
	if !g.WaitUntilIdle("k", 2*time.Second) {
		t.Fatal("single-flight entry was never removed after the wave")
	}
	_, leader, err := g.Do(context.Background(), "k", func() (interface{}, error) {
		atomic.AddInt32(&calls, 1)
		return nil, wantErr
	})
	if err != wantErr {
		t.Fatalf("retry err = %v, want %v", err, wantErr)
	}
	if !leader {
		t.Fatal("retry joined a stale in-flight call instead of starting fresh")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("loader ran %d times, want 2", got)
	}
}

func TestGroupCancellationDoesNotKillSharedLoad(t *testing.T) {
	var g Group
	var calls int32
	started := make(chan struct{})

	// Leader runs to completion regardless of cancelled waiters.
	leaderDone := make(chan struct{})
	go func() {
		defer close(leaderDone)
		_, _, _ = g.Do(context.Background(), "k", func() (interface{}, error) {
			atomic.AddInt32(&calls, 1)
			close(started)
			time.Sleep(100 * time.Millisecond)
			return "ok", nil
		})
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	v, leader, err := g.Do(ctx, "k", func() (interface{}, error) {
		t.Fatal("cancelled waiter must not start a second loader")
		return nil, nil
	})
	if err == nil {
		t.Fatal("cancelled waiter returned nil error")
	}
	if leader {
		t.Fatal("cancelled waiter was reported as the leader")
	}
	if v != nil {
		t.Fatalf("cancelled waiter returned value %v", v)
	}
	<-leaderDone

	if calls != 1 {
		t.Fatalf("loader ran %d times, want 1", calls)
	}

	// The in-flight entry was removed when the leader finished, so a later
	// call starts a fresh load instead of deadlocking.
	v, _, err = g.Do(context.Background(), "k", func() (interface{}, error) {
		return "recovered", nil
	})
	if err != nil || v != "recovered" {
		t.Fatalf("call after completion = %v, %v", v, err)
	}
}

func TestGroupPanicPropagatesAndUnblocks(t *testing.T) {
	var g Group
	const waiters = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	panicked := make([]bool, waiters)
	for i := 0; i < waiters; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			defer func() {
				if r := recover(); r != nil {
					panicked[idx] = r == "kaboom"
				}
			}()
			_, _, _ = g.Do(context.Background(), "k", func() (interface{}, error) {
				panic("kaboom")
			})
		}(i)
	}
	close(start)
	wg.Wait()
	for i, p := range panicked {
		if !p {
			t.Fatalf("waiter %d did not observe the panic (possible deadlock swallowed)", i)
		}
	}

	// A later call for the same key must work once the panicking wave has
	// removed its in-flight entry.
	if !g.WaitUntilIdle("k", 2*time.Second) {
		t.Fatal("single-flight entry was never removed after panic")
	}
	v, _, err := g.Do(context.Background(), "k", func() (interface{}, error) {
		return "recovered", nil
	})
	if err != nil || v != "recovered" {
		t.Fatalf("call after panic = %v, %v", v, err)
	}
}
