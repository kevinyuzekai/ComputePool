package hub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kevinyuzekai/ComputePool/internal/jobs"
	"github.com/kevinyuzekai/ComputePool/internal/paths"
)

// ImageBatchOpts configures an image_resize batch job.
type ImageBatchOpts struct {
	InboxPath string   `json:"inboxPath,omitempty"` // defaults to hub inbox
	Files     []string `json:"files,omitempty"`     // basenames or absolute paths; empty = scan inbox
	MaxEdge   int      `json:"maxEdge"`
	Quality   int      `json:"quality"`
	Format    string   `json:"format"`
	LocalOnly bool     `json:"localOnly"`
	Label     string   `json:"label"`
}

// ImagePathsInfo is returned by /api/images/paths.
type ImagePathsInfo struct {
	Inbox  string `json:"inbox"`
	Outbox string `json:"outbox"`
}

// InboxEntry is one file listed under the inbox.
type InboxEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Skip  string `json:"skip,omitempty"` // reason if not eligible
}

// SetDirs configures inbox/outbox (creates dirs). Empty → defaults.
func (h *Hub) SetDirs(inbox, outbox string) error {
	if inbox == "" {
		inbox = paths.DefaultInbox()
	} else {
		inbox = paths.Expand(inbox)
	}
	if outbox == "" {
		outbox = paths.DefaultOutbox()
	} else {
		outbox = paths.Expand(outbox)
	}
	if err := paths.EnsureDir(inbox); err != nil {
		return fmt.Errorf("inbox: %w", err)
	}
	if err := paths.EnsureDir(outbox); err != nil {
		return fmt.Errorf("outbox: %w", err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.inboxDir = inbox
	h.outboxDir = outbox
	return nil
}

// ImagePaths returns configured inbox/outbox.
func (h *Hub) ImagePaths() ImagePathsInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	return ImagePathsInfo{Inbox: h.inboxDir, Outbox: h.outboxDir}
}

// ListInbox scans inbox (or override path) for image files.
func (h *Hub) ListInbox(dir string) ([]InboxEntry, error) {
	h.mu.Lock()
	inbox := h.inboxDir
	h.mu.Unlock()
	if strings.TrimSpace(dir) != "" {
		inbox = paths.Expand(dir)
	}
	if err := paths.EnsureDir(inbox); err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(inbox)
	if err != nil {
		return nil, err
	}
	out := make([]InboxEntry, 0, len(ents))
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		info, err := e.Info()
		if err != nil {
			continue
		}
		full := filepath.Join(inbox, name)
		entry := InboxEntry{Name: name, Path: full, Bytes: info.Size()}
		if !jobs.ImageExtOK(name) {
			entry.Skip = "unsupported extension"
		} else if info.Size() > int64(jobs.MaxImageBytes) {
			entry.Skip = fmt.Sprintf("too large (>%d bytes)", jobs.MaxImageBytes)
		} else if info.Size() == 0 {
			entry.Skip = "empty file"
		}
		out = append(out, entry)
	}
	return out, nil
}

