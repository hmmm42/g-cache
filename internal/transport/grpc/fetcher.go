package grpc

import (
	"sync"

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
	return nil, nil
}
