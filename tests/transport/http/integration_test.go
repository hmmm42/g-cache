package http_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/hmmm42/g-cache/internal/cache"
	httptransport "github.com/hmmm42/g-cache/internal/transport/http"
)

// 集成测试 - 测试完整的分布式缓存功能
func TestIntegrationDistributedCache(t *testing.T) {
	// 如果在CI环境中运行，可能需要跳过此测试
	// t.Skip("Skipping integration test in CI environment")

	// 创建测试数据和检索函数
	testData := map[string][]byte{
		"key1": []byte("value1"),
		"key2": []byte("value2"),
		"key3": []byte("value3"),
	}

	retrieverFunc := cache.RetrieveFunc(func(key string) ([]byte, error) {
		if val, ok := testData[key]; ok {
			return val, nil
		}
		return nil, fmt.Errorf("key not found: %s", key)
	})

	// 创建多个节点的缓存
	port1 := 18090
	port2 := 18091
	addr1 := fmt.Sprintf("http://localhost:%d/_gcache", port1)
	addr2 := fmt.Sprintf("http://localhost:%d/_gcache", port2)

	// 使用NewGroupManager创建缓存组
	groupNames := []string{"testGroup"}
	groupManager := cache.NewGroupManager(groupNames, retrieverFunc)
	group := groupManager["testGroup"]

	if group == nil {
		t.Fatal("Failed to create cache group via GroupManager")
	}

	// 启动服务器
	server1 := httptransport.NewHTTPServer(addr1, []string{addr1, addr2}, group)
	server2 := httptransport.NewHTTPServer(addr2, []string{addr1, addr2}, group)

	if err := server1.Start(fmt.Sprintf("localhost:%d", port1)); err != nil {
		t.Fatalf("Failed to start server1: %v", err)
	}
	if err := server2.Start(fmt.Sprintf("localhost:%d", port2)); err != nil {
		t.Fatalf("Failed to start server2: %v", err)
	}

	// 延迟关闭服务器
	defer func() {
		ctx := context.Background()
		server1.Stop(ctx)
		server2.Stop(ctx)
	}()

	// 等待服务器启动
	time.Sleep(200 * time.Millisecond)

	// 创建一个HTTP客户端来测试
	client := &http.Client{Timeout: 2 * time.Second}

	// 尝试从不同的节点获取相同的key
	t.Run("fetch same key from both nodes", func(t *testing.T) {
		// 从第一个节点获取
		resp1, err := client.Get(fmt.Sprintf("%s/%s/key1", addr1, "testGroup"))
		if err != nil {
			t.Fatalf("Failed to get from server1: %v", err)
		}
		defer resp1.Body.Close()

		if resp1.StatusCode != http.StatusOK {
			t.Errorf("Server1 returned status %d, expected %d", resp1.StatusCode, http.StatusOK)
		}

		// 从第二个节点获取
		resp2, err := client.Get(fmt.Sprintf("%s/%s/key1", addr2, "testGroup"))
		if err != nil {
			t.Fatalf("Failed to get from server2: %v", err)
		}
		defer resp2.Body.Close()

		if resp2.StatusCode != http.StatusOK {
			t.Errorf("Server2 returned status %d, expected %d", resp2.StatusCode, http.StatusOK)
		}
	})

	// 测试缓存功能 - 多次获取同一个key应该使用缓存
	t.Run("cache hit test", func(t *testing.T) {
		// 第一次获取，这会从retriever中获取
		_, err := group.Get("key2")
		if err != nil {
			t.Fatalf("Failed to get key2: %v", err)
		}

		// 修改原始数据，模拟后端数据变化
		testData["key2"] = []byte("value2-modified")

		// 再次获取，应该返回缓存的值而不是修改后的值
		view, err := group.Get("key2")
		if err != nil {
			t.Fatalf("Failed to get key2 again: %v", err)
		}

		// 缓存应该返回原始值而不是修改后的值
		expected := "value2"
		if string(view.Bytes()) != expected {
			t.Errorf("Expected cached value %q, got %q", expected, string(view.Bytes()))
		}
	})

	// 测试从一个节点获取另一个节点的数据 (分布式特性)
	t.Run("cross-node fetching", func(t *testing.T) {
		// 这个测试比较复杂，因为我们无法控制一致性哈希将key映射到哪个节点
		// 我们可以间接测试两个节点是否能正常通信

		// 从第一个节点请求所有key
		for i := 1; i <= 3; i++ {
			key := fmt.Sprintf("key%d", i)
			resp, err := client.Get(fmt.Sprintf("%s/%s/%s", addr1, "testGroup", key))
			if err != nil || resp.StatusCode != http.StatusOK {
				t.Errorf("Failed to get %s from server1: %v, status: %d",
					key, err, resp.StatusCode)
			}
			resp.Body.Close()
		}

		// 从第二个节点请求所有key
		for i := 1; i <= 3; i++ {
			key := fmt.Sprintf("key%d", i)
			resp, err := client.Get(fmt.Sprintf("%s/%s/%s", addr2, "testGroup", key))
			if err != nil || resp.StatusCode != http.StatusOK {
				t.Errorf("Failed to get %s from server2: %v, status: %d",
					key, err, resp.StatusCode)
			}
			resp.Body.Close()
		}
	})
}

