package http_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hmmm42/g-cache/internal/cache"
	httptransport "github.com/hmmm42/g-cache/internal/transport/http"
)

func TestHTTPServer_StartAndStop(t *testing.T) {
	// 创建检索函数，返回测试数据
	retrieveFunc := func(key string) ([]byte, error) {
		if key == "test-key" {
			return []byte("test-value"), nil
		}
		return nil, nil
	}

	// 创建一个真实的Group实例，而不是mock
	group := cache.NewGroup("testGroup", 1024, cache.RetrieveFunc(retrieveFunc))

	// 创建HTTP服务器
	addr := "localhost:18080"
	server := httptransport.NewHTTPServer("http://"+addr, []string{"http://" + addr}, group)

	// 启动服务器
	if err := server.Start(addr); err != nil {
		t.Fatalf("Failed to start HTTP server: %v", err)
	}

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 创建一个自定义的HTTP客户端，设置短超时
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	// 停止服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Stop(ctx); err != nil {
		t.Errorf("Failed to stop HTTP server: %v", err)
	}

	// 验证服务器已停止 - 尝试连接应该失败
	_, err := client.Get("http://" + addr)
	if err == nil {
		t.Error("Server should be stopped, but connection succeeded")
	}
}

func TestAPIServer_StartAndStop(t *testing.T) {
	// 创建检索函数，返回测试数据
	retrieveFunc := func(key string) ([]byte, error) {
		if key == "test-key" {
			return []byte("test-value"), nil
		}
		return nil, nil
	}

	// 创建一个真实的Group实例
	group := cache.NewGroup("testGroup", 1024, cache.RetrieveFunc(retrieveFunc))

	// 创建API服务器
	addr := "localhost:18081"
	server := httptransport.NewAPIServer(group)

	// 启动服务器
	if err := server.Start(addr); err != nil {
		t.Fatalf("Failed to start API server: %v", err)
	}

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 创建一个自定义的HTTP客户端，设置短超时
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	// 测试API端点
	resp, err := client.Get("http://" + addr + "/api?key=test-key")
	if err != nil {
		t.Fatalf("Failed to call API: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		t.Errorf("API returned unexpected status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// 验证响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	expected := "test-value"
	if string(body) != expected {
		t.Errorf("API returned unexpected body: got %q, want %q", string(body), expected)
	}

	// 停止服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Stop(ctx); err != nil {
		t.Errorf("Failed to stop API server: %v", err)
	}

	// 验证服务器已停止 - 尝试连接应该失败
	_, err = client.Get("http://" + addr + "/api?key=test-key")
	if err == nil {
		t.Error("Server should be stopped, but connection succeeded")
	}
}

func TestAPIServer_HandleAPIRequest(t *testing.T) {
	// 创建检索函数，根据key返回不同的测试数据
	retrieveFunc := func(key string) ([]byte, error) {
		switch key {
		case "test-key":
			return []byte("test-value"), nil
		case "empty-key":
			return []byte{}, nil
		default:
			return nil, nil
		}
	}

	// 创建一个真实的Group实例
	group := cache.NewGroup("testGroup", 1024, cache.RetrieveFunc(retrieveFunc))

	// 创建API服务器
	addr := "localhost:18082"
	server := httptransport.NewAPIServer(group)

	// 启动服务器
	if err := server.Start(addr); err != nil {
		t.Fatalf("Failed to start API server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 创建一个自定义的HTTP客户端，设置短超时
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	// 测试用例
	tests := []struct {
		name       string
		queryParam string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid key",
			queryParam: "key=test-key",
			wantStatus: http.StatusOK,
			wantBody:   "test-value",
		},
		{
			name:       "empty key param",
			queryParam: "",
			wantStatus: http.StatusBadRequest,
			wantBody:   "missing key parameter",
		},
		{
			name:       "key with empty value",
			queryParam: "key=empty-key",
			wantStatus: http.StatusOK,
			wantBody:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "http://" + addr + "/api"
			if tt.queryParam != "" {
				url += "?" + tt.queryParam
			}

			resp, err := client.Get(url)
			if err != nil {
				t.Fatalf("Failed to call API: %v", err)
			}
			defer resp.Body.Close()

			// 检查响应状态
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("API returned unexpected status: got %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			// 验证响应内容
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response: %v", err)
			}

			if tt.wantStatus == http.StatusOK {
				if string(body) != tt.wantBody {
					t.Errorf("API returned unexpected body: got %q, want %q", string(body), tt.wantBody)
				}
			} else {
				// 对于错误情况，只检查响应是否包含期望的错误消息
				if !strings.Contains(string(body), tt.wantBody) {
					t.Errorf("API error does not contain expected message: got %q, should contain %q", string(body), tt.wantBody)
				}
			}
		})
	}
}
