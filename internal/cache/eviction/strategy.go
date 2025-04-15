package eviction

import "time"

//TODO: support LFU, ARC, LRU-K

type CacheStrategy[V any] interface {
	Get(key string) (value V, updateAt time.Time, found bool)
	Put(key string, value V)
	CleanUp(ttl time.Duration)
	Len() int
}

type Entry[V any] struct {
	Key      string
	Value    V
	UpdateAt time.Time
}

func NewEntryAtNow[V any](key string, value V) *Entry[V] {
	return &Entry[V]{
		Key:      key,
		Value:    value,
		UpdateAt: time.Now(),
	}
}

// Expired checks if the entry has expired based on the given duration.
func (e *Entry[V]) Expired(duration time.Duration) bool {
	if e.UpdateAt.IsZero() {
		return false // Never expires if update time is not set
	}
	return e.UpdateAt.Add(duration).Before(time.Now())
}

// Touch updates the entry's last access time to now.
func (e *Entry[V]) Touch() {
	e.UpdateAt = time.Now()
}

func New[V any](_ string, maxBytes int64, onEvicted func(string, V)) (CacheStrategy[V], error) {
	return NewCacheUseLRU[V](maxBytes, DefaultSizeFunc, onEvicted), nil
}
