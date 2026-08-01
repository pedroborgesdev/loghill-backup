package notification

import (
	"context"
	"log/slog"

	"logtheater/internal/domain"
	"logtheater/internal/events"
)

type EventRuntime struct {
	events     *events.Service
	dispatcher NotificationDispatcher
}

func NewEventRuntime(eventService *events.Service, dispatcher NotificationDispatcher) *EventRuntime {
	return &EventRuntime{events: eventService, dispatcher: dispatcher}
}

func (r *EventRuntime) NotifyEvent(ctx context.Context, sender domain.Sender, entry domain.LogEntry) {
	matches := r.events.Matching(sender.ID, entry.Event)
	if len(matches) == 0 {
		slog.Info("log event has no active configuration", "sender_id", sender.ID, "event", entry.Event)
		return
	}
	for _, event := range matches {
		value := domain.Notification{SourceType: domain.NotificationSourceEvent, SourceID: event.ID, Event: event, Sender: sender, Entry: entry}
		if err := r.dispatcher.Dispatch(context.WithoutCancel(ctx), value); err != nil {
			reported := false
			if reporter, ok := r.dispatcher.(interface {
				TryReportRejected(domain.Notification) bool
			}); ok {
				reported = reporter.TryReportRejected(value)
			} else if reporter, ok := r.dispatcher.(interface{ ReportRejected(domain.Notification) }); ok {
				reporter.ReportRejected(value)
				reported = true
			}
			if !reported {
				_ = r.events.RecordDelivery(event.ID, false, domain.DeliveryFailed, "A fila de notificações está cheia; o log foi preservado, mas o e-mail não foi enfileirado.")
			}
			slog.Warn("event notification queue rejected", "event_id", event.ID, "sender_id", sender.ID, "event", entry.Event, "error", err)
		} else {
			slog.Info("event notification enqueued", "event_id", event.ID, "sender_id", sender.ID, "event", entry.Event)
		}
	}
}
