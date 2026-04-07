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
// and keeps a circular buffer of recent history for new subscribers.
type Hub struct {
	mu          sync.RWMutex
	clients     map[chan Event]struct{}
	history     []Event
	historySize int
	head        int // #160: circular buffer index for O(1) insertion
	count       int // number of events stored
}

// NewHub creates a hub that retains up to historySize events.
func NewHub(historySize int) *Hub {
	return &Hub{
		clients:     make(map[chan Event]struct{}),
		history:     make([]Event, historySize),
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

	// Build snapshot in chronological order from circular buffer.
	snap := make([]Event, 0, h.count)
	for i := 0; i < h.count; i++ {
		idx := (h.head - h.count + i + h.historySize) % h.historySize
		snap = append(snap, h.history[idx])
	}

	return ch, snap
}

// Unsubscribe removes a client and closes its channel.
func (h *Hub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// #212: guard against double-close panic
	if _, ok := h.clients[ch]; !ok {
		return
	}
	delete(h.clients, ch)
	close(ch)
}

// Broadcast sends an event to all connected clients (non-blocking).
// Slow clients that can't keep up have events dropped.
func (h *Hub) Broadcast(e Event) {
	// #161: take a snapshot of clients under read lock, then send without holding the lock.
	h.mu.RLock()
	clients := make([]chan Event, 0, len(h.clients))
	for ch := range h.clients {
		clients = append(clients, ch)
	}
	h.mu.RUnlock()

	// Fan out to clients without holding any lock.
	for _, ch := range clients {
		select {
		case ch <- e:
		default:
			// drop for slow client
		}
	}

	// Update ring buffer under write lock.
	h.mu.Lock()
	if h.historySize > 0 {
		h.history[h.head] = e
		h.head = (h.head + 1) % h.historySize
		if h.count < h.historySize {
			h.count++
		}
	}
	h.mu.Unlock()
}
