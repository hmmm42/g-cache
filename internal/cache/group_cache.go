package cache

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/hmmm42/g-cache/config"
	"github.com/hmmm42/g-cache/internal/cache/singleflight"
	"github.com/hmmm42/g-cache/internal/cache/types"
	"gorm.io/gorm"
)

// Group represents a cache namespace and associated data/operations.
type Group struct {
	name      string
	mainCache *cache

	retriever Retriever
	server    Picker

	flight *singleflight.Group
}

type GroupManager map[string]*Group

var (
	mu     sync.RWMutex
	Groups = make(GroupManager)
)

func NewGroup(name string, maxBytes int64, retriever Retriever) *Group {
	if retriever == nil {
		panic("retriever cannot be nil")
	}

	mu.Lock()
	defer mu.Unlock()

	if group, exists := Groups[name]; exists {
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
	Groups[name] = group
	return group
}

func NewGroupManager(groupNames []string, currentPeerAddr string, retrieverFunc RetrieveFunc) GroupManager {
	for _, name := range groupNames {
		retriever := retrieverFunc
		group := NewGroup(name, config.Conf.GroupManager.MaxCacheSize, retriever)
		Groups[name] = group
		log.Printf(
			"Group %s created with max cache size: %d, retriever: %v",
			name, group.mainCache.maxBytes, retriever,
		)
	}
	return Groups
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
	return Groups[name]
}

//func DestroyGroup(name string) {
//	g := GetGroup(name)
//	if g == nil {
//		return
//	}
//	if server, ok := g.server.(Picker); ok {
//
//	}
//}

func (g *Group) Get(key string) (types.ByteView, error) {
	if key == "" {
		return types.ByteView{}, fmt.Errorf("key cannot be empty")
	}

	if value, ok := g.mainCache.get(key); ok {
		return value, nil
	}

	return g.load(key)
}

func (g *Group) load(key string) (value types.ByteView, err error) {
	ctx := context.Background()
	view, err := g.flight.Do(ctx, key, func() (any, error) {
		if g.server != nil {
			if peer, ok := g.server.Pick(key); ok {
				if value, err = g.FetchFromPeer(peer, key); err == nil {
					return value, nil
				}
				log.Printf("Failed to fetch from peer %s: %v", peer, err)
			}
		}

		return g.getLocally(key)
	})

	if err != nil {
		return types.ByteView{}, err
	}
	return view.(types.ByteView), nil
}

func (g *Group) FetchFromPeer(peer Fetcher, key string) (types.ByteView, error) {
	bytes, err := peer.Fetch(g.name, key)
	if err != nil {
		return types.ByteView{}, err
	}
	return types.NewByteView(bytes), nil
}

func (g *Group) getLocally(key string) (types.ByteView, error) {
	bytes, err := g.retriever.retrieve(key)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("caching empty result for non-existent key %q to prevent cache penetration", key)
			g.populateCache(key, types.ByteView{})
		}
		return types.ByteView{}, fmt.Errorf("failed to retrieve key %q locally: %w", key, err)
	}

	value := types.NewByteView(bytes)
	g.populateCache(key, value)

	return value, nil
}

func (g *Group) populateCache(key string, value types.ByteView) {
	g.mainCache.put(key, value)
}
