package http

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/hmmm42/g-cache/internal/cache"
)

type HTTPFetcher struct {
	baseURL string
}

func NewHTTPFetcher(baseURL string) cache.Fetcher {
	return &HTTPFetcher{
		baseURL: baseURL,
	}
}

func (h *HTTPFetcher) Fetch(group string, key string) ([]byte, error) {
	u := fmt.Sprintf("%v%v/%v", h.baseURL, url.QueryEscape(group), url.QueryEscape(key))

	res, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned: %v", res.Status)
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body failed: %v", err)
	}

	return b, nil

}

// FetcherCreator 返回一个创建HTTP获取器的函数
func FetcherCreator(basePath string) func(nodeAddr string) cache.Fetcher {
	return func(nodeAddr string) cache.Fetcher {
		return NewHTTPFetcher(nodeAddr + basePath)
	}
}
