package draw2d_test

import (
	"context"
	"errors"
	"io/ioutil"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/golang/freetype/truetype"
	"github.com/llgcode/draw2d"
)

// countingLoader is a fault-injecting FontLoader recording how many times each
// font was parsed.
type countingLoader struct {
	mu    sync.Mutex
	calls map[string]int
	fonts map[draw2d.FontData]*truetype.Font
	fail  map[draw2d.FontData]error // permanent errors per font
	gate  map[draw2d.FontData]chan struct{}
	delay time.Duration
}

func newCountingLoader() *countingLoader {
	return &countingLoader{
		calls: make(map[string]int),
		fonts: make(map[draw2d.FontData]*truetype.Font),
		fail:  make(map[draw2d.FontData]error),
		gate:  make(map[draw2d.FontData]chan struct{}),
	}
}

func (l *countingLoader) addFont(fd draw2d.FontData, font *truetype.Font) {
	l.mu.Lock()
	l.fonts[fd] = font
	l.mu.Unlock()
}

func (l *countingLoader) failWith(fd draw2d.FontData, err error) {
	l.mu.Lock()
	l.fail[fd] = err
	l.mu.Unlock()
}

func (l *countingLoader) gateFont(fd draw2d.FontData) chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	ch := make(chan struct{})
	l.gate[fd] = ch
	return ch
}

func (l *countingLoader) callCount(fd draw2d.FontData) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls[fd.Name]
}

func (l *countingLoader) LoadFont(fd draw2d.FontData) (*truetype.Font, error) {
	return l.LoadFontContext(context.Background(), fd)
}

func (l *countingLoader) LoadFontContext(ctx context.Context, fd draw2d.FontData) (*truetype.Font, error) {
	l.mu.Lock()
	l.calls[fd.Name]++
	err := l.fail[fd]
	font := l.fonts[fd]
	gate := l.gate[fd]
	delay := l.delay
	l.mu.Unlock()

	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	if font == nil {
		return nil, errors.New("font not registered with test loader")
	}
	return font, nil
}

func mustParseTTF(t *testing.T, path string) *truetype.Font {
	t.Helper()
	data, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	font, err := truetype.Parse(data)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return font
}

func testFontData(name string) draw2d.FontData {
	return draw2d.FontData{Name: name, Family: draw2d.FontFamilySans, Style: draw2d.FontStyleNormal}
}

func TestNewSyncFontCacheRejectsNilLoader(t *testing.T) {
	if _, err := draw2d.NewSyncFontCache(nil, draw2d.FontCacheConfig{Capacity: 4}); err == nil {
		t.Fatal("expected error for nil loader")
	}
}

