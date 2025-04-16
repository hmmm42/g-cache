package picker

import (
	"hash/crc32"
	"slices"
	"sort"
	"strconv"
	"sync"
)

type Hash func(data []byte) uint32

type Map struct {
	mu       sync.RWMutex
	hash     Hash
	replicas int
	keys     []int
	hashMap  map[int]string
}

func New(replicas int, fn Hash) *Map {
	if replicas <= 0 {
		replicas = 1
	}

	m := &Map{
		replicas: replicas,
		hash:     fn,
		hashMap:  make(map[int]string),
	}
	if m.hash == nil {
		m.hash = crc32.ChecksumIEEE
	}
	return m
}

// Add 添加真实节点
func (m *Map) Add(nodes ...string) {
	if len(nodes) == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, node := range nodes {
		for i := range m.replicas {
			hash := int(m.hash([]byte(strconv.Itoa(i) + node)))
			m.keys = append(m.keys, hash)
			m.hashMap[hash] = node
		}
	}
	sort.Ints(m.keys)
}

// Get 根据传入的key查找真实节点
func (m *Map) Get(key string) string {
	if key == "" || m == nil {
		return ""
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	hash := int(m.hash([]byte(key)))
	idx := sort.Search(len(m.keys), func(i int) bool {
		return m.keys[i] >= hash
	})

	if idx == len(m.keys) {
		idx = 0
	}

	return m.hashMap[m.keys[idx]]
}

func (m *Map) Remove(node string) {
	if node == "" || m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	hashesToRemove := make([]int, 0, m.replicas)
	for i := range m.replicas {
		hash := int(m.hash([]byte(strconv.Itoa(i) + node)))
		if _, exists := m.hashMap[hash]; exists {
			hashesToRemove = append(hashesToRemove, hash)
			delete(m.hashMap, hash)
		}
	}

	if len(hashesToRemove) == 0 {
		return
	}

	newKeys := make([]int, 0, len(m.keys)-len(hashesToRemove))
	for _, k := range m.keys {
		if !slices.Contains(hashesToRemove, k) {
			newKeys = append(newKeys, k)
		}
	}
	m.keys = newKeys
}
