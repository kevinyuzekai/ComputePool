package hub

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevinyuzekai/ComputePool/internal/jobs"
)

func writePNG(path string, w, h int) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	_ = os.WriteFile(path, buf.Bytes(), 0o644)
}

func TestImageBatchLocalWorkers(t *testing.T) {
	tmp := t.TempDir()
	inbox := filepath.Join(tmp, "in")
	outbox := filepath.Join(tmp, "out")
	_ = os.MkdirAll(inbox, 0o755)

	writePNG(filepath.Join(inbox, "a.png"), 400, 300)
	writePNG(filepath.Join(inbox, "b.png"), 100, 100)

	h := New("test")
	if err := h.SetDirs(inbox, outbox); err != nil {
		t.Fatal(err)
	}
	h.Start()
	_, _ = h.Register(RegisterRequest{ID: "local-0", Name: "L", Platform: "test", Cores: 1, Local: true})

	job, err := h.SubmitImageBatch(ImageBatchOpts{MaxEdge: 200, Quality: 80, Format: "jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if job.TotalShards != 2 {
		t.Fatalf("shards %d", job.TotalShards)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		asg, _ := h.Poll("local-0", 50*time.Millisecond)
		if asg == nil {
			got := h.GetJob(job.ID)
			if got != nil && (got.Status == JobDone || got.Status == JobFailed) {
				break
			}
			continue
		}
		out, met, err := jobs.Run(asg.Type, asg.Payload)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if err := h.SubmitResult(ResultRequest{
			JobID: asg.JobID, ShardID: asg.ShardID, WorkerID: "local-0",
			Result: out, Metrics: met,
		}); err != nil {
			t.Fatal(err)
		}
	}
	got := h.GetJob(job.ID)
	if got == nil || got.Status != JobDone {
		b, _ := json.Marshal(got)
		t.Fatalf("job not done: %s", b)
	}
	ents, err := os.ReadDir(got.OutDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 2 {
		t.Fatalf("outbox files %d", len(ents))
	}
}
