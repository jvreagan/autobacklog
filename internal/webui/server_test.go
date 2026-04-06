package webui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestPortInUse(t *testing.T) {
	// Occupy a port
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	hub := NewHub(100)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(port, hub, func() any { return nil }, log)

	err = srv.Start()
	if err == nil {
		srv.Shutdown(context.Background())
		t.Fatal("expected error for port in use")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("webui port %d is already in use", port)) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartSuccess(t *testing.T) {
	port := freePort(t)
	hub := NewHub(100)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(port, hub, func() any { return map[string]string{"key": "val"} }, log)

	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/", port))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("GET / status = %d, want 200", resp.StatusCode)
	}
}

func TestSSEReceivesEvents(t *testing.T) {
	port := freePort(t)
	hub := NewHub(100)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(port, hub, func() any { return nil }, log)

	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	// Connect to SSE
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/events", port))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Broadcast an event
	hub.Broadcast(Event{Type: EventLog, Data: "test-line"})

	// Read from SSE stream
	scanner := bufio.NewScanner(resp.Body)
	var gotEvent, gotData string
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for SSE event")
		default:
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			gotEvent = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			gotData = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	if gotEvent != "log" || gotData != "test-line" {
		t.Errorf("got event=%q data=%q, want log/test-line", gotEvent, gotData)
	}
}

func TestSSETypeFilter(t *testing.T) {
	port := freePort(t)
	hub := NewHub(100)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(port, hub, func() any { return nil }, log)

	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	// Connect with type filter
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/events?type=claude", port))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Broadcast a log event (should be filtered) then a claude event
	hub.Broadcast(Event{Type: EventLog, Data: "filtered-out"})
	hub.Broadcast(Event{Type: EventClaude, Data: "should-see"})

	scanner := bufio.NewScanner(resp.Body)
	var gotData string
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout")
		default:
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			gotData = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	if gotData != "should-see" {
		t.Errorf("got data=%q, want should-see", gotData)
	}
}

func TestConfigEndpoint(t *testing.T) {
	port := freePort(t)
	hub := NewHub(100)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := map[string]any{"repo": "https://example.com", "mode": "oneshot"}
	srv := NewServer(port, hub, func() any { return cfg }, log)

	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/config", port))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["repo"] != "https://example.com" {
		t.Errorf("config repo = %v", got["repo"])
	}
}

func TestStaticServing(t *testing.T) {
	port := freePort(t)
	hub := NewHub(100)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(port, hub, func() any { return nil }, log)

	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/index.html", port))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "autobacklog") {
		t.Error("index.html should contain 'autobacklog'")
	}
}

func TestShutdown(t *testing.T) {
	port := freePort(t)
	hub := NewHub(100)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(port, hub, func() any { return nil }, log)

	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}

	// Server should no longer accept connections
	_, err := http.Get(fmt.Sprintf("http://localhost:%d/", port))
	if err == nil {
		t.Error("expected connection error after shutdown")
	}
}
