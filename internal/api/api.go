package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/kevinyuzekai/ComputePool/internal/hub"
)

// Handler serves JSON API for UI + workers.
type Handler struct {
	Hub *hub.Hub
}

func (h *Handler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("/api/status", h.status)
	mux.HandleFunc("/api/hub/info", h.status)
	mux.HandleFunc("/api/hub/start", h.start)
	mux.HandleFunc("/api/hub/stop", h.stop)
	mux.HandleFunc("/api/workers", h.workers)
	mux.HandleFunc("/api/jobs", h.jobs)
	mux.HandleFunc("/api/jobs/", h.jobSub)
	mux.HandleFunc("/api/benchmark", h.benchmark)
	mux.HandleFunc("/api/benchmark/", h.benchmarkSub)
	mux.HandleFunc("/api/worker/register", h.workerRegister)
	mux.HandleFunc("/api/worker/poll", h.workerPoll)
	mux.HandleFunc("/api/worker/result", h.workerResult)
	mux.HandleFunc("/api/test/echo", h.testEcho)
	mux.HandleFunc("/api/test/sleep", h.testSleep)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, h.Hub.Info())
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	h.Hub.Start()
	writeJSON(w, 200, h.Hub.Info())
}

func (h *Handler) stop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	h.Hub.Stop()
	writeJSON(w, 200, h.Hub.Info())
}

func (h *Handler) workers(w http.ResponseWriter, r *http.Request) {
	list := h.Hub.ListWorkers()
	writeJSON(w, 200, map[string]any{"workers": list})
}

func (h *Handler) jobs(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var body struct {
			Type      string          `json:"type"`
			Payload   json.RawMessage `json:"payload"`
			Shards    int             `json:"shards"`
			LocalOnly bool            `json:"localOnly"`
			Label     string          `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON body: " + err.Error()})
			return
		}
		if body.Type == "" {
			writeJSON(w, 400, map[string]string{"error": "type is required (cpu_hash | echo | sleep)"})
			return
		}
		job, err := h.Hub.SubmitSimple(body.Type, body.Payload, body.Shards, body.LocalOnly, body.Label)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, job)
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "GET or POST only"})
		return
	}
	writeJSON(w, 200, map[string]any{"jobs": h.Hub.ListJobs(30)})
}

func (h *Handler) jobSub(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/jobs/"):]
	if id == "" {
		writeJSON(w, 404, map[string]string{"error": "missing id"})
		return
	}
	job := h.Hub.GetJob(id)
	if job == nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, 200, job)
}

func (h *Handler) benchmark(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	var opts hub.BenchmarkOpts
	_ = json.NewDecoder(r.Body).Decode(&opts)
	res, err := h.Hub.StartCompare(opts)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, res)
}

func (h *Handler) benchmarkSub(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/benchmark/"):]
	res := h.Hub.GetCompare(id)
	if res == nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, 200, res)
}

func (h *Handler) workerRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	var req hub.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	wk, err := h.Hub.Register(req)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, wk)
}

func (h *Handler) workerPoll(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	id := r.URL.Query().Get("id")
	if id == "" {
		id = r.URL.Query().Get("workerId")
	}
	timeoutSec := 25
	if t := r.URL.Query().Get("timeout"); t != "" {
		if v, err := strconv.Atoi(t); err == nil && v > 0 && v <= 60 {
			timeoutSec = v
		}
	}
	asg, err := h.Hub.Poll(id, time.Duration(timeoutSec)*time.Second)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if asg == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, 200, asg)
}

func (h *Handler) workerResult(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	var req hub.ResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := h.Hub.SubmitResult(req); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) testEcho(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Message == "" {
		body.Message = "ping"
	}
	pl, _ := json.Marshal(map[string]string{"message": body.Message})
	job, err := h.Hub.SubmitSimple("echo", pl, 1, false, "echo test")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, job)
}

func (h *Handler) testSleep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	var body struct {
		Ms     int64 `json:"ms"`
		Shards int   `json:"shards"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Ms <= 0 {
		body.Ms = 200
	}
	if body.Shards <= 0 {
		body.Shards = 1
	}
	pl, _ := json.Marshal(map[string]int64{"ms": body.Ms})
	job, err := h.Hub.SubmitSimple("sleep", pl, body.Shards, false, "sleep test")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, job)
}
