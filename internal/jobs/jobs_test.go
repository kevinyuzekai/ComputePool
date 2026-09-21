package jobs

import (
	"encoding/json"
	"testing"
)

func TestCPUHashDeterministic(t *testing.T) {
	p, _ := json.Marshal(CPUHashPayload{Seed: "hello", Iterations: 1000})
	a, ma, err := Run(TypeCPUHash, p)
	if err != nil {
		t.Fatal(err)
	}
	b, mb, err := Run(TypeCPUHash, p)
	if err != nil {
		t.Fatal(err)
	}
	var ra, rb CPUHashResult
	_ = json.Unmarshal(a, &ra)
	_ = json.Unmarshal(b, &rb)
	if ra.Digest != rb.Digest {
		t.Fatalf("digest mismatch %s vs %s", ra.Digest, rb.Digest)
	}
	if ma.HashesPerSec <= 0 || mb.HashesPerSec <= 0 {
		t.Fatalf("expected positive throughput")
	}
}

func TestEchoSleep(t *testing.T) {
	ep, _ := json.Marshal(EchoPayload{Message: "hi"})
	out, _, err := Run(TypeEcho, ep)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	_ = json.Unmarshal(out, &m)
	if m["echo"] != "hi" {
		t.Fatalf("got %v", m)
	}
	sp, _ := json.Marshal(SleepPayload{Ms: 20})
	_, met, err := Run(TypeSleep, sp)
	if err != nil {
		t.Fatal(err)
	}
	if met.ElapsedMs < 15 {
		t.Fatalf("sleep too short: %d", met.ElapsedMs)
	}
}
