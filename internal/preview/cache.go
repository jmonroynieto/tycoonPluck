package preview

import (
	"context"
	"image"
	"sync"
)

// Default thumbnail-cache knobs. Sized for swipe boards (≤12 visible) plus a
// modest lookahead of upcoming queue items without unbounded RAM growth.
const (
	DefaultCacheMaxEntries = 48
	DefaultCacheWorkers    = 2
	// SwipeThumbDPI is the resolution used for swipe-board first-page thumbs.
	SwipeThumbDPI = 72
	// ReviewPreviewDPI / ReviewMaxPages match the classic single-file preview.
	ReviewPreviewDPI = 120
	ReviewMaxPages   = 3
)

// Cache memoizes PDF rasters (swipe thumbs or multi-page review previews) with:
//   - Get / GetPages: pure check + retrieve (no work)
//   - Want: per-kind interest set → halt work of that kind outside it, prefetch misses
//   - GetAsync / GetPagesAsync: check, else join/start render, deliver via callback
//
// Keys are (path, dpi, maxPages) so swipe (72 DPI, 1 page) and review (120 DPI,
// up to 3 pages) share one store without colliding. Want only replaces interest
// for the same (dpi, maxPages) kind, so the other mode's inflight work survives.
//
// Cancellation is cooperative through context: dropping interest cancels
// pdftoppm; completed results for unwanted keys are discarded (not stored).
type Cache struct {
	mu       sync.Mutex
	maxSize  int
	workers  chan struct{} // semaphore for concurrent pdftoppm
	entries  map[cacheKey]*cacheEntry
	lru      []cacheKey // oldest at head
	inflight map[cacheKey]*inflightRender
	wanted   map[cacheKey]struct{}
}

type cacheKey struct {
	path     string
	dpi      int
	maxPages int
}

type cacheEntry struct {
	pages []image.Image
}

type inflightRender struct {
	cancel context.CancelFunc
	done   chan struct{} // closed when finished
	pages  []image.Image
	err    error
}

// CacheOption configures NewCache.
type CacheOption func(*Cache)

// WithMaxEntries sets the LRU capacity (minimum 1).
func WithMaxEntries(n int) CacheOption {
	return func(c *Cache) {
		if n < 1 {
			n = 1
		}
		c.maxSize = n
	}
}

// WithWorkers sets max concurrent pdftoppm jobs (minimum 1).
func WithWorkers(n int) CacheOption {
	return func(c *Cache) {
		if n < 1 {
			n = 1
		}
		c.workers = make(chan struct{}, n)
	}
}

