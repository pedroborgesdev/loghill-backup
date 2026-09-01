package services

import (
	"context"
	"errors"
	"time"

	"logtheater/internal/alerts"
	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
	"logtheater/internal/emailprovider"
	"logtheater/internal/events"
	"logtheater/internal/notification"
	"logtheater/internal/validation"
)

var (
	ErrNotificationWithoutSenders = errors.New("notification source has no senders")
	ErrInvalidEmailRecipient      = errors.New("invalid email recipient")
)

type NotificationService struct {
	alerts     *alerts.Service
	events     *events.Service
	senders    *SenderService
	settings   *emailconfig.Store
	provider   emailprovider.Provider
	dispatcher *notification.Dispatcher
	template   *notification.Template
	timeout    time.Duration
}

func NewNotificationService(alertService *alerts.Service, eventService *events.Service, senders *SenderService, settings *emailconfig.Store, provider emailprovider.Provider, dispatcher *notification.Dispatcher, publicURL string, timeout time.Duration) *NotificationService {
	return &NotificationService{alerts: alertService, events: eventService, senders: senders, settings: settings, provider: provider, dispatcher: dispatcher, template: notification.NewTemplate(publicURL), timeout: timeout}
}

func (s *NotificationService) EmailSettings() (emailconfig.Safe, error) {
	return s.settings.Safe()
}

func (s *NotificationService) UpdateEmailSettings(input emailconfig.Input) (emailconfig.Safe, error) {
	return s.settings.Update(input, time.Now())
}

func (s *NotificationService) TestEmailConnection(ctx context.Context) (domain.EmailProviderType, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	err := s.provider.TestConnection(ctx)
	_ = s.settings.RecordTest(err == nil, providerErrorMessage(err), time.Now())
	return s.provider.Provider(), err
}

func (s *NotificationService) SendTestEmail(ctx context.Context, rawRecipient string) (domain.EmailProviderType, string, error) {
	recipient, valid := validation.EmailAddress(rawRecipient)
	if !valid {
		return s.provider.Provider(), "", ErrInvalidEmailRecipient
	}
	message, err := s.template.RenderProviderTest(recipient, s.provider.Provider())
	if err != nil {
		return s.provider.Provider(), recipient, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return s.provider.Provider(), recipient, s.provider.Send(ctx, message)
}

func (s *NotificationService) EnqueueAlertTest(ctx context.Context, id string) (domain.EmailAlert, domain.Sender, error) {
	alert, err := s.alerts.Get(id)
	if err != nil {
		return alert, domain.Sender{}, err
	}
	if !s.settings.IsReady() {
		return alert, domain.Sender{}, alerts.ErrEmailNotConfigured
	}
	if len(alert.SenderIDs) == 0 {
		return alert, domain.Sender{}, ErrNotificationWithoutSenders
	}
	sender, err := s.senders.Get(ctx, alert.SenderIDs[0])
	if err != nil {
		return alert, sender, err
	}
	severity := domain.Info
	if len(alert.Severities) > 0 {
		severity = alert.Severities[0]
	}
	value := domain.Notification{Alert: alert, Sender: sender, Test: true, Entry: domain.LogEntry{Timestamp: time.Now(), Severity: severity, Message: "LogHill email alert test message.", Metadata: map[string]any{"test": true}}}
	return alert, sender, s.dispatcher.Enqueue(value)
}

func (s *NotificationService) EnqueueEventTest(ctx context.Context, id, rawRecipient string) (domain.EventDefinition, domain.Sender, string, error) {
	recipient, valid := validation.EmailAddress(rawRecipient)
	if !valid {
		return domain.EventDefinition{}, domain.Sender{}, "", ErrInvalidEmailRecipient
	}
	if !s.settings.IsReady() {
		return domain.EventDefinition{}, domain.Sender{}, recipient, events.ErrEmailNotConfigured
	}
	event, err := s.events.Get(id)
	if err != nil {
		return event, domain.Sender{}, recipient, err
	}
	if len(event.SenderIDs) == 0 {
		return event, domain.Sender{}, recipient, ErrNotificationWithoutSenders
	}
	sender, err := s.senders.Get(ctx, event.SenderIDs[0])
	if err != nil {
		return event, sender, recipient, err
	}
	entry := domain.LogEntry{Timestamp: time.Now(), SenderID: sender.ID, Severity: domain.Info, Message: "Sample LogHill event test message.", Event: event.Key, Metadata: map[string]any{"recipient": "customer@example.com", "protocol": "TEST-123", "test": true}}
	value := domain.Notification{SourceType: domain.NotificationSourceEvent, SourceID: event.ID, Event: event, Sender: sender, Recipients: []string{recipient}, Test: true, Entry: entry}
	return event, sender, recipient, s.dispatcher.Enqueue(value)
}

func providerErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var providerError *emailprovider.Error
	if errors.As(err, &providerError) {
		return providerError.Message
	}
	if errors.Is(err, emailprovider.ErrNotConfigured) {
		return "The email provider is not configured."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "The email service did not respond within the expected time."
	}
	return "Unable to complete the operation with the email provider."
}
