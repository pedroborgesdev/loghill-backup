package domain

import (
	"encoding/json"
	"time"
)

type EmailProviderType string

const (
	EmailProviderOutlook EmailProviderType = "outlook"
	EmailProviderGmail   EmailProviderType = "gmail"
)

type DeliveryStatus string

const (
	DeliveryPending DeliveryStatus = "pending"
	DeliverySent    DeliveryStatus = "sent"
	DeliveryFailed  DeliveryStatus = "failed"
)

type EmailAlert struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	SenderIDs          []string          `json:"sender_ids"`
	SenderNames        []string          `json:"sender_names"`
	Severities         []LogSeverity     `json:"severities"`
	Recipients         []string          `json:"recipients"`
	Provider           EmailProviderType `json:"provider"`
	Enabled            bool              `json:"enabled"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	LastTriggeredAt    *time.Time        `json:"last_triggered_at"`
	LastDeliveryAt     *time.Time        `json:"last_delivery_at"`
	LastDeliveryStatus *DeliveryStatus   `json:"last_delivery_status"`
	LastDeliveryError  *string           `json:"last_delivery_error"`
	DeliveryCount      int64             `json:"delivery_count"`
	FailureCount       int64             `json:"failure_count"`
	TestDeliveryCount  int64             `json:"test_delivery_count"`
}

// UnmarshalJSON migrates the former single-sender representation in memory.
// The next store write persists only sender_ids and sender_names.
func (a *EmailAlert) UnmarshalJSON(data []byte) error {
	type alias EmailAlert
	value := struct {
		*alias
		SenderID   string `json:"sender_id"`
		SenderName string `json:"sender_name"`
	}{alias: (*alias)(a)}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if len(a.SenderIDs) == 0 && value.SenderID != "" {
		a.SenderIDs = []string{value.SenderID}
	}
	if len(a.SenderNames) == 0 && value.SenderName != "" {
		a.SenderNames = []string{value.SenderName}
	}
	return nil
}

type AlertInput struct {
	Name       string            `json:"name"`
	SenderIDs  []string          `json:"sender_ids"`
	Severities []LogSeverity     `json:"severities"`
	Recipients []string          `json:"recipients"`
	Provider   EmailProviderType `json:"provider"`
	Enabled    bool              `json:"enabled"`
}

type AlertFilters struct {
	SenderID string
	Enabled  *bool
	Severity LogSeverity
	Search   string
	Page     int
	PageSize int
}

type AlertPage struct {
	Items      []EmailAlert `json:"items"`
	Pagination Pagination   `json:"pagination"`
}

type EmailMessage struct {
	To      []string
	Subject string
	Text    string
	HTML    string
}

type Notification struct {
	ExecutionID string
	SourceType  NotificationSourceType
	SourceID    string
	Alert       EmailAlert
	Event       EventDefinition
	Sender      Sender
	Entry       LogEntry
	Recipients  []string
	Test        bool
}

type NotificationSourceType string

const (
	NotificationSourceAlert      NotificationSourceType = "alert"
	NotificationSourceEvent      NotificationSourceType = "event"
	NotificationSourceMonitoring NotificationSourceType = "monitoring"
)
