// gotunnel/pkg/tunnel/dashboard.go
package tunnel

import (
	"encoding/json"
	_ "embed"
	"log/slog"
	"net/http"
)

//go:embed dashboard.html
var dashboardHTML string

// StartDashboard starts the web dashboard on the given address.
// Pass empty string to disable.
func (s *Server) StartDashboard(addr string) {
	if addr == "" {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(dashboardHTML))
	})
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.Stats())
	})

	go func() {
		slog.Info("dashboard started", "addr", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			slog.Error("dashboard error", "err", err)
		}
	}()
}