// SubmitImageBatch creates one image_resize shard per eligible image.
func (h *Hub) SubmitImageBatch(opts ImageBatchOpts) (*Job, error) {
	h.mu.Lock()
	inbox := h.inboxDir
	outbox := h.outboxDir
	h.mu.Unlock()

	if strings.TrimSpace(opts.InboxPath) != "" {
		inbox = paths.Expand(opts.InboxPath)
	}
	if err := paths.EnsureDir(inbox); err != nil {
		return nil, errBad("inbox: " + err.Error())
	}

	maxEdge := opts.MaxEdge
	if maxEdge <= 0 {
		maxEdge = jobs.DefaultMaxEdge
	}
	quality := opts.Quality
	if quality <= 0 {
		quality = jobs.DefaultQuality
	}
	format := strings.ToLower(strings.TrimSpace(opts.Format))
	if format == "" {
		format = "jpeg"
	}
	if format == "jpg" {
		format = "jpeg"
	}
	if format != "jpeg" && format != "png" && format != "webp" {
		return nil, errBad("format must be jpeg, png, or webp")
	}

	var filePaths []string
	if len(opts.Files) > 0 {
		for _, f := range opts.Files {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			if filepath.IsAbs(f) || strings.HasPrefix(f, "~/") {
				filePaths = append(filePaths, paths.Expand(f))
			} else {
				filePaths = append(filePaths, filepath.Join(inbox, filepath.Base(f)))
			}
		}
	} else {
		ents, err := h.ListInbox(inbox)
		if err != nil {
			return nil, errBad(err.Error())
		}
		for _, e := range ents {
			if e.Skip == "" {
				filePaths = append(filePaths, e.Path)
			}
		}
	}
	if len(filePaths) == 0 {
		return nil, errBad("no eligible images (drop files into inbox or pass files[])")
	}
	if len(filePaths) > 256 {
		return nil, errBad("too many images (max 256 per batch)")
	}

	type prepared struct {
		payload json.RawMessage
		name    string
	}
	preparedShards := make([]prepared, 0, len(filePaths))
	var skipped []string
	for _, fp := range filePaths {
		name := filepath.Base(fp)
		if !jobs.ImageExtOK(name) {
			skipped = append(skipped, name+": unsupported")
			continue
		}
		raw, err := os.ReadFile(fp)
		if err != nil {
			skipped = append(skipped, name+": "+err.Error())
			continue
		}
		if len(raw) == 0 {
			skipped = append(skipped, name+": empty")
			continue
		}
		if len(raw) > jobs.MaxImageBytes {
			skipped = append(skipped, fmt.Sprintf("%s: too large (%d)", name, len(raw)))
			continue
		}
		pl, _ := json.Marshal(jobs.ImageResizePayload{
			ImageBase64: base64.StdEncoding.EncodeToString(raw),
			FileName:    name,
			MaxEdge:     maxEdge,
			Quality:     quality,
			Format:      format,
		})
		preparedShards = append(preparedShards, prepared{payload: pl, name: name})
	}
	if len(preparedShards) == 0 {
		msg := "no images could be loaded"
		if len(skipped) > 0 {
			msg += ": " + strings.Join(skipped, "; ")
		}
		return nil, errBad(msg)
	}

	label := strings.TrimSpace(opts.Label)
	if label == "" {
		label = fmt.Sprintf("图片批处理 ×%d", len(preparedShards))
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	job := h.newJobLocked(jobs.TypeImageResize, label, "image_batch", opts.LocalOnly)
	job.OutDir = paths.JobOutbox(outbox, job.ID)
	for _, p := range preparedShards {
		s := h.newShardLocked(job, jobs.TypeImageResize, p.payload, opts.LocalOnly)
		job.Shards = append(job.Shards, s)
		h.pending = append(h.pending, s)
	}
	job.TotalShards = len(job.Shards)
	job.Status = JobRunning
	job.StartedAt = time.Now()
	if len(skipped) > 0 {
		job.Note = "skipped: " + strings.Join(skipped, "; ")
	}
	// create outbox job dir early
	_ = paths.EnsureDir(job.OutDir)
	h.broadcastLocked()
	return slimImageJobClone(job), nil
}

// SubmitImageBytes creates a batch from in-memory uploads (multipart).
func (h *Hub) SubmitImageBytes(files map[string][]byte, opts ImageBatchOpts) (*Job, error) {
	if len(files) == 0 {
		return nil, errBad("no files uploaded")
	}
	if len(files) > 256 {
		return nil, errBad("too many images (max 256)")
	}
	maxEdge := opts.MaxEdge
	if maxEdge <= 0 {
		maxEdge = jobs.DefaultMaxEdge
	}
	quality := opts.Quality
	if quality <= 0 {
		quality = jobs.DefaultQuality
	}
	format := strings.ToLower(strings.TrimSpace(opts.Format))
	if format == "" {
		format = "jpeg"
	}
	if format == "jpg" {
		format = "jpeg"
	}

	h.mu.Lock()
	outbox := h.outboxDir
	h.mu.Unlock()

	label := strings.TrimSpace(opts.Label)
	if label == "" {
		label = fmt.Sprintf("图片上传 ×%d", len(files))
	}

	type prepared struct {
		payload json.RawMessage
	}
	var preparedShards []prepared
	var skipped []string
	for name, raw := range files {
		base := filepath.Base(name)
		if !jobs.ImageExtOK(base) {
			skipped = append(skipped, base+": unsupported")
			continue
		}
		if len(raw) == 0 || len(raw) > jobs.MaxImageBytes {
			skipped = append(skipped, base+": size")
			continue
		}
		pl, _ := json.Marshal(jobs.ImageResizePayload{
			ImageBase64: base64.StdEncoding.EncodeToString(raw),
			FileName:    base,
			MaxEdge:     maxEdge,
			Quality:     quality,
			Format:      format,
		})
		preparedShards = append(preparedShards, prepared{payload: pl})
	}
	if len(preparedShards) == 0 {
		return nil, errBad("no eligible uploaded images")
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	job := h.newJobLocked(jobs.TypeImageResize, label, "image_batch", opts.LocalOnly)
	job.OutDir = paths.JobOutbox(outbox, job.ID)
	for _, p := range preparedShards {
		s := h.newShardLocked(job, jobs.TypeImageResize, p.payload, opts.LocalOnly)
		job.Shards = append(job.Shards, s)
		h.pending = append(h.pending, s)
	}
	job.TotalShards = len(job.Shards)
	job.Status = JobRunning
	job.StartedAt = time.Now()
	if len(skipped) > 0 {
		job.Note = "skipped: " + strings.Join(skipped, "; ")
	}
	_ = paths.EnsureDir(job.OutDir)
	h.broadcastLocked()
	return slimImageJobClone(job), nil
}


func slimImageJobClone(job *Job) *Job {
	cp := cloneJob(job, true)
	if cp == nil {
		return nil
	}
	for _, s := range cp.Shards {
		if s == nil {
			continue
		}
		// Keep tiny hint for UI; workers still have full payload in hub memory.
		var meta struct {
			FileName string `json:"fileName"`
			MaxEdge  int    `json:"maxEdge"`
			Quality  int    `json:"quality"`
			Format   string `json:"format"`
		}
		_ = json.Unmarshal(s.Payload, &meta)
		hint, _ := json.Marshal(map[string]any{
			"fileName":      meta.FileName,
			"maxEdge":       meta.MaxEdge,
			"quality":       meta.Quality,
			"format":        meta.Format,
			"imageBase64":   "[omitted]",
		})
		s.Payload = hint
	}
	return cp
}

func (h *Hub) writeImageOutboxLocked(job *Job, shard *Shard) {
	if job == nil || shard == nil || job.Type != jobs.TypeImageResize {
		return
	}
	if shard.Status != ShardDone || len(shard.Result) == 0 {
		return
	}
	var res jobs.ImageResizeResult
	if err := json.Unmarshal(shard.Result, &res); err != nil {
		return
	}
	if res.ImageBase64 == "" {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(res.ImageBase64)
	if err != nil {
		return
	}
	outDir := job.OutDir
	if outDir == "" {
		outDir = paths.JobOutbox(h.outboxDir, job.ID)
		job.OutDir = outDir
	}
	_ = paths.EnsureDir(outDir)
	name := res.FileName
	if name == "" {
		name = shard.ShardID + ".jpg"
	}
	name = filepath.Base(name)
	// avoid overwrite collisions
	dest := filepath.Join(outDir, name)
	if _, err := os.Stat(dest); err == nil {
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		dest = filepath.Join(outDir, fmt.Sprintf("%s-%s%s", base, shard.ShardID, ext))
	}
	if err := os.WriteFile(dest, raw, 0o644); err != nil {
		return
	}
	shard.OutFile = dest
	// Drop bulky base64 from in-memory result; keep metrics for UI.
	slim := jobs.ImageResizeResult{
		FileName:     res.FileName,
		Format:       res.Format,
		Width:        res.Width,
		Height:       res.Height,
		BytesIn:      res.BytesIn,
		BytesOut:     res.BytesOut,
		ElapsedMs:    res.ElapsedMs,
		SkippedScale: res.SkippedScale,
		Note:         res.Note,
	}
	if b, err := json.Marshal(slim); err == nil {
		shard.Result = b
	}
}
