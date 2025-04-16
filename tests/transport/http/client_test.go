package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	httptransport "github.com/hmmm42/g-cache/internal/transport/http"
)

func TestHTTPFetcher_Fetch(t *testing.T) {
	// 测试数据
	testGroup := "testGroup"
	testKey := "testKey"
	testValue := "hello world"

	// 创建一个测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 检查URL路径是否包含正确的group和key
		if r.URL.Path != "/"+testGroup+"/"+testKey {
			t.Errorf("unexpected path: got %s, want /%s/%s", r.URL.Path, testGroup, testKey)
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}

		// 返回测试值
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte(testValue))
	}))
	defer server.Close()

	// 创建一个HTTPFetcher
	fetcher := httptransport.NewHTTPFetcher(server.URL + "/")

	// 调用Fetch
	data, err := fetcher.Fetch(testGroup, testKey)

	// 验证结果
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if string(data) != testValue {
		t.Errorf("Fetch returned unexpected data: got %q, want %q", string(data), testValue)
	}
}

func TestHTTPFetcher_FetchError(t *testing.T) {
	// 测试用例
	tests := []struct {
		name       string
		serverFunc func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name: "server returns 404",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "not found", http.StatusNotFound)
			},
		},
		{
			name: "server returns 500",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "internal error", http.StatusInternalServerError)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建测试服务器
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			// 创建一个HTTPFetcher
			fetcher := httptransport.NewHTTPFetcher(server.URL + "/")

			// 调用Fetch
			_, err := fetcher.Fetch("group", "key")

			// 验证错误
			if err == nil {
				t.Error("Fetch did not return error")
			}
		})
	}
}

func TestFetcherCreator(t *testing.T) {
	// 测试基础路径
	basePath := "/_gcache/"

	// 测试节点地址
	nodeAddr := "http://example.com"

	// 创建fetcher创建函数
	creator := httptransport.FetcherCreator(basePath)

	// 使用函数创建fetcher
	fetcher := creator(nodeAddr)

	// 确保fetcher不为nil
	if fetcher == nil {
		t.Fatal("FetcherCreator returned nil")
	}

	// 转换为HTTPFetcher类型来检查内部实现
	// 这需要内部类型暴露，如果不想暴露内部类型，可以通过行为测试来验证
	// 这里我们假设我们可以通过功能测试来验证它是否正确初始化
	httpFetcher, ok := fetcher.(*httptransport.HTTPFetcher)
	if !ok {
		t.Fatal("FetcherCreator did not return *HTTPFetcher")
	}

	// 我们不能直接检查内部字段，因为它们是私有的
	// 但我们可以通过调用Fetch方法来间接测试
	// 这里我们不真正调用，只是验证类型转换成功
	_ = httpFetcher
}
