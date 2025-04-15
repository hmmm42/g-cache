package singleflight

import (
	"context"
	"sync"
	"time"
)

type result struct {
	val any
	err error
}

type call struct {
	done   chan struct{}
	result result
}

type cacheEntry struct {
	result  result
	expires time.Time
}

type Group struct {
	mu      sync.RWMutex
	calls   map[string]*call
	cache   map[string]cacheEntry
	ttl     time.Duration
	cleanup *time.Ticker
	done    chan struct{}
}

func NewGroup(ttl time.Duration) *Group {
	if ttl <= 0 {
		ttl = time.Minute
	}

	g := &Group{
		calls:   make(map[string]*call),
		cache:   make(map[string]cacheEntry),
		ttl:     ttl,
		cleanup: time.NewTicker(ttl / 4),
		done:    make(chan struct{}),
	}
	return g
}

func (g *Group) Do(ctx context.Context, key string, fn func() (any, error)) (any, error) {
	if value, ok := g.checkCache(key); ok {
		return value.val, value.err
	}

	c, created := g.createAll(key)
	if !created {
		return g.waitForCall(ctx, c)
	}

	defer g.finishCall(key, c)

	return g.executeAndCache(ctx, key, fn, c)
}

func (g *Group) checkCache(key string) (result, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if entry, ok := g.cache[key]; ok && time.Now().Before(entry.expires) {
		return entry.result, true
	}
	return result{}, false
}

func (g *Group) createAll(key string) (c *call, ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if c, ok = g.calls[key]; ok {
		return c, false
	}

	c = &call{
		done: make(chan struct{}),
	}
	g.calls[key] = c
	return c, true
}

func (g *Group) waitForCall(ctx context.Context, c *call) (any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return c.result.val, c.result.err
	}
}

func (g *Group) finishCall(key string, c *call) {
	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	close(c.done)
}

func (g *Group) executeAndCache(ctx context.Context, key string, fn func() (any, error), c *call) (any, error) {
	value, err := g.executeWithContext(ctx, fn)
	res := result{val: value, err: err}
	c.result = res

	if err == nil {
		g.mu.Lock()
		g.cache[key] = cacheEntry{
			result:  res,
			expires: time.Now().Add(g.ttl),
		}
		g.mu.Unlock()
	}
	return value, err
}

func (g *Group) executeWithContext(ctx context.Context, fn func() (any, error)) (any, error) {
	var (
		value any
		err   error
		done  = make(chan struct{})
	)

	go func() {
		value, err = fn()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
		return value, err
	}
}

func (g *Group) cleanupLoop() {
	for {
		select {
		case <-g.done:
			return
		case <-g.cleanup.C:
			g.removeExpiredEntries()
		}
	}
}

func (g *Group) removeExpiredEntries() {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	for key, entry := range g.cache {
		if now.After(entry.expires) {
			delete(g.cache, key)
		}
	}
}

func (g *Group) Stop() {
	g.cleanup.Stop()
	close(g.done)

	g.mu.Lock()
	defer g.mu.Unlock()

	clear(g.calls)
	clear(g.cache)
}
