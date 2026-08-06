package events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
	"logtheater/internal/repositories"
	"logtheater/internal/validation"
)

const (
	MaxEventName       = 100
	MaxEventKey        = 80
	MaxEventSenders    = 100
	MaxEventRecipients = 20
	MaxEventSubject    = 200
	MaxEventMessage    = 10_000
)

var (
	ErrNotFound           = errors.New("event not found")
	ErrAlreadyExists      = errors.New("event already exists")
	ErrEmailNotConfigured = errors.New("email provider is not configured")
	eventKeyPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,79}$`)
	placeholderPattern    = regexp.MustCompile(`{{\s*([^{}]+?)\s*}}`)
	metadataPattern       = regexp.MustCompile(`^metadata\.[A-Za-z0-9_-]{1,64}$`)
)

var supportedVariables = map[string]bool{
	"event.key": true, "event.name": true,
	"sender.id": true, "sender.name": true, "sender.status": true,
	"log.message": true, "log.severity": true, "log.timestamp": true,
	"app.public_url": true,
}

type ValidationError struct {
	Code, Field, Message string
}

func (e *ValidationError) Error() string { return e.Message }

type Service struct {
	store       *Store
	senders     *repositories.SenderRepository
	emailConfig *emailconfig.Store
	clock       domain.Clock
	writeMu     sync.Mutex
	indexMu     sync.RWMutex
	index       map[string]map[string][]string
}

func NewService(store *Store, senders *repositories.SenderRepository, emailConfig *emailconfig.Store, clock domain.Clock) *Service {
	service := &Service{store: store, senders: senders, emailConfig: emailConfig, clock: clock, index: make(map[string]map[string][]string)}
	service.rebuildIndex()
	return service
}

func ValidKey(value string) bool { return eventKeyPattern.MatchString(value) }

func ValidateLogKey(value string) error {
	if value != "" && !ValidKey(value) {
		return domain.ErrInvalidEventKey
	}
	return nil
}

func (s *Service) List(filters domain.EventFilters) domain.EventPage {
	items := s.store.All()
	filtered := make([]domain.EventDefinition, 0, len(items))
	query := strings.ToLower(strings.TrimSpace(filters.Search))
	for _, event := range items {
		if filters.Enabled != nil && event.Enabled != *filters.Enabled {
			continue
		}
		if filters.SenderID != "" && !contains(event.SenderIDs, filters.SenderID) {
			continue
		}
		if filters.SenderName != "" && !s.matchesSenderName(event.SenderIDs, filters.SenderName) {
			continue
		}
		if filters.ActionType != "" && event.ActionType != filters.ActionType {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(event.Name+" "+event.Key), query) {
			continue
		}
		filtered = append(filtered, event)
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
	return domain.EventPage{Items: filtered[start:end], Pagination: domain.Pagination{Page: filters.Page, PageSize: filters.PageSize, Returned: end - start, Total: int64(total), TotalPages: pages}}
}

func (s *Service) matchesSenderName(ids []string, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	for _, id := range ids {
		sender, err := s.senders.Get(context.Background(), id)
		if err == nil && strings.Contains(strings.ToLower(sender.Name), query) {
			return true
		}
	}
	return false
}

func (s *Service) Summary() map[string]int64 {
	now := s.clock.Now().Add(-24 * time.Hour)
	result := map[string]int64{"total": 0, "active": 0, "recent_triggered": 0, "recent_failures": 0}
	for _, event := range s.store.All() {
		result["total"]++
		if event.Enabled {
			result["active"]++
		}
		if event.LastTriggeredAt != nil && event.LastTriggeredAt.After(now) {
			result["recent_triggered"]++
		}
		if event.LastDeliveryAt != nil && event.LastDeliveryAt.After(now) && event.LastDeliveryStatus != nil && *event.LastDeliveryStatus == domain.DeliveryFailed {
			result["recent_failures"]++
		}
	}
	return result
}

func (s *Service) Get(id string) (domain.EventDefinition, error) {
	event, ok := s.store.Get(strings.TrimSpace(id))
	if !ok {
		return domain.EventDefinition{}, ErrNotFound
	}
	return event, nil
}

func (s *Service) KeyAvailable(key string) bool {
	if !ValidKey(key) {
		return false
	}
	for _, event := range s.store.All() {
		if event.Key == key {
			return false
		}
	}
	return true
}

func (s *Service) Create(ctx context.Context, input domain.EventInput) (domain.EventDefinition, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	input, err := s.validate(ctx, input, true)
	if err != nil {
		return domain.EventDefinition{}, err
	}
	if !s.KeyAvailable(input.Key) {
		return domain.EventDefinition{}, ErrAlreadyExists
	}
	id, err := randomID()
	if err != nil {
		return domain.EventDefinition{}, err
	}
	now := s.clock.Now()
	event := domain.EventDefinition{ID: id, Name: input.Name, Key: input.Key, SenderIDs: input.SenderIDs, ActionType: input.ActionType, Recipients: input.Recipients, SubjectTemplate: input.SubjectTemplate, MessageTemplate: input.MessageTemplate, Enabled: input.Enabled, CreatedAt: now, UpdatedAt: now}
	if err = s.store.Put(event); err != nil {
		return domain.EventDefinition{}, err
	}
	s.rebuildIndex()
	return event, nil
}

func (s *Service) Update(ctx context.Context, id string, input domain.EventInput) (domain.EventDefinition, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	existing, err := s.Get(id)
	if err != nil {
		return domain.EventDefinition{}, err
	}
	if input.Key != "" && input.Key != existing.Key {
		return domain.EventDefinition{}, &ValidationError{Code: "EVENT_KEY_IMMUTABLE", Field: "key", Message: "A chave do evento não pode ser alterada."}
	}
	input.Key = existing.Key
	input, err = s.validate(ctx, input, false)
	if err != nil {
		return domain.EventDefinition{}, err
	}
	existing.Name = input.Name
	existing.SenderIDs = input.SenderIDs
	existing.ActionType = input.ActionType
	existing.Recipients = input.Recipients
	existing.SubjectTemplate = input.SubjectTemplate
	existing.MessageTemplate = input.MessageTemplate
	existing.Enabled = input.Enabled
	existing.UpdatedAt = s.clock.Now()
	if err = s.store.Put(existing); err != nil {
		return domain.EventDefinition{}, err
	}
	s.rebuildIndex()
	return existing, nil
}

func (s *Service) SetEnabled(ctx context.Context, id string, enabled bool) (domain.EventDefinition, error) {
	existing, err := s.Get(id)
	if err != nil {
		return domain.EventDefinition{}, err
	}
	return s.Update(ctx, id, domain.EventInput{Name: existing.Name, Key: existing.Key, SenderIDs: existing.SenderIDs, ActionType: existing.ActionType, Recipients: existing.Recipients, SubjectTemplate: existing.SubjectTemplate, MessageTemplate: existing.MessageTemplate, Enabled: enabled})
}

func (s *Service) Delete(id string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
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

func (s *Service) Matching(senderID, key string) []domain.EventDefinition {
	if key == "" {
		return nil
	}
	s.indexMu.RLock()
	ids := append([]string(nil), s.index[senderID][key]...)
	s.indexMu.RUnlock()
	matches := make([]domain.EventDefinition, 0, len(ids))
	for _, id := range ids {
		event, exists := s.store.Get(id)
		if exists && event.Enabled && event.Key == key && contains(event.SenderIDs, senderID) {
			matches = append(matches, event)
		}
	}
	return matches
}

func (s *Service) MarkPending(id string, test bool) error {
	return s.store.Mutate(id, func(event *domain.EventDefinition) {
		now := s.clock.Now()
		event.LastDeliveryStatus = deliveryPointer(domain.DeliveryPending)
		event.LastDeliveryError = nil
		if !test {
			event.LastTriggeredAt = timePointer(now)
			event.TriggerCount++
		}
	})
}

func (s *Service) RecordTriggered(id string) error {
	return s.store.Mutate(id, func(event *domain.EventDefinition) {
		now := s.clock.Now()
		event.LastTriggeredAt = timePointer(now)
		event.TriggerCount++
	})
}

func (s *Service) RecordDelivery(id string, test bool, status domain.DeliveryStatus, message string) error {
	return s.store.Mutate(id, func(event *domain.EventDefinition) {
		now := s.clock.Now()
		event.LastDeliveryAt = timePointer(now)
		event.LastDeliveryStatus = deliveryPointer(status)
		if message == "" {
			event.LastDeliveryError = nil
		} else {
			event.LastDeliveryError = stringPointer(message)
		}
		if status == domain.DeliverySent {
			if test {
				event.TestDeliveryCount++
			} else {
				event.DeliveryCount++
			}
		} else if status == domain.DeliveryFailed {
			event.FailureCount++
		}
	})
}

func (s *Service) SenderUsageCount(senderID string) int {
	s.indexMu.RLock()
	defer s.indexMu.RUnlock()
	count := 0
	for _, ids := range s.index[senderID] {
		count += len(ids)
	}
	return count
}

func (s *Service) RemoveSender(senderID string) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	affected, err := s.store.RemoveSender(senderID, s.clock.Now())
	if err == nil {
		s.rebuildIndex()
	}
	return affected, err
}

func (s *Service) validate(ctx context.Context, input domain.EventInput, requireKey bool) (domain.EventInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	if length := len([]rune(input.Name)); length < 3 || length > MaxEventName {
		return input, invalid("name", "O nome deve possuir entre 3 e 100 caracteres.")
	}
	if requireKey && input.Key == "" || !ValidKey(input.Key) {
		return input, &ValidationError{Code: "INVALID_EVENT_KEY", Field: "key", Message: "O identificador do evento é inválido."}
	}
	if len(input.SenderIDs) > MaxEventSenders {
		return input, invalid("sender_ids", "Selecione no máximo 100 senders.")
	}
	seenSenders := make(map[string]bool)
	cleanSenders := make([]string, 0, len(input.SenderIDs))
	for _, raw := range input.SenderIDs {
		id := strings.TrimSpace(raw)
		if id == "" || seenSenders[id] {
			return input, invalid("sender_ids", "Os senders devem ser válidos e não podem se repetir.")
		}
		sender, err := s.senders.Get(ctx, id)
		if err != nil || sender.Status == domain.StatusExpired || sender.Status == domain.StatusRevoked {
			return input, invalid("sender_ids", fmt.Sprintf("O sender “%s” não está disponível.", id))
		}
		seenSenders[id] = true
		cleanSenders = append(cleanSenders, id)
	}
	input.SenderIDs = cleanSenders
	if input.ActionType == "" {
		input.ActionType = domain.EventActionEmail
	}
	if input.ActionType != domain.EventActionEmail && input.ActionType != domain.EventActionNone {
		return input, &ValidationError{Code: "EVENT_ACTION_NOT_AVAILABLE", Field: "action_type", Message: "A ação selecionada não está disponível."}
	}
	if input.ActionType == domain.EventActionNone {
		input.Recipients = []string{}
		input.SubjectTemplate = ""
		input.MessageTemplate = ""
		sort.Strings(input.SenderIDs)
		return input, nil
	}
	if len(input.Recipients) == 0 || len(input.Recipients) > MaxEventRecipients {
		return input, invalid("recipients", "Informe de 1 a 20 destinatários.")
	}
	seenRecipients := make(map[string]bool)
	recipients := make([]string, 0, len(input.Recipients))
	for _, raw := range input.Recipients {
		recipient, ok := validation.EmailAddress(raw)
		if !ok {
			return input, invalid("recipients", "Informe apenas endereços de e-mail válidos.")
		}
		if !seenRecipients[recipient] {
			seenRecipients[recipient] = true
			recipients = append(recipients, recipient)
		}
	}
	input.Recipients = recipients
	input.SubjectTemplate = strings.TrimSpace(input.SubjectTemplate)
	if len([]rune(input.SubjectTemplate)) == 0 || len([]rune(input.SubjectTemplate)) > MaxEventSubject || strings.ContainsAny(input.SubjectTemplate, "\r\n") {
		return input, invalid("subject_template", "O assunto é obrigatório, deve ter até 200 caracteres e não pode conter quebras de linha.")
	}
	input.MessageTemplate = strings.TrimSpace(input.MessageTemplate)
	if len([]rune(input.MessageTemplate)) == 0 || len([]rune(input.MessageTemplate)) > MaxEventMessage {
		return input, invalid("message_template", "A mensagem é obrigatória e deve ter até 10.000 caracteres.")
	}
	if variable := unsupportedVariable(input.SubjectTemplate + "\n" + input.MessageTemplate); variable != "" {
		return input, invalid("message_template", fmt.Sprintf("A variável {{%s}} não é suportada.", variable))
	}
	if input.Enabled && !s.emailConfig.IsReady() {
		return input, ErrEmailNotConfigured
	}
	sort.Strings(input.SenderIDs)
	return input, nil
}

func unsupportedVariable(value string) string {
	for _, match := range placeholderPattern.FindAllStringSubmatch(value, -1) {
		name := strings.TrimSpace(match[1])
		if !supportedVariables[name] && !metadataPattern.MatchString(name) {
			return name
		}
	}
	return ""
}

func invalid(field, message string) error {
	return &ValidationError{Code: "INVALID_EVENT", Field: field, Message: message}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func (s *Service) rebuildIndex() {
	next := make(map[string]map[string][]string)
	for _, event := range s.store.All() {
		for _, senderID := range event.SenderIDs {
			if next[senderID] == nil {
				next[senderID] = make(map[string][]string)
			}
			next[senderID][event.Key] = append(next[senderID][event.Key], event.ID)
		}
	}
	s.indexMu.Lock()
	s.index = next
	s.indexMu.Unlock()
}

func randomID() (string, error) {
	data := make([]byte, 6)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return "evt_" + hex.EncodeToString(data), nil
}

func deliveryPointer(value domain.DeliveryStatus) *domain.DeliveryStatus { return &value }
func stringPointer(value string) *string                                 { return &value }
func timePointer(value time.Time) *time.Time                             { return &value }
