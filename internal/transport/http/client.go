package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/hmmm42/g-cache/internal/cache"
)

type HTTPFetcher struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPFetcher(baseURL string) cache.Fetcher {
	return &HTTPFetcher{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

func (h *HTTPFetcher) Fetch(group string, key string) ([]byte, error) {
	ctx := context.Background()
	u := fmt.Sprintf("%s%s/%s", h.baseURL, url.QueryEscape(group), url.QueryEscape(key))

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request failed: %w", err)
	}

	res, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned: %v", res.Status)
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body failed: %w", err)
	}

	return b, nil
}

// FetcherCreator 返回一个创建HTTP获取器的函数
func FetcherCreator(basePath string) func(nodeAddr string) cache.Fetcher {
	return func(nodeAddr string) cache.Fetcher {
		return NewHTTPFetcher(nodeAddr + basePath)
	}
}
