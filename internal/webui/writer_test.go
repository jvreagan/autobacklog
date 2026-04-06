package webui

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

func TestWritesToUnderlying(t *testing.T) {
	var buf bytes.Buffer
	hub := NewHub(100)
	tw := NewTeeWriter(&buf, hub, EventLog)

	tw.Write([]byte("hello\n"))

	if buf.String() != "hello\n" {
		t.Errorf("underlying got %q, want %q", buf.String(), "hello\n")
	}
}

func TestBroadcastsToHub(t *testing.T) {
	hub := NewHub(100)
	ch, _ := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	tw := NewTeeWriter(io.Discard, hub, EventClaude)
	tw.Write([]byte("data\n"))

	select {
	case e := <-ch:
		if e.Type != EventClaude || e.Data != "data" {
			t.Errorf("got %+v, want {claude, data}", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestSplitsLines(t *testing.T) {
	hub := NewHub(100)
	ch, _ := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	tw := NewTeeWriter(io.Discard, hub, EventLog)
	tw.Write([]byte("line1\nline2\nline3\n"))

	var got []string
	for i := 0; i < 3; i++ {
		select {
		case e := <-ch:
			got = append(got, e.Data)
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
	if len(got) != 3 || got[0] != "line1" || got[1] != "line2" || got[2] != "line3" {
		t.Errorf("got %v, want [line1, line2, line3]", got)
	}
}

func TestEmptyLinesSkipped(t *testing.T) {
	hub := NewHub(100)
	ch, _ := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	tw := NewTeeWriter(io.Discard, hub, EventLog)
	tw.Write([]byte("\n\nhello\n\n"))

	select {
	case e := <-ch:
		if e.Data != "hello" {
			t.Errorf("got %q, want hello", e.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	// Should be no more events
	select {
	case e := <-ch:
		t.Errorf("unexpected extra event: %+v", e)
	case <-time.After(50 * time.Millisecond):
		// good
	}
}

// errWriter always returns an error.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write error") }

func TestReturnsUnderlyingError(t *testing.T) {
	hub := NewHub(100)
	tw := NewTeeWriter(errWriter{}, hub, EventLog)

	_, err := tw.Write([]byte("test"))
	if err == nil {
		t.Error("expected error from underlying writer")
	}
}
