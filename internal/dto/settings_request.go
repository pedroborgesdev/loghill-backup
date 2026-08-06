package dto

import "logtheater/internal/domain"

type UpdateSettingsRequest struct {
	LogLimit             *domain.NumberUnitValue `json:"log_limit"`
	InactivePreservation *domain.NumberUnitValue `json:"inactive_preservation"`
	InactiveAfterSeconds *int                    `json:"inactive_after_seconds"`
	DeleteInactiveDays   *int                    `json:"delete_inactive_after_days"`
}
