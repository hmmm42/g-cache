package cache

import (
	"slices"
	"time"
)

type ByteView struct {
	b        []byte
	expireAt time.Time // Expiration time, 0 means no expiration
}

func (v ByteView) Len() int {
	return len(v.b)
}

func (v ByteView) ByteSlice() []byte {
	return slices.Clone(v.b)
}

func (v ByteView) Bytes() []byte {
	return v.b
}

func (v ByteView) String() string {
	return string(v.b)
}

func (v ByteView) isExpired() bool {
	return !v.expireAt.IsZero() && time.Now().After(v.expireAt)
}
