package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	pb "github.com/hmmm42/g-cache/api/genproto/groupcachepb"
	"github.com/hmmm42/g-cache/internal/cache"
	"github.com/hmmm42/g-cache/internal/cache/picker"
	"github.com/hmmm42/g-cache/pkg/etcd/discovery"
	"github.com/hmmm42/g-cache/pkg/validate"
	"google.golang.org/grpc"
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
					slog.Error("stop server failed", "error", err)
				}
				return
			default:
				time.Sleep(2 * time.Second)
			}
		}
	}()
}

func (s *Server) reconstruct() {
	serviceList, err := discovery.ListServicePeers("GroupCache")
	if err != nil {
		slog.Error("list service peers failed", "error", err)
		return
	}

	newClients := make(map[string]*Client)
	newHash := picker.New(defaultReplicas, nil)
	newHash.Add(serviceList...)

	s.mu.RLock()
	for _, addr := range serviceList {
		if !validate.ValidPeerAddr(addr) {
			s.mu.RUnlock()
			panic(fmt.Sprintf("invalid peer address: %s", addr))
		}

		if client, exists := s.clients[addr]; exists {
			newClients[addr] = client
		} else {
			newClients[addr] = NewClient("GroupCache")
		}
		s.mu.RUnlock()
	}

	s.mu.Lock()
	oldClients := s.clients
	s.clients = newClients
	s.consistHash = newHash
	s.mu.Unlock()

	for addr, client := range oldClients {
		if _, ok := newClients[addr]; !ok {
			_ = client.Close()
		}
	}
	slog.Info("hash ring reconstruct", "service_list", serviceList)
}

func (s *Server) Pick(key string) (cache.Fetcher, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peerAddr := s.consistHash.Get(key)
	if peerAddr == "" {
		slog.Warn("hash ring not initialized yet", "local_key", key)
		return nil, false
	}

	if peerAddr == s.addr {
		slog.Debug("key is mapped to current node local", "key", key, "peer", peerAddr)
		return nil, false
	}

	slog.Debug("key is mapped to current node", "key", key, "peer", peerAddr)
	return s.clients[peerAddr], true
}

func (s *Server) Start() error {
	if err := s.initServer(); err != nil {
		return fmt.Errorf("init server failed: %w", err)
	}

	lis, err := s.setupListener()
	if err != nil {
		return fmt.Errorf("setup listener failed: %w", err)
	}

	grpcServer := s.setupGRPCServer()

	errChan := make(chan error, 1)
	go func() {
		defer func() {
			if s != nil {
				if err := s.Stop(); err != nil {
					slog.Error("stop server failed", "error", err)
				}
			}
		}()
		err := s.registerService(lis, errChan)
		if err != nil {
			errChan <- err
		}
	}()

	if err := s.serveRequests(grpcServer, lis); err != nil {
		return fmt.Errorf("serve requests failed: %w", err)
	}
	return nil
}

func (s *Server) initServer() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("server %s is already running", s.addr)
	}

	s.isRunning = true
	s.stopSignal = make(chan error)
	return nil
}

func (s *Server) setupListener() (net.Listener, error) {
	port := strings.Split(s.addr, ":")[1]
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, fmt.Errorf("listen %s failed: %v", s.addr, err)
	}
	return lis, nil
}

func (s *Server) setupGRPCServer() *grpc.Server {
	grpcServer := grpc.NewServer()
	pb.RegisterGroupCacheServer(grpcServer, s)
	return grpcServer
}

func (s *Server) registerService(lis net.Listener, errChan chan error) error {
	defer func() {
		if err := lis.Close(); err != nil {
			slog.Error("close grpc server failed", "error", err)
		}
	}()

	err := discovery.Register(serviceName, s.addr, s.stopSignal)
	if err != nil {
		slog.Error("register service failed", "error", err)
		errChan <- err
		return err
	}

	<-s.stopSignal
	slog.Info("service unregistered", "addr", s.addr)
	return nil
}

func (s *Server) serveRequests(grpcServer *grpc.Server, lis net.Listener) error {
	if err := grpcServer.Serve(lis); err != nil && s.isRunning {
		return fmt.Errorf("failed to serve on %s: %v", s.addr, err)
	}
	return nil
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
