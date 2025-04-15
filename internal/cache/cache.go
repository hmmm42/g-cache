package cache

import "sync"

type cache struct {
	mu       sync.RWMutex
	maxBytes int64
}
