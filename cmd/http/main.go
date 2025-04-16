package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"

	"github.com/hmmm42/g-cache/internal/cache"
	"github.com/hmmm42/g-cache/internal/transport/http"
)

var (
	port = flag.Int("port", 9999, "service node default port")
	api  = flag.Bool("api", false, "Start a api server?")
	// assuming that the api server and http://127.0.0.1:8001 are running on the same physical machine
	apiServerAddr1 = "http://127.0.0.1:9999"
	apiServerAddr2 = "http://127.0.0.1:10000"
)

var db = map[string]string{
	"key1": "value1",
	"key2": "value2",
	"key3": "value3",
}

func main() {
	flag.Parse()

	/* if you have a configuration center, both api client and http server configurations can be pulled from the configuration center */
	serverAddrMap := map[int]string{
		8000: "http://127.0.0.1:8000",
		8001: "http://127.0.0.1:8001",
		8002: "http://127.0.0.1:8002",
	}
	var serverAddrs []string
	for _, v := range serverAddrMap {
		serverAddrs = append(serverAddrs, v)
	}

	retriver := cache.RetrieveFunc(func(key string) ([]byte, error) {
		slog.Info("[InMemDB] retrieve key:", "key", key)
		if v, ok := db[key]; ok {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("key not found: %s", key)
	})

	//gm := cache.NewGroupManager([]string{"scores", "website"}, fmt.Sprintf("127.0.0.1:%d", *port), retriver)
	gm := cache.NewGroupManager([]string{"kv"}, retriver)

	// Start API servers
	errChan := make(chan error, 2)
	if *api {
		if *port == 8002 {

			go func() {
				if err := http.StartHTTPAPIServer(apiServerAddr1, gm["kv"]); err != nil {
					errChan <- fmt.Errorf("API server failed: %v", err)
				}
			}()
		}
	}

	// Start cache servers
	go func() {
		if err := http.StartHTTPCacheServer(serverAddrMap[*port], serverAddrs, gm["kv"]); err != nil {
			errChan <- fmt.Errorf("cache server failed: %v", err)
		}
	}()

	// Handle errors from goroutines
	for err := range errChan {
		log.Printf("Server error: %v", err)
	}
}
