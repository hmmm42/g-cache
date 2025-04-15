package cache

import (
	"log"
	"sync"
)

type Group struct {
	name      string
	mainCache *cache

	retriever Retriever
	server    Picker
}

var (
	mu           sync.RWMutex
	groupManager = make(map[string]*Group)
)

func NewGroup(name string, maxBytes int64, retriever Retriever) *Group {
	if retriever == nil {
		panic("retriever cannot be nil")
	}

	mu.Lock()
	defer mu.Unlock()

	if group, exists := groupManager[name]; exists {
		return group
	}

	c, err := newCache(maxBytes)
	if err != nil {
		log.Printf("failed to create cache: %v", err)
		return nil
	}

	group := &Group{
		name:      name,
		mainCache: c,
		retriever: retriever,
	}
	groupManager[name] = group
	return group
}
