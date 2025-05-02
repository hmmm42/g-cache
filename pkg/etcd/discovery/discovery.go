package discovery

import (
	"context"
	"log/slog"

	"github.com/hmmm42/g-cache/config"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"go.etcd.io/etcd/client/v3/naming/resolver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func Discover(c *clientv3.Client, service string) (*grpc.ClientConn, error) {
	etcdResolver, err := resolver.NewBuilder(c)
	if err != nil {
		return nil, err
	}

	return grpc.NewClient("etcd:///"+service,
		grpc.WithResolvers(etcdResolver),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
}

func ListServicePeers(serviceName string) ([]string, error) {
	cli, err := clientv3.New(config.DefaultEtcdConfig)
	if err != nil {
		slog.Error("create etcd client failed", "error", err)
		return []string{}, err
	}

	endpointsManager, err := endpoints.NewManager(cli, serviceName)
	if err != nil {
		slog.Error("create endpoints manager failed", "error", err)
		return []string{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.DefaultEtcdConfig.DialTimeout)
	defer cancel()

	keyToEndpoint, err := endpointsManager.List(ctx)
	if err != nil {
		slog.Error("list endpoint nodes for target service failed", "error", err)
		return []string{}, err
	}

	var peersAddr []string
	for key, endpoint := range keyToEndpoint {
		peersAddr = append(peersAddr, endpoint.Addr)
		slog.Info("found endpoint",
			"key", key,
			"endpoint", endpoint,
			"metadata", endpoint.Metadata)
	}

	return peersAddr, nil
}

func DynamicServices(update chan struct{}, service string) {
	cli, err := clientv3.New(config.DefaultEtcdConfig)
	if err != nil {
		slog.Error("create etcd client failed", "error", err)
		return
	}
	defer cli.Close()

	watchChan := cli.Watch(context.Background(), service, clientv3.WithPrefix())
	for watchResp := range watchChan {
		for _, event := range watchResp.Events {
			switch event.Type {
			case clientv3.EventTypePut:
				update <- struct{}{}
				slog.Warn("service endpoint added or updated", "key", string(event.Kv.Key), "value", string(event.Kv.Value))
			case clientv3.EventTypeDelete:
				update <- struct{}{}
				slog.Warn("service endpoint deleted", "key", string(event.Kv.Key), "value", string(event.Kv.Value))
			}
		}
	}
}
