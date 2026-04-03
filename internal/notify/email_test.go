package notify

import (
	"log/slog"
	"testing"

	"github.com/jamesreagan/autobacklog/internal/config"
)

func testNotifConfig() config.NotificationsConfig {
	return config.NotificationsConfig{
		Enabled:    true,
		SMTP:       config.SMTPConfig{Host: "smtp.test.com", Port: 587, From: "test@test.com"},
		Recipients: []string{"dev@test.com"},
		Events: config.EventsConfig{
			OnCycleComplete: true,
			OnStuck:         true,
			OnOutOfTokens:   true,
			OnPRCreated:     true,
			OnError:         true,
		},
	}
}

func TestNewEmailNotifier_EventMapping(t *testing.T) {
	cfg := testNotifConfig()
	n := NewEmailNotifier(cfg, slog.Default())

	if !n.events[EventCycleComplete] {
		t.Error("cycle_complete should be enabled")
	}
	if !n.events[EventStuck] {
		t.Error("stuck should be enabled")
	}
	if !n.events[EventOutOfTokens] {
		t.Error("out_of_tokens should be enabled")
	}
	if !n.events[EventPRCreated] {
		t.Error("pr_created should be enabled")
	}
	if !n.events[EventError] {
		t.Error("error should be enabled")
	}
}

func TestEmailNotifier_Send_DisabledEvent(t *testing.T) {
	cfg := testNotifConfig()
	cfg.Events.OnCycleComplete = false
	n := NewEmailNotifier(cfg, slog.Default())

	err := n.Send(Notification{Event: EventCycleComplete, Subject: "test", Body: "body"})
	if err != nil {
		t.Errorf("disabled event should return nil, got: %v", err)
	}
}

func TestNewEmailNotifier_SelectiveEvents(t *testing.T) {
	cfg := testNotifConfig()
	cfg.Events.OnPRCreated = false
	cfg.Events.OnStuck = false
	n := NewEmailNotifier(cfg, slog.Default())

	if n.events[EventPRCreated] {
		t.Error("pr_created should be disabled")
	}
	if n.events[EventStuck] {
		t.Error("stuck should be disabled")
	}
	if !n.events[EventError] {
		t.Error("error should still be enabled")
	}
}

func TestEmailNotifier_Send_NoAuth(t *testing.T) {
	cfg := testNotifConfig()
	cfg.SMTP.Username = ""
	cfg.SMTP.Password = ""
	n := NewEmailNotifier(cfg, slog.Default())

	// Sending will fail (no actual SMTP server), but we verify the code path
	// where auth is nil doesn't panic
	_ = n.Send(Notification{Event: EventError, Subject: "test", Body: "body"})
	// Error is expected since there's no SMTP server — we just verify no panic
}
