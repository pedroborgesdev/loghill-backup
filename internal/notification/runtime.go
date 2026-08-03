package notification

import (
	"context"
	"log/slog"

	"logtheater/internal/alerts"
	"logtheater/internal/domain"
	"logtheater/internal/executions"
)

type Runtime struct {
	alerts     *alerts.Service
	dispatcher NotificationDispatcher
	executions *executions.Store
}

func (r *Runtime) SetExecutions(store *executions.Store) *Runtime { r.executions = store; return r }

func NewRuntime(alertsService *alerts.Service, dispatcher NotificationDispatcher) *Runtime {
	return &Runtime{alerts: alertsService, dispatcher: dispatcher}
}

func (r *Runtime) Notify(ctx context.Context, sender domain.Sender, entry domain.LogEntry) {
	for _, alert := range r.alerts.Matching(sender.ID, entry.Severity) {
		executionID := ""
		if r.executions != nil {
			severity := entry.Severity
			record, err := r.executions.Create(executions.Record{SourceType: executions.SourceAlert, SourceID: alert.ID, SourceName: alert.Name, SenderID: sender.ID, SenderName: sender.Name, TriggerType: "log_severity", TriggerName: string(entry.Severity), TriggerMessage: entry.Message, Severity: &severity, Status: executions.StatusPending, CorrelationID: executions.NewID("corr_"), CausationID: entry.EventOccurrenceID, Actions: []executions.ActionResult{{ID: executions.NewID("action_"), Type: "send_email", Status: executions.StatusPending, AttemptCount: 0}}})
			if err == nil {
				executionID = record.ID
			}
		}
		value := domain.Notification{ExecutionID: executionID, SourceType: domain.NotificationSourceAlert, SourceID: alert.ID, Alert: alert, Sender: sender, Entry: entry}
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
				_ = r.alerts.RecordDelivery(alert.ID, false, domain.DeliveryFailed, "A fila de notificações está cheia; o log foi preservado, mas o e-mail não foi enfileirado.")
			}
			if r.executions != nil && executionID != "" {
				message := "A fila de notificações está cheia."
				_, _ = r.executions.Update(executionID, func(v *executions.Record) {
					v.Status = executions.StatusFailed
					v.ErrorCode = stringPointer("QUEUE_FULL")
					v.ErrorMessage = &message
				})
			}
			slog.Warn("email alert queue rejected notification", "alert_id", alert.ID, "sender_id", sender.ID, "severity", entry.Severity, "error", err)
		} else {
			slog.Info("email alert notification enqueued", "alert_id", alert.ID, "sender_id", sender.ID, "severity", entry.Severity)
		}
	}
}

func stringPointer(value string) *string { return &value }
