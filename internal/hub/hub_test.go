package hub

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kevinyuzekai/ComputePool/internal/jobs"
)

func TestRegisterPollResult(t *testing.T) {
	h := New("test")
	h.Start()
	_, err := h.Register(RegisterRequest{ID: "w1", Name: "Mac", Platform: "darwin-local", Cores: 1, Local: true})
	if err != nil {
		t.Fatal(err)
	}
	pl, _ := json.Marshal(jobs.EchoPayload{Message: "hi"})
	job, err := h.SubmitSimple(jobs.TypeEcho, pl, 1, false, "t")
	if err != nil {
		t.Fatal(err)
	}
	asg, err := h.Poll("w1", time.Second)
	if err != nil || asg == nil {
		t.Fatalf("poll: %v %v", asg, err)
	}
	if asg.JobID != job.ID {
		t.Fatalf("job id")
	}
	res, _ := json.Marshal(map[string]string{"echo": "hi"})
	if err := h.SubmitResult(ResultRequest{
		JobID: asg.JobID, ShardID: asg.ShardID, WorkerID: "w1",
		Result: res, Metrics: jobs.Metrics{ElapsedMs: 1},
	}); err != nil {
		t.Fatal(err)
	}
	got := h.GetJob(job.ID)
	if got.Status != JobDone {
		t.Fatalf("status %s", got.Status)
	}
}

func TestComparePipeline(t *testing.T) {
	h := New("test")
	h.Start()
	_, _ = h.Register(RegisterRequest{ID: "local-0", Name: "L", Platform: "darwin-local", Cores: 1, Local: true})
	_, _ = h.Register(RegisterRequest{ID: "phone", Name: "P", Platform: "iOS", Cores: 2, Local: false})

	br, err := h.StartCompare(BenchmarkOpts{Iterations: 50_000, Shards: 4})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		for _, wid := range []string{"local-0", "phone"} {
			asg, _ := h.Poll(wid, 50*time.Millisecond)
			if asg == nil {
				continue
			}
			out, met, err := jobs.Run(asg.Type, asg.Payload)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if err := h.SubmitResult(ResultRequest{
				JobID: asg.JobID, ShardID: asg.ShardID, WorkerID: wid,
				Result: out, Metrics: met,
			}); err != nil {
				t.Fatalf("result: %v", err)
			}
		}
		snap := h.GetCompare(br.CompareID)
		if snap != nil && snap.Status == "done" {
			if snap.LocalWallMs <= 0 || snap.PoolWallMs <= 0 {
				t.Fatalf("wall times: %+v", snap)
			}
			return
		}
	}
	snap := h.GetCompare(br.CompareID)
	t.Fatalf("timeout compare: %+v", snap)
}
