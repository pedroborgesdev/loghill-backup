package alerts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"logmate/internal/domain"
	"logmate/internal/emailconfig"
	"logmate/internal/repositories"
	"logmate/internal/validation"
)

var (
	ErrNotFound           = errors.New("alert not found")
	ErrEmailNotConfigured = errors.New("email provider is not configured")
)

type ValidationError struct {
	Code, Field, Message string
}

func (e *ValidationError) Error() string { return e.Message }

type Service struct {
	store       *Store
	senders     repositories.SenderRepository
	emailConfig *emailconfig.Store
	clock       domain.Clock
	indexMu     sync.RWMutex
	index       map[string][]string
}

func NewService(store *Store, senders repositories.SenderRepository, emailConfig *emailconfig.Store, clock domain.Clock) *Service {
	service := &Service{store: store, senders: senders, emailConfig: emailConfig, clock: clock, index: make(map[string][]string)}
	service.rebuildIndex()
	return service
}

func (s *Service) List(filters domain.AlertFilters) domain.AlertPage {
	items := s.store.All()
	filtered := make([]domain.EmailAlert, 0, len(items))
	query := strings.ToLower(strings.TrimSpace(filters.Search))
	for _, alert := range items {
		if filters.SenderID != "" && !containsString(alert.SenderIDs, filters.SenderID) {
			continue
		}
		if filters.Enabled != nil && alert.Enabled != *filters.Enabled {
			continue
		}
		if filters.Severity != "" && !containsSeverity(alert.Severities, filters.Severity) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(alert.Name+" "+strings.Join(alert.SenderNames, " ")+" "+strings.Join(alert.Recipients, " ")), query) {
			continue
		}
		filtered = append(filtered, alert)
	}
	if filters.Page < 1 {
		filters.Page = 1
	}
	if filters.PageSize < 1 {
		filters.PageSize = 20
	}
	total := len(filtered)
	start := (filters.Page - 1) * filters.PageSize
	if start > total {
		start = total
	}
	end := start + filters.PageSize
	if end > total {
		end = total
	}
	pages := (total + filters.PageSize - 1) / filters.PageSize
	return domain.AlertPage{Items: filtered[start:end], Pagination: domain.Pagination{Page: filters.Page, PageSize: filters.PageSize, Returned: end - start, Total: int64(total), TotalPages: pages}}
}

func (s *Service) Summary() map[string]int64 {
	result := map[string]int64{"total": 0, "active": 0, "recent_failures": 0}
	for _, alert := range s.store.All() {
		result["total"]++
		if alert.Enabled {
			result["active"]++
		}
		if alert.LastDeliveryStatus != nil && *alert.LastDeliveryStatus == domain.DeliveryFailed {
			result["recent_failures"]++
		}
	}
	return result
}

func (s *Service) Get(id string) (domain.EmailAlert, error) {
	alert, ok := s.store.Get(strings.TrimSpace(id))
	if !ok {
		return domain.EmailAlert{}, ErrNotFound
	}
	return alert, nil
}

func (s *Service) Create(ctx context.Context, input domain.AlertInput) (domain.EmailAlert, error) {
	input, senders, err := s.validate(ctx, input)
	if err != nil {
		return domain.EmailAlert{}, err
	}
	id, err := randomID()
	if err != nil {
		return domain.EmailAlert{}, err
	}
	now := s.clock.Now()
	alert := domain.EmailAlert{
		ID: id, Name: input.Name, SenderIDs: senderIDs(senders), SenderNames: senderNames(senders),
		Severities: input.Severities, Recipients: input.Recipients, Provider: input.Provider,
		Enabled: input.Enabled, CreatedAt: now, UpdatedAt: now,
	}
	if err = s.store.Put(alert); err != nil {
		return domain.EmailAlert{}, err
	}
	s.rebuildIndex()
	return alert, nil
}

func (s *Service) Update(ctx context.Context, id string, input domain.AlertInput) (domain.EmailAlert, error) {
	existing, err := s.Get(id)
	if err != nil {
		return domain.EmailAlert{}, err
	}
	input, senders, err := s.validate(ctx, input)
	if err != nil {
		return domain.EmailAlert{}, err
	}
	existing.Name = input.Name
	existing.SenderIDs = senderIDs(senders)
	existing.SenderNames = senderNames(senders)
	existing.Severities = input.Severities
	existing.Recipients = input.Recipients
	existing.Provider = input.Provider
	existing.Enabled = input.Enabled
	existing.UpdatedAt = s.clock.Now()
	if err = s.store.Put(existing); err != nil {
		return domain.EmailAlert{}, err
	}
	s.rebuildIndex()
	return existing, nil
}

func (s *Service) Delete(id string) error {
	deleted, err := s.store.Delete(id)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrNotFound
	}
	s.rebuildIndex()
	return nil
}

func (s *Service) Matching(senderID string, severity domain.LogSeverity) []domain.EmailAlert {
	s.indexMu.RLock()
	ids := append([]string(nil), s.index[senderID]...)
	s.indexMu.RUnlock()
	matches := make([]domain.EmailAlert, 0)
	for _, id := range ids {
		alert, exists := s.store.Get(id)
		if exists && alert.Enabled && containsSeverity(alert.Severities, severity) {
			matches = append(matches, alert)
		}
	}
	return matches
}

