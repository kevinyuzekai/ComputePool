package hub

import (
	"fmt"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kevinyuzekai/ComputePool/internal/netutil"
)

const (
	WorkerStaleAfter = 45 * time.Second
	DefaultListen    = "0.0.0.0:9797"
	DefaultPort      = 9797
)

// Hub is the LAN orchestrator: worker registry + job/shard queue.
type Hub struct {
	mu sync.Mutex

	listenAddr string
	version    string
	running    bool
	startedAt  time.Time

	workers map[string]*Worker
	jobs    map[string]*Job
	pending []*Shard // FIFO of unassigned shards

	jobSeq   atomic.Uint64
	shardSeq atomic.Uint64

	waiters []chan struct{}

	localWorkers int
}

// New creates a stopped hub.
func New(version string) *Hub {
	return &Hub{
		listenAddr: DefaultListen,
		version:    version,
		workers:    make(map[string]*Worker),
		jobs:       make(map[string]*Job),
	}
}

func (h *Hub) SetListenAddr(addr string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if addr != "" {
		h.listenAddr = addr
	}
}

func (h *Hub) ListenAddr() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.listenAddr
}

func (h *Hub) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.running {
		return
	}
	h.running = true
	h.startedAt = time.Now()
}

func (h *Hub) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.running = false
	h.broadcastLocked()
}

func (h *Hub) Running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.running
}

// Info is returned by /api/status and /api/hub/info.
type Info struct {
	Version       string    `json:"version"`
	Running       bool      `json:"running"`
	ListenAddr    string    `json:"listenAddr"`
	JoinURL       string    `json:"joinURL"`
	LANIP         string    `json:"lanIP"`
	Port          int       `json:"port"`
	StartedAt     time.Time `json:"startedAt,omitempty"`
	WorkerCount   int       `json:"workerCount"`
	OnlineCount   int       `json:"onlineCount"`
	LocalWorkers  int       `json:"localWorkers"`
	PendingShards int       `json:"pendingShards"`
	HostnameHint  string    `json:"hostnameHint"`
	ServiceType   string    `json:"serviceType"`
}

func (h *Hub) Info() Info {
	h.mu.Lock()
	defer h.mu.Unlock()
	lan := netutil.PrimaryLANIPv4()
	port := DefaultPort
	if _, p, err := net.SplitHostPort(h.listenAddr); err == nil {
		if v, e := parsePort(p); e == nil {
			port = v
		}
	}
	online := 0
	now := time.Now()
	for _, w := range h.workers {
		if now.Sub(w.LastSeen) <= WorkerStaleAfter {
			online++
		}
	}
	return Info{
		Version:       h.version,
		Running:       h.running,
		ListenAddr:    h.listenAddr,
		JoinURL:       fmt.Sprintf("http://%s:%d", lan, port),
		LANIP:         lan,
		Port:          port,
		StartedAt:     h.startedAt,
		WorkerCount:   len(h.workers),
		OnlineCount:   online,
		LocalWorkers:  h.localWorkers,
		PendingShards: len(h.pending),
		HostnameHint:  runtime.GOOS + "/" + runtime.GOARCH,
		ServiceType:   "_computepool._tcp",
	}
}

func (h *Hub) SetLocalWorkerCount(n int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.localWorkers = n
}

func (h *Hub) broadcastLocked() {
	for _, ch := range h.waiters {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	h.waiters = nil
}

func (h *Hub) notify() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.broadcastLocked()
}

func parsePort(s string) (int, error) {
	var p int
	_, err := fmt.Sscanf(s, "%d", &p)
	return p, err
}
