package hub

import (
	"time"
)

// Worker is a registered compute node (Mac local goroutine or remote device).
type Worker struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Platform     string    `json:"platform"`
	Cores        int       `json:"cores"`
	Local        bool      `json:"local"`
	LastSeen     time.Time `json:"lastSeen"`
	JobsDone     int64     `json:"jobsDone"`
	Throughput   float64   `json:"throughput"` // approx hashes/sec EMA
	RegisteredAt time.Time `json:"registeredAt"`
}

// RegisterRequest matches POST /api/worker/register.
type RegisterRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Cores    int    `json:"cores"`
	Local    bool   `json:"local"`
}

// Register upserts a worker and returns its public view.
func (h *Hub) Register(req RegisterRequest) (*Worker, error) {
	if req.ID == "" {
		return nil, errBad("id required")
	}
	if req.Name == "" {
		req.Name = req.ID
	}
	if req.Platform == "" {
		req.Platform = "unknown"
	}
	if req.Cores <= 0 {
		req.Cores = 1
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	w, ok := h.workers[req.ID]
	if !ok {
		w = &Worker{
			ID:           req.ID,
			RegisteredAt: now,
		}
		h.workers[req.ID] = w
	}
	w.Name = req.Name
	w.Platform = req.Platform
	w.Cores = req.Cores
	w.Local = req.Local
	w.LastSeen = now
	cp := *w
	return &cp, nil
}

// ListWorkers returns a snapshot of all workers (online first).
func (h *Hub) ListWorkers() []Worker {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	out := make([]Worker, 0, len(h.workers))
	for _, w := range h.workers {
		cp := *w
		out = append(out, cp)
		_ = now
	}
	return out
}

func (h *Hub) touchWorker(id string) {
	if w, ok := h.workers[id]; ok {
		w.LastSeen = time.Now()
	}
}

func (h *Hub) recordResult(workerID string, hashesPerSec float64) {
	w, ok := h.workers[workerID]
	if !ok {
		return
	}
	w.JobsDone++
	w.LastSeen = time.Now()
	if hashesPerSec > 0 {
		if w.Throughput <= 0 {
			w.Throughput = hashesPerSec
		} else {
			w.Throughput = w.Throughput*0.7 + hashesPerSec*0.3
		}
	}
}

type hubError struct{ msg string }

func (e hubError) Error() string { return e.msg }

func errBad(msg string) error { return hubError{msg: msg} }
