package domain

import "time"

type EventActionType string

const EventActionEmail EventActionType = "email"

type EventDefinition struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Key                string          `json:"key"`
	SenderIDs          []string        `json:"sender_ids"`
	ActionType         EventActionType `json:"action_type"`
	Recipients         []string        `json:"recipients"`
	SubjectTemplate    string          `json:"subject_template"`
	MessageTemplate    string          `json:"message_template"`
	Enabled            bool            `json:"enabled"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	LastTriggeredAt    *time.Time      `json:"last_triggered_at"`
	LastDeliveryAt     *time.Time      `json:"last_delivery_at"`
	LastDeliveryStatus *DeliveryStatus `json:"last_delivery_status"`
	LastDeliveryError  *string         `json:"last_delivery_error"`
	TriggerCount       int64           `json:"trigger_count"`
	DeliveryCount      int64           `json:"delivery_count"`
	FailureCount       int64           `json:"failure_count"`
	TestDeliveryCount  int64           `json:"test_delivery_count"`
}

type EventInput struct {
	Name            string          `json:"name"`
	Key             string          `json:"key,omitempty"`
	SenderIDs       []string        `json:"sender_ids"`
	ActionType      EventActionType `json:"action_type"`
	Recipients      []string        `json:"recipients"`
	SubjectTemplate string          `json:"subject_template"`
	MessageTemplate string          `json:"message_template"`
	Enabled         bool            `json:"enabled"`
}

type EventFilters struct {
	Search     string
	SenderID   string
	ActionType EventActionType
	Enabled    *bool
	Page       int
	PageSize   int
}

type EventPage struct {
	Items      []EventDefinition `json:"items"`
	Pagination Pagination        `json:"pagination"`
}
