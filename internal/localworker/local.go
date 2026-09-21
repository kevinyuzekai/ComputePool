package localworker

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/kevinyuzekai/ComputePool/internal/hub"
	"github.com/kevinyuzekai/ComputePool/internal/jobs"
)

// Pool runs N local Mac worker goroutines against the in-process hub.
type Pool struct {
	Hub   *hub.Hub
	Count int
	stop  chan struct{}
}

// Start registers and runs local workers. Count defaults to NumCPU.
func (p *Pool) Start() {
	if p.Count <= 0 {
		p.Count = runtime.NumCPU()
	}
	if p.Count < 1 {
		p.Count = 1
	}
	p.stop = make(chan struct{})
	p.Hub.SetLocalWorkerCount(p.Count)
	host, _ := os.Hostname()
	if host == "" {
		host = "Mac"
	}
	for i := 0; i < p.Count; i++ {
		id := fmt.Sprintf("local-%s-%d", host, i)
		name := fmt.Sprintf("%s · 本地 #%d", host, i+1)
		go p.loop(id, name)
	}
}

// Stop signals workers to exit (best-effort; in-flight shards finish).
func (p *Pool) Stop() {
	if p.stop != nil {
		close(p.stop)
	}
}

func (p *Pool) loop(id, name string) {
	_, _ = p.Hub.Register(hub.RegisterRequest{
		ID:       id,
		Name:     name,
		Platform: runtime.GOOS + "-local/" + runtime.GOARCH,
		Cores:    1,
		Local:    true,
	})
	for {
		select {
		case <-p.stop:
			return
		default:
		}
		asg, err := p.Hub.Poll(id, 2*time.Second)
		if err != nil || asg == nil {
			continue
		}
		result, metrics, runErr := jobs.Run(asg.Type, asg.Payload)
		req := hub.ResultRequest{
			JobID:    asg.JobID,
			ShardID:  asg.ShardID,
			WorkerID: id,
			Result:   result,
			Metrics:  metrics,
		}
		if runErr != nil {
			req.Error = runErr.Error()
		}
		_ = p.Hub.SubmitResult(req)
	}
}
