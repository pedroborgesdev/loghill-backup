package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"logmate/internal/alerts"
	"logmate/internal/domain"
	"logmate/internal/emailconfig"
	"logmate/internal/events"
	"logmate/internal/notification"
	"logmate/internal/repositories"
)

type Resolver struct {
	Repo   repositories.SenderRepository
	Events *events.Service
	Alerts *alerts.Service
	Email  *emailconfig.Store
}

func (r Resolver) Sender(ctx context.Context, id string) (domain.Sender, error) {
	return r.Repo.Get(ctx, id)
}
func (r Resolver) Event(id string) (domain.EventDefinition, error) { return r.Events.Get(id) }
func (r Resolver) Alert(id string) (domain.EmailAlert, error)      { return r.Alerts.Get(id) }
func (r Resolver) EmailReady() bool                                { v, err := r.Email.Runtime(); return err == nil && v.Enabled }

type Executor struct {
	service    *Service
	events     *events.Service
	dispatcher notification.NotificationDispatcher
}

func NewExecutor(service *Service, eventService *events.Service, dispatcher notification.NotificationDispatcher) *Executor {
	return &Executor{service: service, events: eventService, dispatcher: dispatcher}
}

type emailActionConfig struct {
	Recipients []string `json:"recipients"`
	Subject    string   `json:"subject"`
	Message    string   `json:"message"`
}
type eventActionConfig struct {
	EventID  string             `json:"event_id"`
	Message  string             `json:"message"`
	Severity domain.LogSeverity `json:"severity"`
}

func (e *Executor) ExecuteMonitoringAction(ctx context.Context, rule Rule, action Action, sender domain.Sender, entry domain.LogEntry, correlation string, depth int) error {
	switch action.Type {
	case ActionEmail:
		var cfg emailActionConfig
		if err := json.Unmarshal(action.Config, &cfg); err != nil {
			return err
		}
		if len(cfg.Recipients) == 0 {
			return errors.New("email action without recipients")
		}
		if strings.TrimSpace(cfg.Subject) == "" {
			cfg.Subject = "Monitoring: " + rule.Name
		}
		if strings.TrimSpace(cfg.Message) == "" {
			cfg.Message = "The monitoring rule matched."
		}
		event := domain.EventDefinition{ID: "evt_monitoring_" + rule.ID, Name: rule.Name, Key: "monitoring_" + rule.ID, Recipients: cfg.Recipients, SubjectTemplate: cfg.Subject, MessageTemplate: cfg.Message, Enabled: true}
		return e.dispatcher.Dispatch(ctx, domain.Notification{SourceType: domain.NotificationSourceMonitoring, SourceID: rule.ID, Event: event, Sender: sender, Entry: entry, Recipients: cfg.Recipients})
	case ActionEvent:
		var cfg eventActionConfig
		if err := json.Unmarshal(action.Config, &cfg); err != nil {
			return err
		}
		target, err := e.events.Get(cfg.EventID)
		if err != nil {
			return err
		}
		generated := entry
		generated.Event = target.Key
		generated.EventOccurrenceID = correlation
		generated.Timestamp = time.Now()
		if cfg.Message != "" {
			generated.Message = cfg.Message
		}
		if cfg.Severity != "" {
			generated.Severity = cfg.Severity
		}
		if err = e.dispatcher.Dispatch(ctx, domain.Notification{SourceType: domain.NotificationSourceEvent, SourceID: target.ID, Event: target, Sender: sender, Entry: generated}); err != nil {
			return err
		}
		e.service.notify(ctx, sender, generated, "", correlation, depth)
		return nil
	case ActionHTTP:
		var config domain.HTTPRequestConfig
		if err := json.Unmarshal(action.Config, &config); err != nil {
			return err
		}
		event := domain.EventDefinition{ID: "evt_monitoring_" + rule.ID, Name: rule.Name, Key: "monitoring_" + rule.ID, ActionType: domain.EventActionHTTP, HTTPRequest: &config, Enabled: true}
		return e.dispatcher.Dispatch(ctx, domain.Notification{SourceType: domain.NotificationSourceMonitoring, SourceID: rule.ID, Event: event, Sender: sender, Entry: entry})
	default:
		return errors.New("invalid monitoring action type")
	}
}
