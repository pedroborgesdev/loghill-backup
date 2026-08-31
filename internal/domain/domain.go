package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type SenderStatus string

const (
	StatusNeverConnected SenderStatus = "never_connected"
	StatusOnline         SenderStatus = "online"
	StatusInactive       SenderStatus = "inactive"
	StatusArchived       SenderStatus = "archived"
	StatusExpired        SenderStatus = "expired"
	StatusRevoked        SenderStatus = "revoked"
)

type Sender struct {
	ID                string       `json:"id"`
	Name              string       `json:"name"`
	Description       string       `json:"description,omitempty"`
	KeyHash           string       `json:"-"`
	KeyPrefix         string       `json:"key_prefix,omitempty"`
	KeyRotatedAt      *time.Time   `json:"key_rotated_at,omitempty"`
	Status            SenderStatus `json:"status"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
	LastActivityAt    *time.Time   `json:"last_activity_at"`
	LastHealthcheckAt *time.Time   `json:"last_healthcheck_at"`
	InactiveAt        *time.Time   `json:"inactive_at"`
	CompactedAt       *time.Time   `json:"compacted_at"`
	ExpiresAt         *time.Time   `json:"expires_at"`
	ExpiredAt         *time.Time   `json:"expired_at"`
	LogLineCount      int64        `json:"log_line_count"`
	LogFileSize       int64        `json:"log_file_size"`
	RecentErrorCount  int64        `json:"recent_error_count,omitempty"`
	InstanceCount     int          `json:"instance_count"`
}

type LogSeverity string

const (
	Undefined LogSeverity = "UNDEFINED"
	Trace     LogSeverity = "TRACE"
	Debug     LogSeverity = "DEBUG"
	Info      LogSeverity = "INFO"
	Warn      LogSeverity = "WARN"
	Error     LogSeverity = "ERROR"
	Fatal     LogSeverity = "FATAL"
)

func ParseSeverity(v string) (LogSeverity, error) {
	s := LogSeverity(strings.ToUpper(strings.TrimSpace(v)))
	switch s {
	case Undefined, Trace, Debug, Info, Warn, Error, Fatal:
		return s, nil
	}
	return "", ErrInvalidSeverity
}

type LogEntry struct {
	Timestamp         time.Time      `json:"timestamp"`
	ActivityAt        time.Time      `json:"-"`
	SenderID          string         `json:"sender,omitempty"`
	InstanceID        string         `json:"instance_id,omitempty"`
	Severity          LogSeverity    `json:"severity"`
	Message           string         `json:"message"`
	Event             string         `json:"event,omitempty"`
	EventOccurrenceID string         `json:"event_occurrence_id,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}
type Pagination struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Returned   int   `json:"returned,omitempty"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}
type LogPage struct {
	Sender     string     `json:"sender"`
	Items      []LogEntry `json:"items"`
	Pagination Pagination `json:"pagination"`
}
type SenderPage struct {
	Items      []Sender   `json:"items"`
	Pagination Pagination `json:"pagination"`
}
type LogFilters struct {
	Severities     map[LogSeverity]bool
	Search         string
	InstanceID     string
	EventMode      string
	EventKey       string
	Start, End     *time.Time
	Page, PageSize int
	Order          string
}

type SenderInstance struct {
	ID                string       `json:"id"`
	TokenHash         string       `json:"-"`
	CreatedAt         time.Time    `json:"created_at"`
	LastActivityAt    *time.Time   `json:"last_activity_at,omitempty"`
	LastHealthcheckAt *time.Time   `json:"last_healthcheck_at,omitempty"`
	LogLineCount      int64        `json:"log_line_count"`
	LogFileSize       int64        `json:"log_file_size"`
	Legacy            bool         `json:"legacy,omitempty"`
	Status            SenderStatus `json:"status"`
}

type SenderInstancePage struct {
	Sender     string           `json:"sender"`
	Items      []SenderInstance `json:"items"`
	Pagination Pagination       `json:"pagination"`
}
type SenderFilters struct {
	Status                    SenderStatus
	Name, Search, Sort, Order string
	HasErrors                 bool
	GroupByName               bool
	Page, PageSize            int
}

type StorageUnit string

const (
	StorageLines StorageUnit = "lines"
	StorageMB    StorageUnit = "mb"
)

type NumberUnitValue struct {
	Value int         `json:"value"`
	Unit  StorageUnit `json:"unit"`
}

type Settings struct {
	LogLimit             NumberUnitValue `json:"log_limit"`
	InactivePreservation NumberUnitValue `json:"inactive_preservation"`
	InactiveAfterSeconds int             `json:"inactive_after_seconds"`
	DeleteInactiveDays   int             `json:"delete_inactive_after_days"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

type SettingsValidationError struct {
	Field   string
	Message string
}

func (e *SettingsValidationError) Error() string { return e.Message }

func DefaultSettings(now time.Time) Settings {
	return Settings{
		LogLimit:             NumberUnitValue{Value: 10_000, Unit: StorageLines},
		InactivePreservation: NumberUnitValue{Value: 2_000, Unit: StorageLines},
		InactiveAfterSeconds: 300,
		DeleteInactiveDays:   7,
		UpdatedAt:            now,
	}
}

func ValidateSettings(value Settings) error {
	if err := validateNumberUnit("log_limit", value.LogLimit, false); err != nil {
		return err
	}
	if err := validateNumberUnit("inactive_preservation", value.InactivePreservation, false); err != nil {
		return err
	}
	if value.LogLimit.Unit == value.InactivePreservation.Unit &&
		value.LogLimit.Value > 0 &&
		value.InactivePreservation.Value > value.LogLimit.Value {
		return &SettingsValidationError{
			Field:   "inactive_preservation.value",
			Message: "The retained amount cannot exceed the maximum limit.",
		}
	}
	if value.InactiveAfterSeconds < 1 || value.InactiveAfterSeconds > 86_400 {
		return &SettingsValidationError{Field: "inactive_after_seconds", Message: "Enter a duration between 1 and 86,400 seconds."}
	}
	if value.DeleteInactiveDays < 1 || value.DeleteInactiveDays > 3_650 {
		return &SettingsValidationError{Field: "delete_inactive_after_days", Message: "Enter a period between 1 and 3,650 days."}
	}
	return nil
}

// ValidateStoredSettings accepts an old value above the current UI limit so it
// can be shown and corrected without silently truncating persisted data.
func ValidateStoredSettings(value Settings) error {
	if err := validateNumberUnit("log_limit", value.LogLimit, true); err != nil {
		return err
	}
	if err := validateNumberUnit("inactive_preservation", value.InactivePreservation, true); err != nil {
		return err
	}
	if value.LogLimit.Unit == value.InactivePreservation.Unit &&
		value.LogLimit.Value > 0 &&
		value.InactivePreservation.Value > value.LogLimit.Value {
		return &SettingsValidationError{
			Field:   "inactive_preservation.value",
			Message: "The retained amount cannot exceed the maximum limit.",
		}
	}
	return nil
}

func validateNumberUnit(field string, value NumberUnitValue, allowLegacy bool) error {
	if value.Unit != StorageLines && value.Unit != StorageMB {
		return &SettingsValidationError{Field: field + ".unit", Message: "Invalid unit."}
	}
	if value.Value < 0 || (!allowLegacy && value.Value > 10_000) {
		return &SettingsValidationError{
			Field:   field + ".value",
			Message: fmt.Sprintf("Enter a value between 0 and %s.", "10.000"),
		}
	}
	return nil
}

var (
	ErrNotFound                 = errors.New("sender not found")
	ErrExpired                  = errors.New("sender expired")
	ErrLogFileNotFound          = errors.New("log file not found")
	ErrInvalidSeverity          = errors.New("invalid severity")
	ErrInvalidName              = errors.New("invalid sender name")
	ErrSenderAlreadyExists      = errors.New("sender already exists")
	ErrInvalidSenderKey         = errors.New("invalid sender key")
	ErrInvalidInstanceToken     = errors.New("invalid instance token")
	ErrSenderRevoked            = errors.New("sender revoked")
	ErrConflict                 = errors.New("conflict")
	ErrTooManySubscribers       = errors.New("too many subscribers")
	ErrInvalidSettings          = errors.New("invalid settings")
	ErrInvalidEventKey          = errors.New("invalid event key")
	ErrInvalidEventOccurrenceID = errors.New("invalid event occurrence id")
)

type Clock interface{ Now() time.Time }
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }
