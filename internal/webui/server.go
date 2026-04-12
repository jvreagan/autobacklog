package webui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"log/slog"
	"net"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

// Server serves the web UI and SSE event streams.
type Server struct {
	port     int
	hub      *Hub
	configFn func() any
	statsFn  func() any
	log      *slog.Logger
	srv      *http.Server
	listener net.Listener
}

// NewServer creates a new web UI server. configFn returns the sanitized
// config to expose via the /api/config endpoint. statsFn returns the latest
// cycle stats (may be nil).
func NewServer(port int, hub *Hub, configFn func() any, statsFn func() any, log *slog.Logger) *Server {
	return &Server{
		port:     port,
		hub:      hub,
		configFn: configFn,
		statsFn:  statsFn,
		log:      log,
	}
}

// Start binds the port eagerly and serves in a background goroutine.
// Returns an error immediately if the port is already in use.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port)) // #129: bind localhost only
	if err != nil {
		return fmt.Errorf("webui port %d is already in use", s.port)
	}
	s.listener = ln

	mux := http.NewServeMux()

	// Serve embedded static files
	staticSub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		ln.Close()
		return fmt.Errorf("loading static files: %w", err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(staticSub)))

	// SSE event stream
	mux.HandleFunc("GET /api/events", s.handleSSE)

	// Config endpoint
	mux.HandleFunc("GET /api/config", s.handleConfig)

	// Stats endpoint
	mux.HandleFunc("GET /api/stats", s.handleStats)

	s.srv = &http.Server{Handler: mux}

	go func() {
		s.log.Info("webui server started", "port", s.port, "url", fmt.Sprintf("http://localhost:%d", s.port))
		if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.log.Error("webui server error", "error", err)
		}
	}()

	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	typeFilter := r.URL.Query().Get("type")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // #213: disable reverse proxy buffering

	ch, history := s.hub.Subscribe()
	defer s.hub.Unsubscribe(ch)

	// Send history
	for _, e := range history {
		if typeFilter != "" && string(e.Type) != typeFilter {
			continue
		}
		writeSSEEvent(w, e)
	}
	flusher.Flush()

	// Stream live events
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return
			}
			if typeFilter != "" && string(e.Type) != typeFilter {
				continue
			}
			writeSSEEvent(w, e)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// writeSSEEvent writes a properly escaped SSE event. Newlines in data are
// split into multiple "data:" lines per the SSE specification (#119).
func writeSSEEvent(w io.Writer, e Event) {
	fmt.Fprintf(w, "event: %s\n", e.Type)
	for _, line := range strings.Split(e.Data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.configFn()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(cfg); err != nil {
		s.log.Error("failed to encode config", "error", err)
	}
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if s.statsFn == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("null"))
		return
	}
	data := s.statsFn()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.log.Error("failed to encode stats", "error", err)
	}
}
