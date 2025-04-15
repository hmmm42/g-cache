package main

import (
	"fmt"

	"github.com/hmmm42/g-cache/config"
)

func main() {
	fmt.Println(config.Conf.Eviction)

	// 使用配置创建缓存实例
	//cache := eviction.NewCacheUseLRU[string](1024*1024*10, nil, nil)

	// 主程序逻辑...
}
