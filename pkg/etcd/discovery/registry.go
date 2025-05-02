package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hmmm42/g-cache/config"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
)

func Register(service string, addr string, stop chan error) error {
	cli, err := clientv3.New(config.DefaultEtcdConfig)
	if err != nil {
		panic(err)
	}

	leaseGrantResp, err := cli.Grant(context.Background(), int64(config.Conf.Etcd.TTL))
	if err != nil {
		return fmt.Errorf("grant creates a new lease failed: %v", err)
	}

	leaseID := leaseGrantResp.ID

	err = etcdAddEndpoint(cli, leaseID, service, addr)
	if err != nil {
		return fmt.Errorf("add endpoint failed: %v", err)
	}

	alive, err := cli.KeepAlive(context.Background(), leaseID)
	if err != nil {
		return fmt.Errorf("keep alive failed: %v", err)
	}

	slog.Debug("registered service success", "service", service, "addr", addr)

	for {
		select {
		case <-stop:
			return err
		case <-cli.Ctx().Done():
			return fmt.Errorf("etcd client connect broken")
		case _, ok := <-alive:
			if !ok {
				slog.Error("keepalive channel closed, revoke given lease")
				if err := etcdDelEndpoint(cli, service, addr); err != nil {
					slog.Error("delete endpoint failed", "error", err)
				}
				return fmt.Errorf("keepalive channel closed, revoke given lease")
			}
		default:
			time.Sleep(2 * time.Second)
		}
	}
}

func etcdAddEndpoint(client *clientv3.Client, leaseID clientv3.LeaseID, service string, addr string) error {
	endpointManager, err := endpoints.NewManager(client, service)
	if err != nil {
		return err
	}

	metadata := endpoints.Endpoint{
		Addr:     addr,
		Metadata: "weight:10;version:v1.0.0",
	}

	return endpointManager.AddEndpoint(client.Ctx(),
		fmt.Sprintf("%s/%s", service, addr),
		metadata,
		clientv3.WithLease(leaseID))
}

func etcdDelEndpoint(client *clientv3.Client, service string, addr string) error {
	endpointManager, err := endpoints.NewManager(client, service)
	if err != nil {
		return err
	}

	return endpointManager.DeleteEndpoint(client.Ctx(),
		fmt.Sprintf("%s/%s", service, addr))
}
