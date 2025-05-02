package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	pb "github.com/hmmm42/g-cache/api/genproto/groupcachepb"
	"github.com/hmmm42/g-cache/internal/cache"
	"github.com/hmmm42/g-cache/internal/cache/picker"
	"github.com/hmmm42/g-cache/pkg/validate"
)

var (
	defaultAddr     = "127.0.0.1:9999"
	defaultReplicas = 50
	serviceName     = "GroupCache"
)

type Server struct {
	pb.UnimplementedGroupCacheServer

	addr        string
	isRunning   bool
	stopSignal  chan error
	updateChan  chan struct{}
	mu          sync.RWMutex
	consistHash *picker.Map
	clients     map[string]*Client
}

func NewServer(update chan struct{}, addr string) (*Server, error) {
	if addr == "" {
		addr = defaultAddr
	}
	if !validate.ValidPeerAddr(addr) {
		return nil, fmt.Errorf("invalid peer address: %s", addr)
	}
	return &Server{addr: addr, updateChan: update}, nil
}

func (s *Server) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	group, key := req.GetGroup(), req.GetKey()
	resp := &pb.GetResponse{}
	slog.Info("received RPC Get request", "server", s.addr, "group", group, "key", key)

	if key == "" || group == "" {
		return resp, fmt.Errorf("key and group are required")
	}

	g := cache.GetGroup(group)
	if g == nil {
		return resp, fmt.Errorf("group %s not found", group)
	}

	value, err := g.Get(key)
	if err != nil {
		return resp, err
	}

	resp.Value = value.Bytes()
	return resp, nil
}

func (s *Server) SetPeers(peersAddr []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(peersAddr) == 0 {
		peersAddr = []string{s.addr}
	}

	s.consistHash = picker.New(defaultReplicas, nil)
	s.consistHash.Add(peersAddr...)
	s.clients = make(map[string]*Client)

	for _, addr := range peersAddr {
		if !validate.ValidPeerAddr(addr) {
			s.mu.Unlock()
			panic(fmt.Sprintf("invalid peer address: %s", addr))
		}
		s.clients[addr] = NewClient("GroupCache")
	}

	go func() {
		for {
			select {
			case <-s.updateChan:
				s.reconstruct()
			case <-s.stopSignal:
				if err := s.Stop(); err != nil {
					slog.Error("failed to stop server", "error", err)
				}
				return
			default:
				time.Sleep(2 * time.Second)
			}
		}
	}()
}

func (s *Server) reconstruct() {

}

func (s *Server) Pick(key string) (cache.Fetcher, bool) {
	//TODO implement me
	panic("implement me")
}

func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return nil
	}
	close(s.stopSignal)
	s.isRunning = false
	s.cleanUp()
	slog.Info("server stopped", "server", s.addr)
	return nil
}

func (s *Server) cleanUp() {
	for k := range s.clients {
		delete(s.clients, k)
	}
	s.clients = nil
	s.consistHash = nil
}
