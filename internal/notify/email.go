package notify

import (
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"

	"github.com/jamesreagan/autobacklog/internal/config"
)

// EmailNotifier sends notifications via SMTP.
type EmailNotifier struct {
	cfg    config.NotificationsConfig
	log    *slog.Logger
	events map[EventType]bool
}

// NewEmailNotifier creates an email notifier from config.
func NewEmailNotifier(cfg config.NotificationsConfig, log *slog.Logger) *EmailNotifier {
	events := map[EventType]bool{
		EventCycleComplete: cfg.Events.OnCycleComplete,
		EventStuck:         cfg.Events.OnStuck,
		EventOutOfTokens:   cfg.Events.OnOutOfTokens,
		EventPRCreated:     cfg.Events.OnPRCreated,
		EventError:         cfg.Events.OnError,
	}
	return &EmailNotifier{cfg: cfg, log: log, events: events}
}

// Send delivers a notification via SMTP email if the event type is enabled.
// Uses Go's smtp.SendMail which upgrades to STARTTLS when the server advertises it.
// PlainAuth refuses to send credentials over unencrypted connections.
func (e *EmailNotifier) Send(n Notification) error {
	if !e.events[n.Event] {
		e.log.Debug("notification event disabled, skipping", "event", n.Event)
		return nil
	}

	addr := fmt.Sprintf("%s:%d", e.cfg.SMTP.Host, e.cfg.SMTP.Port)

	var auth smtp.Auth
	if e.cfg.SMTP.Username != "" {
		auth = smtp.PlainAuth("", e.cfg.SMTP.Username, e.cfg.SMTP.Password, e.cfg.SMTP.Host)
	}

	to := e.cfg.Recipients
	subject := fmt.Sprintf("[autobacklog] %s", n.Subject)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		e.cfg.SMTP.From,
		strings.Join(to, ","),
		subject,
		n.Body,
	)

	e.log.Info("sending notification email", "event", n.Event, "subject", subject, "recipients", to)

	if err := smtp.SendMail(addr, auth, e.cfg.SMTP.From, to, []byte(msg)); err != nil {
		return fmt.Errorf("sending email: %w", err)
	}

	return nil
}