// 测试HTTP客户端和服务器的工作流程
func TestHTTPClientServerWorkflow(t *testing.T) {
	// 设置测试数据
	testData := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	// 创建检索函数
	testBytes := make(map[string][]byte)
	for k, v := range testData {
		testBytes[k] = []byte(v)
	}

	retrieverFunc := cache.RetrieveFunc(func(key string) ([]byte, error) {
		if val, ok := testBytes[key]; ok {
			return val, nil
		}
		return nil, fmt.Errorf("key not found: %s", key)
	})

	// 使用NewGroupManager创建缓存组
	port := 18092
	addr := fmt.Sprintf("http://localhost:%d/_gcache", port)
	groupNames := []string{"testGroup"}
	groupManager := cache.NewGroupManager(groupNames, retrieverFunc)
	group := groupManager["testGroup"]

	// 启动服务器 - 确保包含多个节点以避免自己向自己fetch数据
	server := httptransport.NewHTTPServer(
		addr,
		[]string{
			addr,
			"http://nonexistent-peer:8000", // 添加一个虚拟节点作为其他节点
		},
		group)

	if err := server.Start(fmt.Sprintf("localhost:%d", port)); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// 延迟关闭服务器
	defer func() {
		ctx := context.Background()
		server.Stop(ctx)
	}()

	// 等待服务器启动
	time.Sleep(200 * time.Millisecond)

	// 创建一个标准HTTP客户端用于直接测试HTTP接口
	client := &http.Client{Timeout: 2 * time.Second}

	// 测试HTTP接口
	for key, expected := range testData {
		// 通过标准HTTP请求获取数据
		resp, err := client.Get(fmt.Sprintf("%s/testGroup/%s", addr, key))
		if err != nil {
			t.Errorf("Failed to get %s via HTTP: %v", key, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("HTTP request returned status %d, expected %d", resp.StatusCode, http.StatusOK)
			continue
		}

		// 读取响应内容
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Errorf("Failed to read response body: %v", err)
			continue
		}

		if string(body) != expected {
			t.Errorf("For key %s, expected %q, got %q", key, expected, string(body))
		}
	}

	// 测试通过缓存接口直接获取数据 (不经过HTTP)
	for key, expected := range testData {
		// 直接从Group获取
		view, err := group.Get(key)
		if err != nil {
			t.Errorf("Failed to get %s directly from group: %v", key, err)
			continue
		}

		if string(view.Bytes()) != expected {
			t.Errorf("For key %s (direct access), expected %q, got %q", key, expected, string(view.Bytes()))
		}
	}

	// 测试获取不存在的数据
	resp, err := client.Get(fmt.Sprintf("%s/testGroup/nonexistent", addr))
	if err != nil {
		t.Errorf("Failed to make HTTP request for nonexistent key: %v", err)
	} else {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Error("Expected non-OK status for nonexistent key, got OK")
		}
	}
}

// 测试多个缓存组的独立性
func TestMultipleGroups(t *testing.T) {
	// 创建两个缓存组的名称
	groupNames := []string{"group1", "group2"}

	// 创建通用的检索函数，根据组名返回不同的值
	retrieverFunc := cache.RetrieveFunc(func(key string) ([]byte, error) {
		if key == "key1" {
			for _, groupName := range []string{"group1", "group2"} {
				if cache.GetGroup(groupName) != nil {
					// 这里简化处理，根据测试的请求路径来返回对应组的值
					if groupName == "group1" {
						return []byte("group1-value1"), nil
					} else if groupName == "group2" {
						return []byte("group2-value1"), nil
					}
				}
			}
		}
		return nil, fmt.Errorf("key not found: %s", key)
	})

	// 启动服务器
	port := 18093
	addr := fmt.Sprintf("http://localhost:%d", port)

	// 使用NewGroupManager创建多个缓存组
	groupManager := cache.NewGroupManager(groupNames, retrieverFunc)
	group1 := groupManager["group1"]

	// 这里使用group1初始化服务器，但两个组都应该可访问
	server := httptransport.NewHTTPServer(addr, []string{addr}, group1)

	if err := server.Start(fmt.Sprintf("localhost:%d", port)); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// 延迟关闭服务器
	defer func() {
		ctx := context.Background()
		server.Stop(ctx)
	}()

	// 等待服务器启动
	time.Sleep(200 * time.Millisecond)

	// 创建HTTP客户端
	client := &http.Client{Timeout: 2 * time.Second}

	// 从group1获取数据
	resp1, err := client.Get(fmt.Sprintf("%s/group1/key1", addr))
	if err != nil {
		t.Fatalf("Failed to get from group1: %v", err)
	}
	defer resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		t.Errorf("Group1 returned status %d, expected %d", resp1.StatusCode, http.StatusOK)
	}

	// 读取并检查数据
	body1, err := io.ReadAll(resp1.Body)
	if err != nil {
		t.Fatalf("Failed to read response from group1: %v", err)
	}
	if string(body1) != "group1-value1" {
		t.Errorf("Expected 'group1-value1', got %q", string(body1))
	}

	// 从group2获取数据
	resp2, err := client.Get(fmt.Sprintf("%s/group2/key1", addr))
	if err != nil {
		t.Fatalf("Failed to get from group2: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("Group2 returned status %d, expected %d", resp2.StatusCode, http.StatusOK)
	}

	// 读取并检查数据
	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("Failed to read response from group2: %v", err)
	}
	if string(body2) != "group2-value1" {
		t.Errorf("Expected 'group2-value1', got %q", string(body2))
	}
}
