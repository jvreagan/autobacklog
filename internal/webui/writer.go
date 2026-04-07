package webui

import (
	"bytes"
	"io"
)

// TeeWriter is an io.Writer that writes to an underlying writer and
// broadcasts each non-empty line to an SSE hub as the given event type.
type TeeWriter struct {
	underlying io.Writer
	hub        *Hub
	eventType  EventType
}

// NewTeeWriter creates a TeeWriter that writes to w and broadcasts to hub.
func NewTeeWriter(w io.Writer, hub *Hub, eventType EventType) *TeeWriter {
	return &TeeWriter{
		underlying: w,
		hub:        hub,
		eventType:  eventType,
	}
}

// Write writes p to the underlying writer, then splits on newlines and
// broadcasts each non-empty line as an event.
func (tw *TeeWriter) Write(p []byte) (int, error) {
	n, err := tw.underlying.Write(p)

	// #157: only broadcast the bytes actually written
	broadcast := p[:n]

	// Broadcast each non-empty line
	lines := bytes.Split(broadcast, []byte("\n"))
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		tw.hub.Broadcast(Event{
			Type: tw.eventType,
			Data: string(line),
		})
	}

	return n, err
}