func TestSyncFontCacheConcurrentLoadDedup(t *testing.T) {
	loader := newCountingLoader()
	fd := testFontData("luxi")
	loader.addFont(fd, mustParseTTF(t, "resource/font/luxisr.ttf"))
	// Gate the loader so all goroutines arrive while the first load is in
	// flight: this removes scheduling luck from the single-flight assertion.
	gate := loader.gateFont(fd)

	cache, err := draw2d.NewSyncFontCache(loader, draw2d.FontCacheConfig{Capacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	const n = 64
	var wg sync.WaitGroup
	results := make([]*truetype.Font, n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx], errs[idx] = cache.Load(fd)
		}(i)
	}
	close(start)
	// Park until the single in-flight load reaches the gate.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && loader.callCount(fd) == 0 {
		time.Sleep(time.Millisecond)
	}
	if got := loader.callCount(fd); got != 1 {
		t.Fatalf("loader started %d times before gate opened, want 1", got)
	}
	close(gate)
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil || results[i] == nil {
			t.Fatalf("goroutine %d: font=%v err=%v", i, results[i], errs[i])
		}
		if i > 0 && results[i] != results[0] {
			t.Fatalf("goroutine %d got a different font instance", i)
		}
	}
	if got := loader.callCount(fd); got != 1 {
		t.Fatalf("loader ran %d times, want exactly 1", got)
	}
	if cache.Len() != 1 {
		t.Fatalf("Len = %d, want 1", cache.Len())
	}
	s := cache.Stats()
	if s.Loads != 1 {
		t.Fatalf("Loads = %d, want 1", s.Loads)
	}

	// A second wave is served entirely from the cache.
	for i := 0; i < n; i++ {
		if _, err := cache.Load(fd); err != nil {
			t.Fatal(err)
		}
	}
	if got := loader.callCount(fd); got != 1 {
		t.Fatalf("loader ran %d times after cache fill, want 1", got)
	}
	if s := cache.Stats(); s.Hits < uint64(n) {
		t.Fatalf("Hits = %d, want >= %d", s.Hits, n)
	}
}
func TestSyncFontCacheLRUEvictionAndReload(t *testing.T) {
	loader := newCountingLoader()
	fonts := []draw2d.FontData{testFontData("a"), testFontData("b"), testFontData("c")}
	font := mustParseTTF(t, "resource/font/luxisr.ttf")
	for _, fd := range fonts {
		loader.addFont(fd, font)
	}

	cache, err := draw2d.NewSyncFontCache(loader, draw2d.FontCacheConfig{Capacity: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	for _, fd := range fonts {
		if _, err := cache.Load(fd); err != nil {
			t.Fatal(err)
		}
	}
	if cache.Len() != 2 {
		t.Fatalf("Len = %d, want bounded to 2", cache.Len())
	}
	if s := cache.Stats(); s.Evictions != 1 {
		t.Fatalf("Evictions = %d, want 1", s.Evictions)
	}
	// "a" was evicted first (LRU). Reloading it works and parses again.
	if _, err := cache.Load(fonts[0]); err != nil {
		t.Fatal(err)
	}
	if got := loader.callCount(fonts[0]); got != 2 {
		t.Fatalf("evicted font loaded %d times, want 2 (initial + reload)", got)
	}
	if cache.Len() != 2 {
		t.Fatalf("Len = %d after reload, want 2", cache.Len())
	}
}

func TestSyncFontCacheReadFailureNotCached(t *testing.T) {
	loader := newCountingLoader()
	fd := testFontData("missing")
	loader.failWith(fd, errors.New("disk read failed"))

	cache, err := draw2d.NewSyncFontCache(loader, draw2d.FontCacheConfig{Capacity: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	if _, err := cache.Load(fd); err == nil {
		t.Fatal("expected read error")
	}
	if cache.Len() != 0 {
		t.Fatalf("failed load cached an entry: Len = %d", cache.Len())
	}
	if _, err := cache.Load(fd); err == nil {
		t.Fatal("expected the read to be retried after failure")
	}
	if got := loader.callCount(fd); got != 2 {
		t.Fatalf("failing loader ran %d times, want 2 (errors are not cached)", got)
	}
	s := cache.Stats()
	if s.LoadErrors != 2 || s.Loads != 2 {
		t.Fatalf("stats = %+v, want 2 load errors/loads", s)
	}
}

// TestSyncFontCacheConcurrentFailureDedup makes many concurrent requests to a
// gated failing loader: the error must be produced once and shared, then the
// next wave retries and succeeds exactly once.
func TestSyncFontCacheConcurrentFailureDedup(t *testing.T) {
	loader := newCountingLoader()
	fd := testFontData("flaky")
	font := mustParseTTF(t, "resource/font/luxisr.ttf")
	loader.failWith(fd, errors.New("injected failure"))
	gate := loader.gateFont(fd)

	cache, err := draw2d.NewSyncFontCache(loader, draw2d.FontCacheConfig{Capacity: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	const n = 32
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, errs[idx] = cache.Load(fd)
		}(i)
	}
	close(start)
	time.Sleep(30 * time.Millisecond) // park all waiters on the in-flight load
	close(gate)
	wg.Wait()

	for i, err := range errs {
		if err == nil {
			t.Fatalf("goroutine %d expected the injected error", i)
		}
	}
	if got := loader.callCount(fd); got != 1 {
		t.Fatalf("failing loader ran %d times, want 1 for the wave", got)
	}

	// Recover the loader; the failed result was never cached.
	loader.mu.Lock()
	delete(loader.fail, fd)
	delete(loader.gate, fd)
	loader.fonts[fd] = font
	loader.mu.Unlock()

	if got, err := cache.Load(fd); err != nil || got != font {
		t.Fatalf("retry after failure: font=%v err=%v", got, err)
	}
	if got := loader.callCount(fd); got != 2 {
		t.Fatalf("loader ran %d times, want 2 after recovery", got)
	}
	s := cache.Stats()
	if s.LoadErrors != 1 || s.Loads != 2 {
		t.Fatalf("stats = %+v, want 1 error load and 2 total loads", s)
	}
	if s.LoadDedups < uint64(n-1) {
		t.Fatalf("LoadDedups = %d, want >= %d", s.LoadDedups, n-1)
	}
}

func TestSyncFontCacheBadFontNotCached(t *testing.T) {
	// A loader whose bytes never form a valid TTF: truetype.Parse must fail
	// and the failure must remain retryable.
	cache, err := draw2d.NewSyncFontCache(
		fontLoaderFunc(func(fd draw2d.FontData) (*truetype.Font, error) {
			return truetype.Parse([]byte("this is definitely not a truetype font"))
		}),
		draw2d.FontCacheConfig{Capacity: 4},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	fd := testFontData("broken")
	if _, err := cache.Load(fd); err == nil {
		t.Fatal("expected parse error for corrupted font")
	}
	if _, err := cache.Load(fd); err == nil {
		t.Fatal("corrupted font result must not be cached")
	}
	if cache.Len() != 0 {
		t.Fatalf("Len = %d, want 0", cache.Len())
	}
}

func TestSyncFontCacheDuplicateStore(t *testing.T) {
	loader := newCountingLoader()
	cache, err := draw2d.NewSyncFontCache(loader, draw2d.FontCacheConfig{Capacity: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	fd := testFontData("stored")
	first := mustParseTTF(t, "resource/font/luxisr.ttf")
	second := mustParseTTF(t, "resource/font/luxirb.ttf")

	cache.Store(fd, first)
	cache.Store(fd, second) // duplicate Store replaces, does not grow the cache
	if cache.Len() != 1 {
		t.Fatalf("Len = %d, want 1", cache.Len())
	}
	got, err := cache.Load(fd)
	if err != nil {
		t.Fatal(err)
	}
	if got != second {
		t.Fatal("Load did not return the most recently stored font")
	}
	if loader.callCount(fd) != 0 {
		t.Fatal("stored font triggered a disk parse")
	}

	// nil Store must be ignored.
	cache.Store(testFontData("nil"), nil)
	if cache.Len() != 1 {
		t.Fatalf("nil Store changed Len to %d", cache.Len())
	}
}

func TestSyncFontCacheTTLExpiry(t *testing.T) {
	loader := newCountingLoader()
	fd := testFontData("ttl")
	loader.addFont(fd, mustParseTTF(t, "resource/font/luxisr.ttf"))

	cache, err := draw2d.NewSyncFontCache(loader, draw2d.FontCacheConfig{
		Capacity: 4,
		TTL:      30 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	if _, err := cache.Load(fd); err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)
	if _, err := cache.Load(fd); err != nil {
		t.Fatal(err)
	}
	if got := loader.callCount(fd); got != 2 {
		t.Fatalf("font parsed %d times, want 2 after TTL expiry", got)
	}
	if s := cache.Stats(); s.Expirations == 0 {
		t.Fatalf("stats = %+v, want at least one expiration", s)
	}
}

func TestSyncFontCacheCancellationDoesNotKillSharedLoad(t *testing.T) {
	loader := newCountingLoader()
	fd := testFontData("slow")
	loader.addFont(fd, mustParseTTF(t, "resource/font/luxisr.ttf"))
	gate := loader.gateFont(fd)

	cache, err := draw2d.NewSyncFontCache(loader, draw2d.FontCacheConfig{Capacity: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	leaderErr := make(chan error, 1)
	go func() {
		_, e := cache.Load(fd)
		leaderErr <- e
	}()

	// Give the leader time to enter the loader and block on the gate.
	time.Sleep(30 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.LoadContext(ctx, fd); err == nil {
		t.Fatal("cancelled LoadContext returned nil error")
	}

	close(gate)
	if err := <-leaderErr; err != nil {
		t.Fatalf("leader load failed: %v", err)
	}
	if got := loader.callCount(fd); got != 1 {
		t.Fatalf("loader ran %d times, want 1", got)
	}
	// Leader's result is cached and served to the next caller.
	if _, err := cache.Load(fd); err != nil {
		t.Fatal(err)
	}
	if got := loader.callCount(fd); got != 1 {
		t.Fatalf("loader ran %d times, still want 1", got)
	}
}

func TestSyncFontCacheHighConcurrencyBounded(t *testing.T) {
	loader := newCountingLoader()
	const distinct = 16
	font := mustParseTTF(t, "resource/font/luxisr.ttf")
	var keys []draw2d.FontData
	for i := 0; i < distinct; i++ {
		fd := testFontData("font-" + string(rune('a'+i)))
		loader.addFont(fd, font)
		keys = append(keys, fd)
	}

	cache, err := draw2d.NewSyncFontCache(loader, draw2d.FontCacheConfig{Capacity: 8})
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	var wg sync.WaitGroup
	for g := 0; g < 32; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				fd := keys[(seed+i)%distinct]
				if _, err := cache.Load(fd); err != nil {
					t.Error(err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	if cache.Len() > 8 {
		t.Fatalf("Len = %d exceeds capacity 8", cache.Len())
	}
	totalCalls := 0
	for _, fd := range keys {
		totalCalls += loader.callCount(fd)
	}
	// Each font needs at least one parse; evictions force more but the count
	// stays far below the number of requests (32*500 = 16000).
	if totalCalls < distinct || totalCalls >= 16000 {
		t.Fatalf("total parses = %d outside expected bounded range", totalCalls)
	}
	t.Logf("high concurrency: %d requests, %d parses, %d evictions",
		32*500, totalCalls, cache.Stats().Evictions)
}

func TestFolderLoaderLoadsRealFont(t *testing.T) {
	loader := draw2d.NewFolderLoader("resource/font")
	fd := testFontData("luxi") // FontFileName -> luxisr.ttf
	font, err := loader.LoadFont(fd)
	if err != nil {
		t.Fatal(err)
	}
	if font == nil {
		t.Fatal("nil font without error")
	}

	// Missing file produces an error, not a panic.
	if _, err := loader.LoadFont(testFontData("does-not-exist")); err == nil {
		t.Fatal("expected error for missing font file")
	}

	// Folder and namer remain configurable.
	loader.SetFolder(filepath.Clean("resource/font"))
	loader.SetNamer(draw2d.FontFileName)
	if loader.Folder() != filepath.Clean("resource/font") {
		t.Fatalf("folder = %q", loader.Folder())
	}
}

func TestFolderLoaderContextCancellation(t *testing.T) {
	loader := draw2d.NewFolderLoader("resource/font")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loader.LoadFontContext(ctx, testFontData("luxi")); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

// fontLoaderFunc adapts a plain function to FontLoader.
type fontLoaderFunc func(draw2d.FontData) (*truetype.Font, error)

func (f fontLoaderFunc) LoadFont(fd draw2d.FontData) (*truetype.Font, error) {
	return f(fd)
}
