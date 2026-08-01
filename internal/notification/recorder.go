package notification

import (
	"strings"

	"logtheater/internal/alerts"
	"logtheater/internal/domain"
	"logtheater/internal/events"
)

type Recorder struct {
	alerts *alerts.Service
	events *events.Service
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
