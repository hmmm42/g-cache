package eviction

import (
	"unsafe"

	"github.com/hmmm42/g-cache/internal/cache/types"
)

// SizeFunc 是计算值大小的函数类型
type SizeFunc func(value any) int64

// DefaultSizeFunc 是默认的大小计算函数
// 它为常见类型提供了合理的大小估计
var DefaultSizeFunc = func(value any) int64 {
	switch v := value.(type) {
	case string:
		return int64(len(v))
	case []byte:
		return int64(len(v))
	case types.ByteView:
		return int64(v.Len())
	case nil:
		return 0
	default:
		return int64(unsafe.Sizeof(v))
	}
}
