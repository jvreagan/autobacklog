package webui

import (
	"fmt"
	"testing"
	"time"
)

func TestBroadcastToSubscriber(t *testing.T) {
	hub := NewHub(100)
	ch, _ := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	hub.Broadcast(Event{Type: EventLog, Data: "hello"})

	select {
	case e := <-ch:
		if e.Data != "hello" || e.Type != EventLog {
			t.Errorf("got %+v, want {log, hello}", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	hub := NewHub(100)
	ch1, _ := hub.Subscribe()
	defer hub.Unsubscribe(ch1)
	ch2, _ := hub.Subscribe()
	defer hub.Unsubscribe(ch2)

	hub.Broadcast(Event{Type: EventClaude, Data: "msg"})

	for _, ch := range []chan Event{ch1, ch2} {
		select {
		case e := <-ch:
			if e.Data != "msg" {
				t.Errorf("got %q, want msg", e.Data)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
}

func TestUnsubscribe(t *testing.T) {
	hub := NewHub(100)
	ch, _ := hub.Subscribe()
	hub.Unsubscribe(ch)

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after unsubscribe")
	}

	// Broadcasting after unsubscribe should not panic
	hub.Broadcast(Event{Type: EventLog, Data: "after unsub"})
}

func TestSlowClientDrops(t *testing.T) {
	hub := NewHub(100)
	ch, _ := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	// Fill the channel buffer (256)
	for i := 0; i < 300; i++ {
		hub.Broadcast(Event{Type: EventLog, Data: "flood"})
	}

	// Should not block — slow messages are dropped
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count > 256 {
		t.Errorf("received %d events, expected at most 256 (buffer size)", count)
	}
}

func TestHistoryOnSubscribe(t *testing.T) {
	hub := NewHub(100)

	hub.Broadcast(Event{Type: EventLog, Data: "first"})
	hub.Broadcast(Event{Type: EventClaude, Data: "second"})

	ch, history := hub.Subscribe()
	defer hub.Unsubscribe(ch)

	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].Data != "first" || history[1].Data != "second" {
		t.Errorf("history = %+v", history)
	}
}

func TestHistoryRingBuffer(t *testing.T) {
	hub := NewHub(3)

	for i := 0; i < 5; i++ {
		hub.Broadcast(Event{Type: EventLog, Data: fmt.Sprintf("msg%d", i)})
	}

	_, history := hub.Subscribe()
	if len(history) != 3 {
		t.Fatalf("history len = %d, want 3", len(history))
	}
	// Should contain the last 3: msg2, msg3, msg4
	if history[0].Data != "msg2" || history[1].Data != "msg3" || history[2].Data != "msg4" {
		t.Errorf("history = %+v, want [msg2, msg3, msg4]", history)
	}
}