func (s *Service) MarkPending(id string, test bool) error {
	return s.store.Mutate(id, func(alert *domain.EmailAlert) {
		now := s.clock.Now()
		alert.LastDeliveryStatus = deliveryPointer(domain.DeliveryPending)
		alert.LastDeliveryError = nil
		if !test {
			alert.LastTriggeredAt = timePointer(now)
		}
	})
}

func (s *Service) RecordDelivery(id string, test bool, status domain.DeliveryStatus, message string) error {
	return s.store.Mutate(id, func(alert *domain.EmailAlert) {
		now := s.clock.Now()
		alert.LastDeliveryAt = timePointer(now)
		alert.LastDeliveryStatus = deliveryPointer(status)
		if message == "" {
			alert.LastDeliveryError = nil
		} else {
			alert.LastDeliveryError = stringPointer(message)
		}
		if status == domain.DeliverySent {
			if test {
				alert.TestDeliveryCount++
			} else {
				alert.DeliveryCount++
			}
		} else if status == domain.DeliveryFailed {
			alert.FailureCount++
		}
	})
}

func (s *Service) validate(ctx context.Context, input domain.AlertInput) (domain.AlertInput, []domain.Sender, error) {
	input.Name = strings.TrimSpace(input.Name)
	if len([]rune(input.Name)) < 3 || len([]rune(input.Name)) > 100 {
		return input, nil, invalid("name", "The name must be between 3 and 100 characters.")
	}
	uniqueSenderIDs := make(map[string]bool)
	senders := make([]domain.Sender, 0, len(input.SenderIDs))
	rawSenderIDs := append([]string(nil), input.SenderIDs...)
	input.SenderIDs = input.SenderIDs[:0]
	for _, rawID := range rawSenderIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return input, senders, invalid("sender_ids", "Sender identifiers are required.")
		}
		if uniqueSenderIDs[id] {
			return input, senders, invalid("sender_ids", "A sender cannot be selected more than once.")
		}
		uniqueSenderIDs[id] = true
		sender, err := s.senders.Get(ctx, id)
		if err != nil || sender.Status == domain.StatusExpired || sender.Status == domain.StatusRevoked {
			return input, nil, &ValidationError{Code: "INVALID_ALERT", Field: "sender_ids", Message: fmt.Sprintf("Sender “%s” is no longer available.", id)}
		}
		input.SenderIDs = append(input.SenderIDs, id)
		senders = append(senders, sender)
	}
	if len(input.Severities) == 0 {
		return input, senders, invalid("severities", "Select at least one severity.")
	}
	severitySet := make(map[domain.LogSeverity]bool)
	severities := make([]domain.LogSeverity, 0, len(input.Severities))
	for _, raw := range input.Severities {
		severity, parseErr := domain.ParseSeverity(string(raw))
		if parseErr != nil {
			return input, senders, invalid("severities", "One of the severities is invalid.")
		}
		if !severitySet[severity] {
			severitySet[severity] = true
			severities = append(severities, severity)
		}
	}
	if len(input.Recipients) == 0 || len(input.Recipients) > 20 {
		return input, senders, invalid("recipients", "Enter between 1 and 20 recipients.")
	}
	recipientSet := make(map[string]bool)
	recipients := make([]string, 0, len(input.Recipients))
	for _, raw := range input.Recipients {
		recipient, valid := validation.EmailAddress(raw)
		if !valid {
			return input, senders, invalid("recipients", "Enter valid email addresses only.")
		}
		if !recipientSet[recipient] {
			recipientSet[recipient] = true
			recipients = append(recipients, recipient)
		}
	}
	if runtime, err := s.emailConfig.Runtime(); err == nil {
		input.Provider = runtime.Provider
	}
	if input.Enabled && !s.emailConfig.IsReady() {
		return input, senders, ErrEmailNotConfigured
	}
	input.Severities = severities
	input.Recipients = recipients
	sort.Slice(input.Severities, func(i, j int) bool { return input.Severities[i] < input.Severities[j] })
	return input, senders, nil
}

func invalid(field, message string) error {
	return &ValidationError{Code: "INVALID_ALERT", Field: field, Message: message}
}

func containsSeverity(values []domain.LogSeverity, expected domain.LogSeverity) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func senderIDs(values []domain.Sender) []string {
	result := make([]string, 0, len(values))
	for _, sender := range values {
		result = append(result, sender.ID)
	}
	return result
}

func senderNames(values []domain.Sender) []string {
	result := make([]string, 0, len(values))
	for _, sender := range values {
		result = append(result, sender.Name)
	}
	return result
}

func (s *Service) rebuildIndex() {
	next := make(map[string][]string)
	for _, alert := range s.store.All() {
		for _, senderID := range alert.SenderIDs {
			next[senderID] = append(next[senderID], alert.ID)
		}
	}
	s.indexMu.Lock()
	s.index = next
	s.indexMu.Unlock()
}

func (s *Service) SenderUsageCount(senderID string) int {
	s.indexMu.RLock()
	defer s.indexMu.RUnlock()
	return len(s.index[senderID])
}

func (s *Service) RemoveSender(senderID string) (int, error) {
	affected, err := s.store.RemoveSender(senderID, s.clock.Now())
	if err == nil {
		s.rebuildIndex()
	}
	return affected, err
}

func (s *Service) RenameSender(senderID, name string) error {
	return s.store.RenameSender(senderID, name, s.clock.Now())
}

func randomID() (string, error) {
	data := make([]byte, 8)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return "alert-" + hex.EncodeToString(data), nil
}
