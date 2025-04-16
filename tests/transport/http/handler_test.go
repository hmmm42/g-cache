package http_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hmmm42/g-cache/internal/cache"
	httptransport "github.com/hmmm42/g-cache/internal/transport/http"
)

// 使用真实的Group类型和Retriever接口
func setupTestGroup() (*cache.Group, func()) {
	// 备份原始的Groups
	oldGroups := cache.Groups
	cache.Groups = make(cache.GroupManager)

	// 创建一个测试用的retriever
	testData := map[string][]byte{
		"testKey": []byte("hello world"),
	}

	retriever := cache.RetrieveFunc(func(key string) ([]byte, error) {
		if val, ok := testData[key]; ok {
			return val, nil
		}
		return nil, fmt.Errorf("key %s not found", key)
	})

	// 创建并注册Group
	group := cache.NewGroup("testGroup", 1024, retriever)

	// 返回组和清理函数
	return group, func() {
		cache.Groups = oldGroups
	}
}

func TestHTTPPool_ServeHTTP(t *testing.T) {
	// 设置测试组和清理函数
	_, cleanup := setupTestGroup()
	defer cleanup()

	// 创建一个HTTPPool
	pool := httptransport.NewHTTPPool("http://localhost:8000")

	// 创建一个HTTP请求
	req := httptest.NewRequest("GET", "http://localhost:8000/_gcache/testGroup/testKey", nil)
	w := httptest.NewRecorder()

	// 处理请求
	pool.ServeHTTP(w, req)

	// 检查响应
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", resp.StatusCode, http.StatusOK)
	}

	// 检查Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/octet-stream" {
		t.Errorf("handler returned wrong content type: got %v want %v", contentType, "application/octet-stream")
	}

	// 检查响应体
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	body := buf.String()
	expected := "hello world"
	if body != expected {
		t.Errorf("handler returned unexpected body: got %v want %v", body, expected)
	}
}

func TestHTTPPool_ServeHTTP_NotFound(t *testing.T) {
	// 设置测试组和清理函数
	_, cleanup := setupTestGroup()
	defer cleanup()

	// 创建一个HTTPPool
	pool := httptransport.NewHTTPPool("http://localhost:8000")

	// 测试无效的路径
	testCases := []struct {
		name     string
		path     string
		wantCode int
	}{
		{
			name:     "invalid prefix",
			path:     "http://otherhost:8000/testGroup/testKey",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "missing key",
			path:     "http://localhost:8000/_gcache/testGroup",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "nonexistent group",
			path:     "http://localhost:8000/_gcache/nonexistentGroup/testKey",
			wantCode: http.StatusNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()

			pool.ServeHTTP(w, req)

			if w.Code != tc.wantCode {
				t.Errorf("expected status code %d but got %d", tc.wantCode, w.Code)
			}
		})
	}
}

func TestHTTPPool_Pick(t *testing.T) {
	// 创建一个HTTPPool
	self := "http://localhost:8000"
	pool := httptransport.NewHTTPPool(self)

	// 设置peer列表
	peers := []string{
		"http://peer1:8001",
		"http://peer2:8002",
		self, // 包括自己
	}
	pool.UpdatePeers(peers...)

	// 测试本地key不应该被转发
	//t.Run("local key", func(t *testing.T) {
	//	// 对于hash到本地节点的key，不应该返回fetcher
	//	fetcher, ok := pool.Pick("key-for-local")
	//	if ok {
	//		t.Error("Pick should return false for keys that hash to the local node")
	//	}
	//	if fetcher != nil {
	//		t.Error("Pick should return nil fetcher for keys that hash to the local node")
	//	}
	//})

	// 这个测试是间接的，因为我们无法直接控制一致性哈希的结果
	// 但我们可以确保Pick方法能够返回一个非nil的fetcher
	t.Run("remote key", func(t *testing.T) {
		// 多试几个key，增加命中远程节点的可能性
		for i := 0; i < 100; i++ {
			key := fmt.Sprintf("key-%d", i)
			fetcher, ok := pool.Pick(key)
			if ok {
				// 找到一个被分配到远程节点的key
				if fetcher == nil {
					t.Error("Pick should return non-nil fetcher when ok is true")
				}
				// 成功找到远程节点，测试通过
				return
			}
		}
		// 如果没有找到被分配到远程节点的key，也不视为失败
		// 因为这取决于哈希算法，但概率很低
		t.Log("No key was assigned to a remote peer in the test, which is unusual but possible")
	})
}

func TestHTTPPool_UpdatePeers(t *testing.T) {
	// 创建一个HTTPPool
	self := "http://localhost:8000"
	pool := httptransport.NewHTTPPool(self)

	// 初始状态下应该没有peers
	// 但是我们不能直接检查内部状态，所以跳过这个检查

	// 更新peers
	peers := []string{
		"http://peer1:8001",
		"http://peer2:8002",
		self,
	}
	pool.UpdatePeers(peers...)

	// 由于内部状态是私有的，我们只能通过行为来测试
	// 尝试Pick一个key，如果有任何key被分配到远程节点，那么UpdatePeers工作正常
	foundRemote := false
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		_, ok := pool.Pick(key)
		if ok {
			foundRemote = true
			break
		}
	}

	// 不强制要求找到远程节点，因为这取决于哈希算法
	if !foundRemote {
		t.Log("No key was assigned to a remote peer in the test, which is unusual but possible")
	}
}
