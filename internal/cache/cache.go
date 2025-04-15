package cache

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/hmmm42/g-cache/internal/cache/byteview"
	"github.com/hmmm42/g-cache/internal/cache/eviction"
)

type cache struct {
	mu       sync.RWMutex
	strategy eviction.CacheStrategy[byteview.ByteView]
	maxBytes int64
}

func newCache(maxBytes int64) (*cache, error) {
	if maxBytes < 0 {
		return nil, errors.New("maxBytes must be greater than or equal to 0")
	}

	onEvicted := func(key string, _ byteview.ByteView) {
		// TODO: use logrus
		log.Printf("cache entry evicted: key=%s", key)
	}

	s, err := eviction.New[byteview.ByteView]("", maxBytes, onEvicted)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache strategy: %w", err)
	}

	return &cache{
		strategy: s,
		maxBytes: maxBytes,
	}, nil
}

func (c *cache) get(key string) (byteview.ByteView, bool) {
	if c == nil {
		return byteview.ByteView{}, false
	}

	//TODO: add metrics

	c.mu.RLock()
	defer c.mu.RUnlock()

	if v, _, exists := c.strategy.Get(key); exists {
		return v, true
	}
	return byteview.ByteView{}, false
}

func (c *cache) put(key string, value byteview.ByteView) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.strategy.Put(key, value)
}
