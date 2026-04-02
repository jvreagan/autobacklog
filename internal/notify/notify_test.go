package notify

import (
	"fmt"
	"testing"
)

func TestNoopNotifier(t *testing.T) {
	n := NoopNotifier{}
	err := n.Send(Notification{
		Event:   EventError,
		Subject: "test",
		Body:    "body",
	})
	if err != nil {
		t.Errorf("NoopNotifier.Send() = %v", err)
	}
}

func TestCycleCompleteNotification(t *testing.T) {
	n := CycleCompleteNotification(10, 3, 2, "$5.00 / $100.00")
	if n.Event != EventCycleComplete {
		t.Errorf("Event = %q", n.Event)
	}
	if n.Subject == "" {
		t.Error("Subject should not be empty")
	}
	if n.Body == "" {
		t.Error("Body should not be empty")
	}
}

func TestStuckNotification(t *testing.T) {
	n := StuckNotification("Fix bug", "main.go", 3, "tests failed")
	if n.Event != EventStuck {
		t.Errorf("Event = %q", n.Event)
	}
}

func TestOutOfTokensNotification(t *testing.T) {
	n := OutOfTokensNotification(95.0, 100.0)
	if n.Event != EventOutOfTokens {
		t.Errorf("Event = %q", n.Event)
	}
}

func TestPRCreatedNotification(t *testing.T) {
	n := PRCreatedNotification("Fix auth", "https://github.com/pr/1", "Fixed auth flow")
	if n.Event != EventPRCreated {
		t.Errorf("Event = %q", n.Event)
	}
}

func TestErrorNotification(t *testing.T) {
	n := ErrorNotification("REVIEW", fmt.Errorf("timeout"))
	if n.Event != EventError {
		t.Errorf("Event = %q", n.Event)
	}
}
