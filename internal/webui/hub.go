package webui

import "sync"

// EventType identifies the stream an event belongs to.
type EventType string

const (
	EventLog    EventType = "log"
	EventClaude EventType = "claude"
)

// Event is a single message broadcast through the hub.
type Event struct {
	Type EventType
	Data string
}

// Hub is an SSE broadcaster that fans out events to connected clients
// and keeps a ring buffer of recent history for new subscribers.
type Hub struct {
	mu          sync.RWMutex
	clients     map[chan Event]struct{}
	history     []Event
	historySize int
}

// NewHub creates a hub that retains up to historySize events.
func NewHub(historySize int) *Hub {
	return &Hub{
		clients:     make(map[chan Event]struct{}),
		history:     make([]Event, 0, historySize),
		historySize: historySize,
	}
}

// Subscribe registers a new client. It returns a buffered channel for
// receiving future events and a snapshot of the current history.
func (h *Hub) Subscribe() (chan Event, []Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan Event, 256)
	h.clients[ch] = struct{}{}

	snap := make([]Event, len(h.history))
	copy(snap, h.history)

	return ch, snap
}

// Unsubscribe removes a client and closes its channel.
func (h *Hub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.clients, ch)
	close(ch)
}

// Broadcast sends an event to all connected clients (non-blocking).
// Slow clients that can't keep up have events dropped.
func (h *Hub) Broadcast(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Append to ring buffer
	if len(h.history) < h.historySize {
		h.history = append(h.history, e)
	} else if h.historySize > 0 {
		// Shift left and overwrite the last slot
		copy(h.history, h.history[1:])
		h.history[h.historySize-1] = e
	}

	// Fan out to clients
	for ch := range h.clients {
		select {
		case ch <- e:
		default:
			// drop for slow client
		}
	}
}
