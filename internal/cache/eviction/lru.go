package eviction

import (
	"container/list"
	"hash/fnv"
	"sync"
	"time"

	"github.com/hmmm42/g-cache/config"
)

// segment represents a portion of the cache with its own lock
type segment[V any] struct {
	mu       sync.RWMutex
	maxBytes int64
	nbytes   int64

	// ll.Back: most recently used
	ll    *list.List
	cache map[string]*list.Element

	sizeFunc  SizeFunc
	OnEvicted func(key string, value V)
}

// removeOldest removes the least recently used item from a segment.
func (seg *segment[V]) removeOldest() {
	if ele := seg.ll.Front(); ele != nil {
		seg.removeElement(ele)
	}
}

// removeElement removes an element from a segment.
func (seg *segment[V]) removeElement(e *list.Element) {
	seg.ll.Remove(e)
	entry := e.Value.(*Entry[V])
	delete(seg.cache, entry.Key)
	seg.nbytes -= int64(len(entry.Key)) + seg.sizeFunc(entry.Value)
	if seg.OnEvicted != nil {
		seg.OnEvicted(entry.Key, entry.Value)
	}
}

type CacheUseLRU[V any] struct {
	segments        []*segment[V]
	numSegments     int
	cleanupInterval time.Duration
	ttl             time.Duration
	stopCleanup     chan struct{}
	mu              sync.RWMutex
	sizeFunc        SizeFunc
}

func NewCacheUseLRU[V any](maxBytes int64, sizeFunc SizeFunc, onEvicted func(key string, value V)) *CacheUseLRU[V] {
	if sizeFunc == nil {
		sizeFunc = DefaultSizeFunc
	}
	cfg := config.Conf.Eviction
	c := &CacheUseLRU[V]{
		segments:        make([]*segment[V], cfg.NumSegments),
		numSegments:     cfg.NumSegments,
		cleanupInterval: cfg.CleanUpInterval,
		ttl:             cfg.TTL,
		stopCleanup:     make(chan struct{}),
		sizeFunc:        sizeFunc,
	}

	segmentMaxBytes := maxBytes / int64(c.numSegments)
	for i := range c.segments {
		c.segments[i] = &segment[V]{
			maxBytes:  segmentMaxBytes,
			ll:        list.New(),
			cache:     make(map[string]*list.Element),
			OnEvicted: onEvicted,
			sizeFunc:  sizeFunc,
		}
	}

	go c.cleanupRoutine()
	return c
}

// getSegment returns the appropriate segment for a given key.
// Use the FNV hash algorithm to determine which segment the key belongs to.
func (c *CacheUseLRU[V]) getSegment(key string) *segment[V] {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return c.segments[h.Sum32()%uint32(c.numSegments)]
}

// SetTTL sets the time-to-live for cache entries.
func (c *CacheUseLRU[V]) SetTTL(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttl = ttl
}

// SetCleanupInterval sets the interval between cleanup runs.
func (c *CacheUseLRU[V]) SetCleanupInterval(interval time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Stop existing cleanup routine
	close(c.stopCleanup)

	// Create new stop channel and set new interval
	c.stopCleanup = make(chan struct{})
	c.cleanupInterval = interval

	// Start new cleanup routine
	go c.cleanupRoutine()
}

// Stop stops the cleanup routine.
func (c *CacheUseLRU[V]) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	close(c.stopCleanup)
}

// cleanupRoutine periodically cleans up expired entries across all segments.
func (c *CacheUseLRU[V]) cleanupRoutine() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			currentTTL := c.ttl // capture current TTL value
			c.CleanUp(currentTTL)
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanupSegment removes expired entries from a single segment
func (c *CacheUseLRU[V]) cleanupSegment(seg *segment[V], ttl time.Duration) {
	seg.mu.Lock()
	defer seg.mu.Unlock()

	var next *list.Element
	for e := seg.ll.Front(); e != nil; e = next {
		next = e.Next()
		if e.Value == nil {
			continue
		}
		if e.Value.(*Entry[V]).Expired(ttl) {
			seg.removeElement(e)
		}
	}
}

func (c *CacheUseLRU[V]) Get(key string) (value V, updateAt time.Time, found bool) {
	seg := c.getSegment(key)
	seg.mu.RLock()
	defer seg.mu.RUnlock()

	if ele, hit := seg.cache[key]; hit {
		seg.ll.MoveToBack(ele)
		entry := ele.Value.(*Entry[V])
		entry.Touch()
		return entry.Value, entry.UpdateAt, true
	}
	return value, time.Time{}, false
}

func (c *CacheUseLRU[V]) Put(key string, value V) {
	seg := c.getSegment(key)
	seg.mu.Lock()
	defer seg.mu.Unlock()

	newBytes := int64(len(key)) + c.sizeFunc(value)
	if ele, hit := seg.cache[key]; hit {
		entry := ele.Value.(*Entry[V])
		oldBytes := int64(len(entry.Key)) + seg.sizeFunc(entry.Value)
		entry.Value = value
		entry.Touch()
		seg.ll.MoveToBack(ele)
		seg.nbytes += newBytes - oldBytes
	} else {
		entry := NewEntryAtNow[V](key, value)
		newEle := seg.ll.PushBack(entry)
		seg.cache[key] = newEle
		seg.nbytes += newBytes
	}

	for seg.maxBytes != 0 && seg.nbytes > seg.maxBytes {
		seg.removeOldest()
	}
}

func (c *CacheUseLRU[K]) Len() int {
	total := 0
	for _, seg := range c.segments {
		seg.mu.RLock()
		total += len(seg.cache) // Use map length instead of list length
		seg.mu.RUnlock()
	}
	return total
}

func (c *CacheUseLRU[V]) CleanUp(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, seg := range c.segments {
		c.cleanupSegment(seg, ttl)
	}
}
