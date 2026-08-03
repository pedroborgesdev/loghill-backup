package notification

import (
	"strings"
	"time"

	"logtheater/internal/alerts"
	"logtheater/internal/domain"
	"logtheater/internal/events"
	"logtheater/internal/executions"
)

type Recorder struct {
	alerts     *alerts.Service
	events     *events.Service
	executions *executions.Store
}

func (r *Recorder) SetExecutions(store *executions.Store) *Recorder { r.executions = store; return r }

func (r *Recorder) MarkExecutionProcessing(value domain.Notification) {
	if r.executions == nil || value.ExecutionID == "" {
		return
	}
	now := time.Now()
	_, _ = r.executions.Update(value.ExecutionID, func(v *executions.Record) {
		v.Status = executions.StatusProcessing
		for i := range v.Actions {
			v.Actions[i].Status = executions.StatusProcessing
			v.Actions[i].StartedAt = &now
		}
	})
}

func (r *Recorder) RecordExecutionDelivery(value domain.Notification, status domain.DeliveryStatus, message string, attempts int) {
	if r.executions == nil || value.ExecutionID == "" {
		return
	}
	now := time.Now()
	_, _ = r.executions.Update(value.ExecutionID, func(v *executions.Record) {
		v.AttemptCount = attempts
		final := executions.StatusFailed
		if status == domain.DeliverySent {
			final = executions.StatusSuccess
		}
		v.Status = final
		for i := range v.Actions {
			v.Actions[i].Status = final
			v.Actions[i].FinishedAt = &now
			v.Actions[i].AttemptCount = attempts
			if v.Actions[i].StartedAt != nil {
				duration := now.Sub(*v.Actions[i].StartedAt).Milliseconds()
				v.Actions[i].DurationMs = &duration
			}
			if message != "" {
				v.Actions[i].ErrorMessage = &message
			}
		}
		if message != "" {
			v.ErrorMessage = &message
		}
	})
}

func NewRecorder(alertService *alerts.Service, eventService *events.Service) *Recorder {
	return &Recorder{alerts: alertService, events: eventService}
}

func (r *Recorder) MarkPending(id string, test bool) error {
	if strings.HasPrefix(id, "evt_") {
		return r.events.MarkPending(id, test)
	}
	return r.alerts.MarkPending(id, test)
}

func (r *Recorder) RecordDelivery(id string, test bool, status domain.DeliveryStatus, message string) error {
	if strings.HasPrefix(id, "evt_") {
		return r.events.RecordDelivery(id, test, status, message)
	}
	return r.alerts.RecordDelivery(id, test, status, message)
}
