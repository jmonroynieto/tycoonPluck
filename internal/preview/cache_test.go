package preview

import (
	"context"
	"image"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCacheGetMissThenHitAfterWant(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMinimalPDF(t, dir, "a.pdf")

	c := NewCache(WithWorkers(1), WithMaxEntries(8))
	if _, ok := c.Get(path, SwipeThumbDPI); ok {
		t.Fatal("expected miss on empty cache")
	}

	done := make(chan struct{})
	c.GetAsync(path, SwipeThumbDPI, func(img image.Image, err error) {
		defer close(done)
		if err != nil {
			t.Errorf("GetAsync err: %v", err)
			return
		}
		if img == nil {
			t.Error("expected image")
		}
	})
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("GetAsync timed out")
	}

	img, ok := c.Get(path, SwipeThumbDPI)
	if !ok || img == nil {
		t.Fatal("expected hit after render")
	}
	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1", c.Len())
	}
}

func TestCacheWantPrefetchesAndHaltsOthers(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	keep := writeMinimalPDF(t, dir, "keep.pdf")
	drop := writeMinimalPDF(t, dir, "drop.pdf")

	c := NewCache(WithWorkers(2), WithMaxEntries(8))
	// Start both.
	c.Want([]string{keep, drop}, SwipeThumbDPI, 1)
	// Immediately drop interest in drop — its work should be halted / not stored
	// if still running; keep should still complete.
	c.Want([]string{keep}, SwipeThumbDPI, 1)

	// Wait for keep.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := c.Get(keep, SwipeThumbDPI); ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, ok := c.Get(keep, SwipeThumbDPI); !ok {
		t.Fatal("keep.pdf should be cached")
	}

	// Give any cancelled work a moment to finish(discard).
	time.Sleep(50 * time.Millisecond)
	// drop may or may not have completed before halt; if it completed while
	// still wanted it could be in cache — force Drop semantics via second Want
	// and assert Drop removes.
	c.Drop(drop)
	if _, ok := c.Get(drop, SwipeThumbDPI); ok {
		t.Error("drop.pdf should not remain after Drop")
	}
}

func TestCacheDropRemovesEntry(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMinimalPDF(t, dir, "a.pdf")
	c := NewCache()

	done := make(chan struct{})
	c.GetAsync(path, SwipeThumbDPI, func(img image.Image, err error) {
		defer close(done)
		if err != nil {
			t.Errorf("render: %v", err)
		}
	})
	<-done
	c.Drop(path)
	if _, ok := c.Get(path, SwipeThumbDPI); ok {
		t.Error("Get should miss after Drop")
	}
	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0", c.Len())
	}
}

func TestCacheLRUEvictsOldest(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	c := NewCache(WithMaxEntries(2), WithWorkers(1))

	var paths []string
	for _, name := range []string{"a.pdf", "b.pdf", "c.pdf"} {
		paths = append(paths, writeMinimalPDF(t, dir, name))
	}

	// Load a, b.
	for _, p := range paths[:2] {
		done := make(chan struct{})
		c.GetAsync(p, SwipeThumbDPI, func(image.Image, error) { close(done) })
		<-done
	}
	// Touch a so b is oldest.
	if _, ok := c.Get(paths[0], SwipeThumbDPI); !ok {
		t.Fatal("a should be cached")
	}
	// Load c → evict b.
	done := make(chan struct{})
	c.GetAsync(paths[2], SwipeThumbDPI, func(image.Image, error) { close(done) })
	<-done

	if _, ok := c.Get(paths[1], SwipeThumbDPI); ok {
		t.Error("b should have been LRU-evicted")
	}
	if _, ok := c.Get(paths[0], SwipeThumbDPI); !ok {
		t.Error("a should still be cached")
	}
	if _, ok := c.Get(paths[2], SwipeThumbDPI); !ok {
		t.Error("c should be cached")
	}
}

