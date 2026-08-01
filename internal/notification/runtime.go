package notification

import (
	"context"
	"log/slog"

	"logtheater/internal/alerts"
	"logtheater/internal/domain"
)

type Runtime struct {
	alerts     *alerts.Service
	dispatcher NotificationDispatcher
}

func NewRuntime(alertsService *alerts.Service, dispatcher NotificationDispatcher) *Runtime {
	return &Runtime{alerts: alertsService, dispatcher: dispatcher}
}

func (r *Runtime) Notify(ctx context.Context, sender domain.Sender, entry domain.LogEntry) {
	for _, alert := range r.alerts.Matching(sender.ID, entry.Severity) {
		value := domain.Notification{SourceType: domain.NotificationSourceAlert, SourceID: alert.ID, Alert: alert, Sender: sender, Entry: entry}
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
			slog.Warn("email alert queue rejected notification", "alert_id", alert.ID, "sender_id", sender.ID, "severity", entry.Severity, "error", err)
		} else {
			slog.Info("email alert notification enqueued", "alert_id", alert.ID, "sender_id", sender.ID, "severity", entry.Severity)
		}
	}
}
