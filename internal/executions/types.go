package executions

import (
	"time"

	"logtheater/internal/domain"
)

type SourceType string
type Status string

const (
	SourceAlert      SourceType = "alert"
	SourceEvent      SourceType = "event"
	SourceMonitoring SourceType = "monitoring"
	StatusPending    Status     = "pending"
	StatusProcessing Status     = "processing"
	StatusSuccess    Status     = "success"
	StatusPartial    Status     = "partial"
	StatusFailed     Status     = "failed"
	StatusCancelled  Status     = "cancelled"
	StatusSkipped    Status     = "skipped"
)

type ActionResult struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"`
	Status       Status     `json:"status"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	DurationMs   *int64     `json:"duration_ms,omitempty"`
	AttemptCount int        `json:"attempt_count"`
	ErrorCode    *string    `json:"error_code,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
}

type ConditionResult struct {
	ID          string `json:"id"`
	Matched     bool   `json:"matched"`
	Description string `json:"description"`
	Error       string `json:"error,omitempty"`
}

type Record struct {
	ID                 string              `json:"id"`
	SourceType         SourceType          `json:"source_type"`
	SourceID           string              `json:"source_id"`
	SourceName         string              `json:"source_name"`
	SenderID           string              `json:"sender_id"`
	SenderName         string              `json:"sender_name,omitempty"`
	TriggerType        string              `json:"trigger_type"`
	TriggerID          string              `json:"trigger_id,omitempty"`
	TriggerName        string              `json:"trigger_name,omitempty"`
	TriggerMessage     string              `json:"trigger_message,omitempty"`
	Severity           *domain.LogSeverity `json:"severity,omitempty"`
	Status             Status              `json:"status"`
	CorrelationID      string              `json:"correlation_id,omitempty"`
	CausationID        string              `json:"causation_id,omitempty"`
	StartedAt          time.Time           `json:"started_at"`
	FinishedAt         *time.Time          `json:"finished_at,omitempty"`
	DurationMs         *int64              `json:"duration_ms,omitempty"`
	AttemptCount       int                 `json:"attempt_count"`
	Actions            []ActionResult      `json:"actions"`
	Conditions         []ConditionResult   `json:"conditions,omitempty"`
	ErrorCode          *string             `json:"error_code,omitempty"`
	ErrorMessage       *string             `json:"error_message,omitempty"`
	Metadata           map[string]any      `json:"metadata,omitempty"`
	RetryOfExecutionID *string             `json:"retry_of_execution_id,omitempty"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

type Filters struct {
	SourceType                                                 SourceType
	SourceID, SenderID, TriggerType, ActionType, Search, Order string
	Statuses                                                   map[Status]bool
	Severities                                                 map[domain.LogSeverity]bool
	StartedFrom, StartedTo                                     *time.Time
	Recent                                                     bool
	Page, PageSize                                             int
}

type Page struct {
	Items      []Record          `json:"items"`
	Pagination domain.Pagination `json:"pagination"`
}

type Summary struct {
	LastHour              int64 `json:"last_hour"`
	Last24Hours           int64 `json:"last_24_hours"`
	Running               int64 `json:"running"`
	FailedLastHour        int64 `json:"failed_last_hour"`
	AlertsLast24Hours     int64 `json:"alerts_last_24_hours"`
	EventsLast24Hours     int64 `json:"events_last_24_hours"`
	MonitoringLast24Hours int64 `json:"monitoring_last_24_hours"`
}