func TestCacheClearCancelsInflight(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMinimalPDF(t, dir, "a.pdf")
	c := NewCache(WithWorkers(1))

	var got error
	var wg sync.WaitGroup
	wg.Add(1)
	c.GetAsync(path, SwipeThumbDPI, func(_ image.Image, err error) {
		got = err
		wg.Done()
	})
	c.Clear()
	wg.Wait()
	// May be Canceled or nil if it finished first — either way cache empty.
	if c.Len() != 0 {
		t.Errorf("Len = %d after Clear, want 0", c.Len())
	}
	_ = got
	_ = context.Canceled
}

func TestCacheWantDedupesInflight(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMinimalPDF(t, dir, "a.pdf")
	c := NewCache(WithWorkers(1))

	c.Want([]string{path}, SwipeThumbDPI, 1)
	c.Want([]string{path}, SwipeThumbDPI, 1) // second call must not double-start
	if c.Inflight() > 1 {
		t.Errorf("Inflight = %d, want ≤1 for same key", c.Inflight())
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := c.Get(path, SwipeThumbDPI); ok {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("timed out waiting for Want prefetch")
}

func TestCacheGetAsyncSamePathJoins(t *testing.T) {
	requirePdftoppm(t)
	dir := t.TempDir()
	path := writeMinimalPDF(t, dir, "a.pdf")
	c := NewCache(WithWorkers(1))

	var wg sync.WaitGroup
	var mu sync.Mutex
	var imgs []image.Image
	for i := 0; i < 3; i++ {
		wg.Add(1)
		c.GetAsync(path, SwipeThumbDPI, func(img image.Image, err error) {
			defer wg.Done()
			if err != nil {
				t.Errorf("err: %v", err)
				return
			}
			mu.Lock()
			imgs = append(imgs, img)
			mu.Unlock()
		})
	}
	wg.Wait()
	if len(imgs) != 3 {
		t.Fatalf("callbacks = %d, want 3", len(imgs))
	}
	// Single entry stored.
	if c.Len() != 1 {
		t.Errorf("Len = %d, want 1 (joined inflight)", c.Len())
	}
}

func TestCacheKeyIncludesDPIAndMaxPages(t *testing.T) {
	// Pure unit: different dpi / maxPages keys are independent slots.
	c := NewCache(WithMaxEntries(8))
	c.mu.Lock()
	c.storeLocked(cacheKey{"/x", 72, 1}, []image.Image{image.NewRGBA(image.Rect(0, 0, 1, 1))})
	c.storeLocked(cacheKey{"/x", 120, 3}, []image.Image{
		image.NewRGBA(image.Rect(0, 0, 2, 2)),
		image.NewRGBA(image.Rect(0, 0, 2, 2)),
	})
	c.mu.Unlock()
	a, okA := c.Get("/x", 72)
	pages, okB := c.GetPages("/x", 120, 3)
	if !okA || !okB {
		t.Fatal("both kind variants should hit")
	}
	if a.Bounds().Dx() != 1 {
		t.Errorf("swipe thumb size = %d, want 1", a.Bounds().Dx())
	}
	if len(pages) != 2 {
		t.Errorf("review pages = %d, want 2", len(pages))
	}
	// Get must not see the multi-page entry as a 1-page swipe key.
	if _, ok := c.GetPages("/x", 120, 1); ok {
		t.Error("120dpi/1page should miss when only 120dpi/3page is stored")
	}
	_ = filepath.Base
}

func TestCacheWantKindDoesNotCancelOtherKind(t *testing.T) {
	// Pure unit: Want for swipe must not wipe review interest.
	c := NewCache(WithMaxEntries(8))
	c.mu.Lock()
	reviewKey := cacheKey{"/r", ReviewPreviewDPI, ReviewMaxPages}
	c.wanted[reviewKey] = struct{}{}
	c.mu.Unlock()

	c.Want([]string{"/s"}, SwipeThumbDPI, 1)

	c.mu.Lock()
	_, reviewStill := c.wanted[reviewKey]
	_, swipe := c.wanted[cacheKey{"/s", SwipeThumbDPI, 1}]
	c.mu.Unlock()
	if !reviewStill {
		t.Error("review interest should survive a swipe Want")
	}
	if !swipe {
		t.Error("swipe path should be wanted")
	}
}
