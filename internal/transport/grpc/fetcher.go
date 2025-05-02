package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	pb "github.com/hmmm42/g-cache/api/genproto/groupcachepb"
	"github.com/hmmm42/g-cache/pkg/etcd/discovery"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Client struct {
	serviceName string
	conn        *clientv3.Client
	mu          sync.RWMutex // 保护连接状态
}

func NewClient(serviceName string) *Client {
	cli, err := clientv3.NewFromURL("http://localhost:2379")
	if err != nil {
		return nil
	}
	return &Client{
		serviceName: serviceName,
		conn:        cli,
	}
}

func (c *Client) Fetch(group string, key string) ([]byte, error) {
	conn, err := discovery.Discover(c.conn, c.serviceName)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := pb.NewGroupCacheClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := client.Get(ctx, &pb.GetRequest{
		Group: group,
		Key:   key,
	})
	if err != nil {
		return nil, fmt.Errorf("could not get %s/%s from peer %s", group, key, c.serviceName)
	}

	slog.Debug("grpc call", "duration", time.Since(start).Milliseconds(), "peer", c.serviceName, "key", key)
	return resp.Value, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
