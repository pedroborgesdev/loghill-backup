package dto

import "time"

type CreateSenderRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type UpdateSenderRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ReceiveLogRequest struct {
	Sender            string         `json:"sender" binding:"required"`
	Severity          string         `json:"severity" binding:"required"`
	Message           string         `json:"message" binding:"required"`
	Timestamp         *time.Time     `json:"timestamp"`
	Event             string         `json:"event"`
	EventOccurrenceID string         `json:"event_occurrence_id"`
	Metadata          map[string]any `json:"metadata"`
}