// NewCache builds an empty thumbnail cache.
func NewCache(opts ...CacheOption) *Cache {
	c := &Cache{
		maxSize:  DefaultCacheMaxEntries,
		workers:  make(chan struct{}, DefaultCacheWorkers),
		entries:  make(map[cacheKey]*cacheEntry),
		inflight: make(map[cacheKey]*inflightRender),
		wanted:   make(map[cacheKey]struct{}),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func normalizeDPI(dpi int) int {
	if dpi <= 0 {
		return SwipeThumbDPI
	}
	return dpi
}

func normalizeMaxPages(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

// Get returns a cached first-page image if present (maxPages=1 key).
// It never starts work.
func (c *Cache) Get(path string, dpi int) (image.Image, bool) {
	pages, ok := c.GetPages(path, dpi, 1)
	if !ok || len(pages) == 0 {
		return nil, false
	}
	return pages[0], true
}

// GetPages returns cached page rasters if present (and marks LRU).
// It never starts work.
func (c *Cache) GetPages(path string, dpi, maxPages int) ([]image.Image, bool) {
	if c == nil || path == "" {
		return nil, false
	}
	k := cacheKey{path, normalizeDPI(dpi), normalizeMaxPages(maxPages)}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[k]
	if !ok {
		return nil, false
	}
	c.touchLocked(k)
	// Return a copy of the slice header so callers cannot append into the entry.
	out := make([]image.Image, len(e.pages))
	copy(out, e.pages)
	return out, true
}

// Len reports how many completed entries are stored.
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Inflight reports how many renders are currently running or queued.
func (c *Cache) Inflight() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.inflight)
}

// Want declares the paths the UI expects soon at (dpi, maxPages).
//
// Only interest keys of that same kind are replaced — the other mode's
// Want set is left alone. Inflight work for this kind not in paths is
// cancelled; missing keys are prefetched.
//
// Safe to call often (after board sync, slot replace, or review advance).
func (c *Cache) Want(paths []string, dpi, maxPages int) {
	if c == nil {
		return
	}
	dpi = normalizeDPI(dpi)
	maxPages = normalizeMaxPages(maxPages)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Drop previous interest for this kind only.
	for k := range c.wanted {
		if k.dpi == dpi && k.maxPages == maxPages {
			delete(c.wanted, k)
		}
	}

	next := make(map[cacheKey]struct{}, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		k := cacheKey{p, dpi, maxPages}
		next[k] = struct{}{}
		c.wanted[k] = struct{}{}
	}

	// Halt inflight of this kind that is no longer interesting.
	for k, inf := range c.inflight {
		if k.dpi != dpi || k.maxPages != maxPages {
			continue
		}
		if _, ok := next[k]; !ok {
			inf.cancel()
		}
	}

	// Prefetch misses (and re-touch hits).
	for k := range next {
		if _, ok := c.entries[k]; ok {
			c.touchLocked(k)
			continue
		}
		if _, ok := c.inflight[k]; ok {
			continue
		}
		c.startLocked(k)
	}
}

// GetAsync is GetPagesAsync for a single first page (swipe thumbs).
func (c *Cache) GetAsync(path string, dpi int, cb func(image.Image, error)) {
	if cb == nil {
		return
	}
	c.GetPagesAsync(path, dpi, 1, func(pages []image.Image, err error) {
		if err != nil || len(pages) == 0 {
			cb(nil, err)
			return
		}
		cb(pages[0], nil)
	})
}

// GetPagesAsync checks the cache, otherwise joins/starts a render and invokes
// cb once. cb may run on a worker goroutine — UI callers must hop threads.
//
// If the render is halted before completion, cb receives context.Canceled.
func (c *Cache) GetPagesAsync(path string, dpi, maxPages int, cb func([]image.Image, error)) {
	if cb == nil {
		return
	}
	if c == nil {
		cb(nil, context.Canceled)
		return
	}
	if path == "" {
		cb(nil, context.Canceled)
		return
	}
	dpi = normalizeDPI(dpi)
	maxPages = normalizeMaxPages(maxPages)
	k := cacheKey{path, dpi, maxPages}

	c.mu.Lock()
	if e, ok := c.entries[k]; ok {
		c.touchLocked(k)
		out := make([]image.Image, len(e.pages))
		copy(out, e.pages)
		c.mu.Unlock()
		cb(out, nil)
		return
	}
	// Ensure this key is wanted so a card/preview request still works even if
	// the last Want() omitted it.
	if c.wanted == nil {
		c.wanted = make(map[cacheKey]struct{})
	}
	c.wanted[k] = struct{}{}
	inf := c.inflight[k]
	if inf == nil {
		inf = c.startLocked(k)
	}
	done := inf.done
	c.mu.Unlock()

	go func() {
		<-done
		c.mu.Lock()
		pages, err := inf.pages, inf.err
		c.mu.Unlock()
		cb(pages, err)
	}()
}

// Drop removes all cached entries and cancels inflight work for path
// (any dpi / maxPages). Call when a file leaves the filesystem (assign/move).
func (c *Cache) Drop(path string) {
	if c == nil || path == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, inf := range c.inflight {
		if k.path == path {
			inf.cancel()
		}
	}
	for k := range c.entries {
		if k.path == path {
			delete(c.entries, k)
			c.removeLRULocked(k)
		}
	}
	for k := range c.wanted {
		if k.path == path {
			delete(c.wanted, k)
		}
	}
}

// Clear drops every entry and cancels all inflight work.
func (c *Cache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, inf := range c.inflight {
		inf.cancel()
	}
	c.entries = make(map[cacheKey]*cacheEntry)
	c.lru = nil
	c.inflight = make(map[cacheKey]*inflightRender)
	c.wanted = make(map[cacheKey]struct{})
}

// startLocked begins a render for k. Caller must hold c.mu.
func (c *Cache) startLocked(k cacheKey) *inflightRender {
	ctx, cancel := context.WithCancel(context.Background())
	inf := &inflightRender{
		cancel: cancel,
		done:   make(chan struct{}),
	}
	c.inflight[k] = inf
	go c.run(k, ctx, inf)
	return inf
}

func (c *Cache) run(k cacheKey, ctx context.Context, inf *inflightRender) {
	// Bound concurrent pdftoppm processes.
	select {
	case c.workers <- struct{}{}:
		defer func() { <-c.workers }()
	case <-ctx.Done():
		c.finish(k, inf, nil, ctx.Err())
		return
	}

	if err := ctx.Err(); err != nil {
		c.finish(k, inf, nil, err)
		return
	}

	pages, err := RenderPagesContext(ctx, k.path, k.dpi, k.maxPages)
	c.finish(k, inf, pages, err)
}

func (c *Cache) finish(k cacheKey, inf *inflightRender, pages []image.Image, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Another generation may have replaced this inflight entry.
	if c.inflight[k] != inf {
		inf.pages, inf.err = pages, err
		close(inf.done)
		return
	}
	delete(c.inflight, k)
	inf.pages, inf.err = pages, err

	// Only store successful results that are still wanted.
	if err == nil && len(pages) > 0 {
		if _, want := c.wanted[k]; want {
			c.storeLocked(k, pages)
		}
	}
	close(inf.done)
}

func (c *Cache) storeLocked(k cacheKey, pages []image.Image) {
	// Keep our own slice so callers of GetPages cannot mutate the entry.
	stored := make([]image.Image, len(pages))
	copy(stored, pages)
	if _, ok := c.entries[k]; ok {
		c.entries[k] = &cacheEntry{pages: stored}
		c.touchLocked(k)
		return
	}
	c.entries[k] = &cacheEntry{pages: stored}
	c.lru = append(c.lru, k)
	for len(c.entries) > c.maxSize {
		oldest := c.lru[0]
		c.lru = c.lru[1:]
		delete(c.entries, oldest)
	}
}

func (c *Cache) touchLocked(k cacheKey) {
	for i, x := range c.lru {
		if x == k {
			c.lru = append(c.lru[:i], c.lru[i+1:]...)
			break
		}
	}
	c.lru = append(c.lru, k)
}

func (c *Cache) removeLRULocked(k cacheKey) {
	for i, x := range c.lru {
		if x == k {
			c.lru = append(c.lru[:i], c.lru[i+1:]...)
			return
		}
	}
}
