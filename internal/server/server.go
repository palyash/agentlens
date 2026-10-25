// Package server exposes the read-only dashboard API and static UI on localhost.
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/palyash/agentlens/internal/analyze"
	"github.com/palyash/agentlens/internal/transcript"
	"github.com/palyash/agentlens/internal/web"
)

// New returns the HTTP handler. Only GET routes exist; nothing is ever written.
func New(root string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/data", func(w http.ResponseWriter, r *http.Request) {
		days := 30
		if v := r.URL.Query().Get("days"); v != "" {
			days, _ = strconv.Atoi(v)
		}
		ss := transcript.LoadAll(root)
		rep := analyze.Aggregate(ss, days, r.URL.Query().Get("project"))
		rep["fixes"] = analyze.Recommend(ss)
		writeJSON(w, rep)
	})
	mux.HandleFunc("GET /api/session/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, analyze.Detail(transcript.LoadAll(root), r.PathValue("id")))
	})
	mux.Handle("GET /", http.FileServerFS(web.FS))
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
