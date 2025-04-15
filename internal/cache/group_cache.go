package cache

import (
	"log"
	"sync"

	"github.com/hmmm42/g-cache/config"
	"github.com/hmmm42/g-cache/internal/cache/singleflight"
)

type Group struct {
	name      string
	mainCache *cache

	retriever Retriever
	server    Picker

	flight *singleflight.Group
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
		flight:    singleflight.NewGroup(config.Conf.SingleFlight.TTL),
	}
	groupManager[name] = group
	return group
}

func (g *Group) RegisterServer(p Picker) {
	if g.server != nil {
		panic("server already registered for group")
	}
	g.server = p
}

func GetGroup(name string) *Group {
	mu.RLock()
	defer mu.RUnlock()
	return groupManager[name]
}
