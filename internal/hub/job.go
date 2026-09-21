package hub

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kevinyuzekai/ComputePool/internal/jobs"
)

const (
	ShardPending  = "pending"
	ShardAssigned = "assigned"
	ShardDone     = "done"
	ShardFailed   = "failed"

	JobPending    = "pending"
	JobRunning    = "running"
	JobDone       = "done"
	JobFailed     = "failed"
	JobCancelled  = "cancelled"
)

// Shard is one unit of work assigned to a single worker.
type Shard struct {
	JobID    string          `json:"jobId"`
	ShardID  string          `json:"shardId"`
	Type     string          `json:"type"`
	Payload  json.RawMessage `json:"payload"`
	Status   string          `json:"status"`
	WorkerID string          `json:"workerId,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
	Metrics  jobs.Metrics    `json:"metrics,omitempty"`
	Error    string          `json:"error,omitempty"`
	Assigned time.Time       `json:"assignedAt,omitempty"`
	Finished time.Time       `json:"finishedAt,omitempty"`
	LocalOnly bool           `json:"localOnly,omitempty"` // only Mac local workers may claim
}

// Assignment is what Poll returns to a worker.
type Assignment struct {
	JobID   string          `json:"jobId"`
	ShardID string          `json:"shardId"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Job groups shards for a benchmark or protocol test.
type Job struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Label       string    `json:"label"`
	Status      string    `json:"status"`
	Mode        string    `json:"mode"` // local | all | compare-phase
	CreatedAt   time.Time `json:"createdAt"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	FinishedAt  time.Time `json:"finishedAt,omitempty"`
	TotalShards int       `json:"totalShards"`
	DoneShards  int       `json:"doneShards"`
	WallMs      int64     `json:"wallMs,omitempty"`
	Shards      []*Shard  `json:"shards,omitempty"`

	// Benchmark meta
	TotalIterations int64   `json:"totalIterations,omitempty"`
	CompareOf       string  `json:"compareOf,omitempty"` // parent compare id
	Phase           string  `json:"phase,omitempty"`     // local | pool
}

// BenchmarkResult summarizes Mac-alone vs Mac+devices.
type BenchmarkResult struct {
	CompareID       string  `json:"compareId"`
	TotalIterations int64   `json:"totalIterations"`
	ShardCount      int     `json:"shardCount"`
	LocalJobID      string  `json:"localJobId"`
	PoolJobID       string  `json:"poolJobId"`
	LocalWallMs     int64   `json:"localWallMs"`
	PoolWallMs      int64   `json:"poolWallMs"`
	Speedup         float64 `json:"speedup"`
	LocalWorkers    int     `json:"localWorkersUsed"`
	RemoteWorkers   int     `json:"remoteWorkersUsed"`
	Status          string  `json:"status"`
	LocalJob        *Job    `json:"localJob,omitempty"`
	PoolJob         *Job    `json:"poolJob,omitempty"`
}

// ResultRequest matches POST /api/worker/result.
type ResultRequest struct {
	JobID    string          `json:"jobId"`
	ShardID  string          `json:"shardId"`
	WorkerID string          `json:"workerId"`
	Result   json.RawMessage `json:"result"`
	Metrics  jobs.Metrics    `json:"metrics"`
	Error    string          `json:"error,omitempty"`
}

// SubmitSimple enqueues echo/sleep/cpu_hash with optional local-only flag.
func (h *Hub) SubmitSimple(jobType string, payload json.RawMessage, shardCount int, localOnly bool, label string) (*Job, error) {
	if shardCount <= 0 {
		shardCount = 1
	}
	if shardCount > 256 {
		shardCount = 256
	}
	switch jobType {
	case jobs.TypeCPUHash, jobs.TypeEcho, jobs.TypeSleep:
	default:
		return nil, errBad("unsupported type")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	job := h.newJobLocked(jobType, label, "manual", localOnly)
	for i := 0; i < shardCount; i++ {
		pl := payload
		if jobType == jobs.TypeCPUHash && shardCount > 1 {
			// Split iterations evenly if payload has iterations.
			var p jobs.CPUHashPayload
			if err := json.Unmarshal(payload, &p); err == nil && p.Iterations > 0 {
				per := p.Iterations / int64(shardCount)
				if i == shardCount-1 {
					per = p.Iterations - per*int64(shardCount-1)
				}
				p.Seed = fmt.Sprintf("%s#%d", p.Seed, i)
				p.Iterations = per
				pl, _ = json.Marshal(p)
			}
		}
		s := h.newShardLocked(job, jobType, pl, localOnly)
		job.Shards = append(job.Shards, s)
		h.pending = append(h.pending, s)
	}
	job.TotalShards = len(job.Shards)
	job.Status = JobRunning
	job.StartedAt = time.Now()
	h.broadcastLocked()
	return cloneJob(job, true), nil
}

// BenchmarkOpts configures 算力对比.
type BenchmarkOpts struct {
	Iterations int64 `json:"iterations"`
	Shards     int   `json:"shards"`
}

// StartCompare runs Mac-local-only then Mac+all-devices with the same work.
func (h *Hub) StartCompare(opts BenchmarkOpts) (*BenchmarkResult, error) {
	if opts.Iterations <= 0 {
		opts.Iterations = 2_000_000
	}
	if opts.Shards <= 0 {
		opts.Shards = runtimeNumCPU() * 2
	}
	if opts.Shards < 2 {
		opts.Shards = 2
	}
	if opts.Shards > 128 {
		opts.Shards = 128
	}

	compareID := fmt.Sprintf("cmp-%d", h.jobSeq.Add(1))

	localJob, err := h.enqueueCPUHash(compareID, "local", "Mac 单机", opts.Iterations, opts.Shards, true)
	if err != nil {
		return nil, err
	}

	// Pool job is created but shards stay gated until local finishes —
	// simpler approach: create pool job after local done via WaitAndContinue.
	// For MVP we enqueue both; pool shards are marked localOnly=false but
	// we hold them until local completes using a "held" list.
	poolJob, err := h.enqueueCPUHashHeld(compareID, "pool", "Mac + 设备", opts.Iterations, opts.Shards)
	if err != nil {
		return nil, err
	}

	go h.runComparePipeline(compareID, localJob.ID, poolJob.ID)

	return &BenchmarkResult{
		CompareID:       compareID,
		TotalIterations: opts.Iterations,
		ShardCount:      opts.Shards,
		LocalJobID:      localJob.ID,
		PoolJobID:       poolJob.ID,
		Status:          "running",
	}, nil
}

func (h *Hub) enqueueCPUHash(compareID, phase, label string, iterations int64, shards int, localOnly bool) (*Job, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	job := h.newJobLocked(jobs.TypeCPUHash, label, phase, localOnly)
	job.CompareOf = compareID
	job.Phase = phase
	job.TotalIterations = iterations
	per := iterations / int64(shards)
	for i := 0; i < shards; i++ {
		n := per
		if i == shards-1 {
			n = iterations - per*int64(shards-1)
		}
		pl, _ := json.Marshal(jobs.CPUHashPayload{
			Seed:       fmt.Sprintf("%s-%s-%d", compareID, phase, i),
			Iterations: n,
		})
		s := h.newShardLocked(job, jobs.TypeCPUHash, pl, localOnly)
		job.Shards = append(job.Shards, s)
		h.pending = append(h.pending, s)
	}
	job.TotalShards = len(job.Shards)
	job.Status = JobRunning
	job.StartedAt = time.Now()
	h.broadcastLocked()
	return cloneJob(job, false), nil
}

func (h *Hub) enqueueCPUHashHeld(compareID, phase, label string, iterations int64, shards int) (*Job, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	job := h.newJobLocked(jobs.TypeCPUHash, label, phase, false)
	job.CompareOf = compareID
	job.Phase = phase
	job.TotalIterations = iterations
	job.Status = JobPending // not yet released
	per := iterations / int64(shards)
	for i := 0; i < shards; i++ {
		n := per
		if i == shards-1 {
			n = iterations - per*int64(shards-1)
		}
		pl, _ := json.Marshal(jobs.CPUHashPayload{
			Seed:       fmt.Sprintf("%s-%s-%d", compareID, phase, i),
			Iterations: n,
		})
		s := h.newShardLocked(job, jobs.TypeCPUHash, pl, false)
		job.Shards = append(job.Shards, s)
		// NOT added to pending yet
	}
	job.TotalShards = len(job.Shards)
	return cloneJob(job, false), nil
}

func (h *Hub) runComparePipeline(compareID, localID, poolID string) {
	// Wait for local job done
	for {
		h.mu.Lock()
		lj := h.jobs[localID]
		done := lj != nil && (lj.Status == JobDone || lj.Status == JobFailed)
		h.mu.Unlock()
		if done {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Release pool shards
	h.mu.Lock()
	pj := h.jobs[poolID]
	if pj != nil {
		pj.Status = JobRunning
		pj.StartedAt = time.Now()
		for _, s := range pj.Shards {
			if s.Status == ShardPending {
				h.pending = append(h.pending, s)
			}
		}
		h.broadcastLocked()
	}
	h.mu.Unlock()
	_ = compareID
}

// GetCompare builds a BenchmarkResult snapshot.
func (h *Hub) GetCompare(compareID string) *BenchmarkResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	var localJob, poolJob *Job
	for _, j := range h.jobs {
		if j.CompareOf != compareID {
			continue
		}
		if j.Phase == "local" {
			localJob = cloneJob(j, true)
		}
		if j.Phase == "pool" {
			poolJob = cloneJob(j, true)
		}
	}
	if localJob == nil && poolJob == nil {
		return nil
	}
	br := &BenchmarkResult{
		CompareID: compareID,
		Status:    "running",
		LocalJob:  localJob,
		PoolJob:   poolJob,
	}
	if localJob != nil {
		br.LocalJobID = localJob.ID
		br.LocalWallMs = localJob.WallMs
		br.TotalIterations = localJob.TotalIterations
		br.ShardCount = localJob.TotalShards
	}
	if poolJob != nil {
		br.PoolJobID = poolJob.ID
		br.PoolWallMs = poolJob.WallMs
	}
	remote := 0
	localN := 0
	now := time.Now()
	for _, w := range h.workers {
		if now.Sub(w.LastSeen) > WorkerStaleAfter {
			continue
		}
		if w.Local {
			localN++
		} else {
			remote++
		}
	}
	br.LocalWorkers = localN
	br.RemoteWorkers = remote
	if localJob != nil && poolJob != nil &&
		localJob.Status == JobDone && poolJob.Status == JobDone &&
		poolJob.WallMs > 0 {
		br.Speedup = float64(localJob.WallMs) / float64(poolJob.WallMs)
		br.Status = "done"
	} else if (localJob != nil && localJob.Status == JobFailed) ||
		(poolJob != nil && poolJob.Status == JobFailed) {
		br.Status = "failed"
	}
	return br
}

func (h *Hub) newJobLocked(jobType, label, mode string, _ bool) *Job {
	id := fmt.Sprintf("job-%d", h.jobSeq.Add(1))
	j := &Job{
		ID:        id,
		Type:      jobType,
		Label:     label,
		Status:    JobPending,
		Mode:      mode,
		CreatedAt: time.Now(),
		Shards:    nil,
	}
	h.jobs[id] = j
	return j
}

func (h *Hub) newShardLocked(job *Job, typ string, payload json.RawMessage, localOnly bool) *Shard {
	sid := fmt.Sprintf("s-%d", h.shardSeq.Add(1))
	return &Shard{
		JobID:     job.ID,
		ShardID:   sid,
		Type:      typ,
		Payload:   payload,
		Status:    ShardPending,
		LocalOnly: localOnly,
	}
}

// Poll blocks up to timeout waiting for a shard this worker may claim.
func (h *Hub) Poll(workerID string, timeout time.Duration) (*Assignment, error) {
	if workerID == "" {
		return nil, errBad("workerId required")
	}
	deadline := time.Now().Add(timeout)
	for {
		h.mu.Lock()
		w, ok := h.workers[workerID]
		if !ok {
			h.mu.Unlock()
			return nil, errBad("worker not registered")
		}
		w.LastSeen = time.Now()
		if a := h.tryAssignLocked(w); a != nil {
			h.mu.Unlock()
			return a, nil
		}
		if time.Now().After(deadline) {
			h.mu.Unlock()
			return nil, nil // no job
		}
		ch := make(chan struct{}, 1)
		h.waiters = append(h.waiters, ch)
		remain := time.Until(deadline)
		h.mu.Unlock()
		timer := time.NewTimer(remain)
		select {
		case <-ch:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (h *Hub) tryAssignLocked(w *Worker) *Assignment {
	if !h.running {
		return nil
	}
	for i, s := range h.pending {
		if s.Status != ShardPending {
			continue
		}
		if s.LocalOnly && !w.Local {
			continue
		}
		// Assign
		s.Status = ShardAssigned
		s.WorkerID = w.ID
		s.Assigned = time.Now()
		h.pending = append(h.pending[:i], h.pending[i+1:]...)
		return &Assignment{
			JobID:   s.JobID,
			ShardID: s.ShardID,
			Type:    s.Type,
			Payload: s.Payload,
		}
	}
	return nil
}

// SubmitResult records a finished shard.
func (h *Hub) SubmitResult(req ResultRequest) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	job, ok := h.jobs[req.JobID]
	if !ok {
		return errBad("unknown job")
	}
	var shard *Shard
	for _, s := range job.Shards {
		if s.ShardID == req.ShardID {
			shard = s
			break
		}
	}
	if shard == nil {
		return errBad("unknown shard")
	}
	if shard.Status == ShardDone {
		return nil
	}
	shard.Finished = time.Now()
	shard.WorkerID = req.WorkerID
	if req.Error != "" {
		shard.Status = ShardFailed
		shard.Error = req.Error
	} else {
		shard.Status = ShardDone
		shard.Result = req.Result
		shard.Metrics = req.Metrics
	}
	job.DoneShards++
	h.recordResult(req.WorkerID, req.Metrics.HashesPerSec)

	allDone := true
	failed := false
	for _, s := range job.Shards {
		if s.Status == ShardFailed {
			failed = true
		}
		if s.Status != ShardDone && s.Status != ShardFailed {
			allDone = false
		}
	}
	if allDone {
		job.FinishedAt = time.Now()
		if job.StartedAt.IsZero() {
			job.StartedAt = job.CreatedAt
		}
		job.WallMs = job.FinishedAt.Sub(job.StartedAt).Milliseconds()
		if job.WallMs <= 0 {
			job.WallMs = 1 // sub-ms jobs still countable for speedup
		}
		if failed {
			job.Status = JobFailed
		} else {
			job.Status = JobDone
		}
	}
	return nil
}

// GetJob returns a job snapshot.
func (h *Hub) GetJob(id string) *Job {
	h.mu.Lock()
	defer h.mu.Unlock()
	j, ok := h.jobs[id]
	if !ok {
		return nil
	}
	return cloneJob(j, true)
}

// ListJobs returns recent jobs (newest first, capped).
func (h *Hub) ListJobs(limit int) []Job {
	h.mu.Lock()
	defer h.mu.Unlock()
	if limit <= 0 {
		limit = 20
	}
	out := make([]Job, 0, len(h.jobs))
	for _, j := range h.jobs {
		out = append(out, *cloneJob(j, false))
	}
	// simple insertion by CreatedAt desc
	for i := 0; i < len(out); i++ {
		for k := i + 1; k < len(out); k++ {
			if out[k].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[k] = out[k], out[i]
			}
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func cloneJob(j *Job, withShards bool) *Job {
	cp := *j
	if withShards {
		shards := make([]*Shard, len(j.Shards))
		for i, s := range j.Shards {
			sc := *s
			shards[i] = &sc
		}
		cp.Shards = shards
	} else {
		cp.Shards = nil
	}
	return &cp
}

func runtimeNumCPU() int {
	return defaultCPU()
}
