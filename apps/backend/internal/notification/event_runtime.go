package notification

import (
	"context"
	"log/slog"

	"logmate/internal/domain"
	"logmate/internal/events"
	"logmate/internal/executions"
)

type EventRuntime struct {
	events     *events.Service
	dispatcher NotificationDispatcher
	executions *executions.Store
}

func (r *EventRuntime) SetExecutions(store *executions.Store) *EventRuntime {
	r.executions = store
	return r
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
		if event.ActionType == domain.EventActionNone {
			_ = r.events.RecordTriggered(event.ID)
			if r.executions != nil {
				severity := entry.Severity
				_, _ = r.executions.Create(executions.Record{SourceType: executions.SourceEvent, SourceID: event.ID, SourceName: event.Name, SenderID: sender.ID, SenderName: sender.Name, TriggerType: "log_event", TriggerID: entry.EventOccurrenceID, TriggerName: entry.Event, TriggerMessage: entry.Message, Severity: &severity, Status: executions.StatusSuccess, CorrelationID: executions.NewID("corr_"), CausationID: entry.EventOccurrenceID, Actions: []executions.ActionResult{}})
			}
			continue
		}
		executionID := ""
		if r.executions != nil {
			severity := entry.Severity
			actionType := "send_email"
			if event.ActionType == domain.EventActionWebhook {
				actionType = "webhook"
			} else if event.ActionType == domain.EventActionHTTP {
				actionType = "http"
			}
			record, err := r.executions.Create(executions.Record{SourceType: executions.SourceEvent, SourceID: event.ID, SourceName: event.Name, SenderID: sender.ID, SenderName: sender.Name, TriggerType: "log_event", TriggerID: entry.EventOccurrenceID, TriggerName: entry.Event, TriggerMessage: entry.Message, Severity: &severity, Status: executions.StatusPending, CorrelationID: executions.NewID("corr_"), CausationID: entry.EventOccurrenceID, Actions: []executions.ActionResult{{ID: executions.NewID("action_"), Type: actionType, Status: executions.StatusPending}}})
			if err == nil {
				executionID = record.ID
			}
		}
		value := domain.Notification{ExecutionID: executionID, SourceType: domain.NotificationSourceEvent, SourceID: event.ID, Event: event, Sender: sender, Entry: entry}
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
				_ = r.events.RecordDelivery(event.ID, false, domain.DeliveryFailed, "The notification queue is full; the log was preserved, but the action was not queued.")
			}
			if r.executions != nil && executionID != "" {
				message := "The notification queue is full."
				code := "QUEUE_FULL"
				_, _ = r.executions.Update(executionID, func(v *executions.Record) {
					v.Status = executions.StatusFailed
					v.ErrorCode = &code
					v.ErrorMessage = &message
				})
			}
			slog.Warn("event notification queue rejected", "event_id", event.ID, "sender_id", sender.ID, "event", entry.Event, "error", err)
		} else {
			slog.Info("event notification enqueued", "event_id", event.ID, "sender_id", sender.ID, "event", entry.Event)
		}
	}
}
