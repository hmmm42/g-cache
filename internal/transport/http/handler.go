package http

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/hmmm42/g-cache/internal/cache"
	"github.com/hmmm42/g-cache/internal/cache/picker"
)

// TODO: 使用配置
const (
	defaultBasePath = "/_gcache/"
	defaultReplicas = 50
)

type HTTPPool struct {
	self         string
	basePath     string
	mu           sync.Mutex
	peerSelector *picker.Map
	fetcherMap   map[string]*HTTPFetcher
}

func NewHTTPPool(self string) *HTTPPool {
	return &HTTPPool{
		self:     self,
		basePath: defaultBasePath,
	}
}

func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, p.basePath) {
		http.Error(w, "invalid cache endpoint", http.StatusNotFound)
		return
	}

	p.Log("%s, %s", r.Method, r.URL.Path)

	parts := strings.SplitN(r.URL.Path[len(p.basePath):], "/", 2)
	if len(parts) != 2 {
		http.Error(w, "bad request format, expected /<basepath>/<groupname>/<key>", http.StatusBadRequest)
		return
	}

	groupName, key := parts[0], parts[1]
	group := cache.GetGroup(groupName)
	if group == nil {
		http.Error(w, fmt.Sprintf("no group found: %s", groupName), http.StatusNotFound)
		return
	}
	if key == "" {
		http.Error(w, "key cannot be empty", http.StatusBadRequest)
		return
	}

	view, err := group.Get(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	if _, err = w.Write(view.Bytes()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (p *HTTPPool) Pick(key string) (cache.Fetcher, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	peerAddr := p.peerSelector.Get(key)
	if peerAddr == p.self {
		return nil, false
	}
	log.Printf("[request forward by peer %s], pick remote peer: %s", p.self, peerAddr)
	return p.fetcherMap[peerAddr], true
}

func (p *HTTPPool) UpdatePeers(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.peerSelector = picker.New(defaultReplicas, nil)
	p.peerSelector.Add(peers...)
	p.fetcherMap = make(map[string]*HTTPFetcher, len(peers))

	for _, peer := range peers {
		p.fetcherMap[peer] = &HTTPFetcher{
			baseURL: peer + p.basePath, // such "http://10.0.0.1:9999/_ggcache/"
		}
	}
}

func (p *HTTPPool) Log(format string, v ...any) {
	log.Printf("[Server %s] %s", p.self, fmt.Sprintf(format, v...))
}
