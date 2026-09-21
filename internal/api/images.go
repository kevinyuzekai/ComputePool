package api

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kevinyuzekai/ComputePool/internal/hub"
	"github.com/kevinyuzekai/ComputePool/internal/jobs"
)

const maxUploadBody = 64 << 20 // 64 MiB multipart / JSON ceiling

func (h *Handler) mountImages(mux *http.ServeMux) {
	mux.HandleFunc("/api/images/paths", h.imagePaths)
	mux.HandleFunc("/api/images/inbox", h.imageInbox)
	mux.HandleFunc("/api/images/batch", h.imageBatch)
}

func (h *Handler) imagePaths(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, h.Hub.ImagePaths())
}

func (h *Handler) imageInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "GET only"})
		return
	}
	dir := r.URL.Query().Get("path")
	list, err := h.Hub.ListInbox(dir)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	paths := h.Hub.ImagePaths()
	writeJSON(w, 200, map[string]any{
		"inbox":   paths.Inbox,
		"outbox":  paths.Outbox,
		"files":   list,
		"maxBytes": jobs.MaxImageBytes,
	})
}

func (h *Handler) imageBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "POST only"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBody)
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		h.imageBatchMultipart(w, r)
		return
	}
	var opts hub.ImageBatchOpts
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	job, err := h.Hub.SubmitImageBatch(opts)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, job)
}

func (h *Handler) imageBatchMultipart(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxUploadBody); err != nil {
		writeJSON(w, 400, map[string]string{"error": "multipart: " + err.Error()})
		return
	}
	opts := hub.ImageBatchOpts{
		Label:     r.FormValue("label"),
		Format:    r.FormValue("format"),
		LocalOnly: r.FormValue("localOnly") == "1" || r.FormValue("localOnly") == "true",
	}
	if v := r.FormValue("maxEdge"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.MaxEdge = n
		}
	}
	if v := r.FormValue("quality"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.Quality = n
		}
	}
	files := map[string][]byte{}
	if r.MultipartForm != nil {
		for _, hdrs := range r.MultipartForm.File {
			for _, fh := range hdrs {
				f, err := fh.Open()
				if err != nil {
					continue
				}
				b, err := io.ReadAll(io.LimitReader(f, int64(jobs.MaxImageBytes)+1))
				_ = f.Close()
				if err != nil {
					continue
				}
				name := filepath.Base(fh.Filename)
				if name == "" {
					name = "upload.jpg"
				}
				files[name] = b
			}
		}
	}
	job, err := h.Hub.SubmitImageBytes(files, opts)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, job)
}
