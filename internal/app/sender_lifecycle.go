package app

import (
	"context"
	"fmt"

	"logtheater/internal/alerts"
	"logtheater/internal/domain"
	"logtheater/internal/events"
	"logtheater/internal/monitoring"
	"logtheater/internal/services"
)

type SenderDependencies struct {
	AlertRules      int `json:"alert_rules"`
	Events          int `json:"events"`
	MonitoringRules int `json:"monitoring_rules"`
}

func (d SenderDependencies) HasAny() bool {
	return d.AlertRules > 0 || d.Events > 0 || d.MonitoringRules > 0
}

type DeleteSenderOptions struct {
	RemoveFromAlerts     bool
	RemoveFromEvents     bool
	RemoveFromMonitoring bool
}

type SenderLifecycle struct {
	Senders    *services.SenderService
	Alerts     *alerts.Service
	Events     *events.Service
	Monitoring *monitoring.Service
}

func (l *SenderLifecycle) Dependencies(senderID string) SenderDependencies {
	deps := SenderDependencies{}
	if l.Alerts != nil {
		deps.AlertRules = l.Alerts.SenderUsageCount(senderID)
	}
	if l.Events != nil {
		deps.Events = l.Events.SenderUsageCount(senderID)
	}
	if l.Monitoring != nil {
		deps.MonitoringRules = l.Monitoring.SenderUsageCount(senderID)
	}
	return deps
}

func (l *SenderLifecycle) Update(ctx context.Context, senderID, name, description string) (domain.Sender, error) {
	item, err := l.Senders.UpdateSender(ctx, senderID, name, description)
	if err != nil {
		return domain.Sender{}, err
	}
	if l.Alerts != nil {
		if err = l.Alerts.RenameSender(item.ID, item.Name); err != nil {
			return domain.Sender{}, err
		}
	}
	return item, nil
}

type DependencyConflictError struct {
	Code         string
	Message      string
	Dependencies SenderDependencies
}

func (e *DependencyConflictError) Error() string { return e.Message }

func (l *SenderLifecycle) Delete(ctx context.Context, senderID string, opts DeleteSenderOptions) error {
	deps := l.Dependencies(senderID)
	if (deps.AlertRules > 0 && !opts.RemoveFromAlerts) || (deps.Events > 0 && !opts.RemoveFromEvents) || (deps.MonitoringRules > 0 && !opts.RemoveFromMonitoring) {
		code := "SENDER_HAS_DEPENDENCIES"
		message := fmt.Sprintf("This sender is associated with %d alert rules and %d events.", deps.AlertRules, deps.Events)
		if deps.Events == 0 && deps.MonitoringRules == 0 {
			code = "SENDER_HAS_ALERTS"
			message = fmt.Sprintf("This sender is associated with %d alert rules.", deps.AlertRules)
		} else if deps.AlertRules == 0 && deps.MonitoringRules == 0 {
			code = "SENDER_HAS_EVENTS"
			message = fmt.Sprintf("This sender is associated with %d events.", deps.Events)
		}
		return &DependencyConflictError{Code: code, Message: message, Dependencies: deps}
	}
	if deps.AlertRules > 0 {
		if _, err := l.Alerts.RemoveSender(senderID); err != nil {
			return err
		}
	}
	if deps.Events > 0 {
		if _, err := l.Events.RemoveSender(senderID); err != nil {
			return err
		}
	}
	if deps.MonitoringRules > 0 {
		if err := l.Monitoring.RemoveSender(senderID); err != nil {
			return err
		}
	}
	return l.Senders.DeleteSender(ctx, senderID)
}
