package notify

// EventType classifies the kind of event that triggers a notification.
type EventType string

const (
	EventCycleComplete EventType = "cycle_complete"
	EventStuck         EventType = "stuck"
	EventOutOfTokens   EventType = "out_of_tokens"
	EventPRCreated     EventType = "pr_created"
	EventError         EventType = "error"
)

// Notification contains the details for a notification.
type Notification struct {
	Event   EventType
	Subject string
	Body    string
}

// Notifier sends notifications.
type Notifier interface {
	Send(n Notification) error
}

// NoopNotifier discards all notifications (used when notifications are disabled).
type NoopNotifier struct{}

// Send discards the notification and always returns nil.
func (NoopNotifier) Send(Notification) error { return nil }
