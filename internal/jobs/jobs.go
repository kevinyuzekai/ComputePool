package jobs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Types supported by workers (Mac local + iOS).
const (
	TypeCPUHash = "cpu_hash"
	TypeEcho    = "echo"
	TypeSleep   = "sleep"
)

// CPUHashPayload asks the worker to run Iterations of SHA-256 starting from Seed.
type CPUHashPayload struct {
	Seed       string `json:"seed"`
	Iterations int64  `json:"iterations"`
}

// CPUHashResult is returned by cpu_hash shards.
type CPUHashResult struct {
	Digest     string `json:"digest"`
	Iterations int64  `json:"iterations"`
	ElapsedMs  int64  `json:"elapsedMs"`
}

// EchoPayload / SleepPayload for protocol tests.
type EchoPayload struct {
	Message string `json:"message"`
}

type SleepPayload struct {
	Ms int64 `json:"ms"`
}

// Metrics carries approximate throughput hints.
type Metrics struct {
	ElapsedMs    int64   `json:"elapsedMs"`
	HashesPerSec float64 `json:"hashesPerSec,omitempty"`
}

// Run executes a job type locally (used by Mac workers and unit tests).
func Run(jobType string, payload json.RawMessage) (json.RawMessage, Metrics, error) {
	switch jobType {
	case TypeCPUHash:
		var p CPUHashPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, Metrics{}, err
		}
		if p.Iterations <= 0 {
			p.Iterations = 1
		}
		res, m := runCPUHash(p)
		b, _ := json.Marshal(res)
		return b, m, nil
	case TypeEcho:
		var p EchoPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, Metrics{}, err
		}
		b, _ := json.Marshal(map[string]string{"echo": p.Message})
		return b, Metrics{ElapsedMs: 0}, nil
	case TypeSleep:
		var p SleepPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, Metrics{}, err
		}
		if p.Ms < 0 {
			p.Ms = 0
		}
		if p.Ms > 60_000 {
			p.Ms = 60_000
		}
		start := time.Now()
		time.Sleep(time.Duration(p.Ms) * time.Millisecond)
		elapsed := time.Since(start).Milliseconds()
		b, _ := json.Marshal(map[string]int64{"sleptMs": elapsed})
		return b, Metrics{ElapsedMs: elapsed}, nil
	default:
		return nil, Metrics{}, fmt.Errorf("unknown job type %q", jobType)
	}
}

func runCPUHash(p CPUHashPayload) (CPUHashResult, Metrics) {
	start := time.Now()
	h := sha256.Sum256([]byte(p.Seed))
	buf := h[:]
	var next [32]byte
	for i := int64(0); i < p.Iterations; i++ {
		next = sha256.Sum256(buf)
		buf = next[:]
	}
	elapsed := time.Since(start)
	ms := elapsed.Milliseconds()
	if ms <= 0 {
		ms = 1
	}
	hps := float64(p.Iterations) / elapsed.Seconds()
	return CPUHashResult{
		Digest:     hex.EncodeToString(buf),
		Iterations: p.Iterations,
		ElapsedMs:  ms,
	}, Metrics{ElapsedMs: ms, HashesPerSec: hps}
}
