package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"logmate/internal/config"
	"logmate/internal/domain"
	"logmate/internal/repositories"
	settingsstore "logmate/internal/settings"
)

var (
	namePattern     = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	eventKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{2,79}$`)
	instancePattern = regexp.MustCompile(`^ins_[a-f0-9]{32}$`)
)

type SenderValidationError struct {
	Field, Message string
}

func (e *SenderValidationError) Error() string { return e.Message }

type SenderCredentials struct {
	SenderKey     string `json:"sender_key"`
	DisplayedOnce bool   `json:"displayed_once"`
}

type Service struct {
	repo           repositories.SenderRepository
	cfg            config.Config
	clock          domain.Clock
	settings       *settingsstore.Store
	Hub            *Hub
	started        time.Time
	alertSink      LogAlertSink
	eventSink      LogEventSink
	monitoringSink LogMonitoringSink
}

type SenderService = Service

type LogAlertSink interface {
	Notify(context.Context, domain.Sender, domain.LogEntry)
}

type LogEventSink interface {
	NotifyEvent(context.Context, domain.Sender, domain.LogEntry)
}

type LogMonitoringSink interface {
	Notify(context.Context, domain.Sender, domain.LogEntry)
	NotifySenderStatus(context.Context, domain.Sender, domain.SenderStatus)
	NotifySenderCreated(context.Context, domain.Sender)
}

func New(repo repositories.SenderRepository, cfg config.Config, clock domain.Clock, settings *settingsstore.Store) *Service {
	return &Service{repo: repo, cfg: cfg, clock: clock, settings: settings, Hub: NewHub(cfg.SSEMaxClients, cfg.SSEBuffer), started: clock.Now()}
}

func NewSenderService(repo repositories.SenderRepository, cfg config.Config, clock domain.Clock, settings *settingsstore.Store) *SenderService {
	return New(repo, cfg, clock, settings)
}

func (s *Service) SetAlertSink(sink LogAlertSink)           { s.alertSink = sink }
func (s *Service) SetEventSink(sink LogEventSink)           { s.eventSink = sink }
func (s *Service) SetMonitoringSink(sink LogMonitoringSink) { s.monitoringSink = sink }
func NormalizeName(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	var normalized strings.Builder
	lastHyphen := false
	for _, current := range norm.NFD.String(value) {
		if unicode.Is(unicode.Mn, current) {
			continue
		}
		valid := current >= 'a' && current <= 'z' || current >= '0' && current <= '9'
		if valid {
			normalized.WriteRune(current)
			lastHyphen = false
			continue
		}
		if !lastHyphen && normalized.Len() > 0 {
			normalized.WriteByte('-')
			lastHyphen = true
		}
	}
	result := strings.Trim(normalized.String(), "-")
	if len(result) > 63 {
		result = strings.TrimRight(result[:63], "-")
	}
	if !namePattern.MatchString(result) {
		return "", domain.ErrInvalidName
	}
	return result, nil
}

func normalizeDisplayName(value string) (string, error) {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	length := len([]rune(value))
	if length < 3 || length > 80 {
		return "", &SenderValidationError{Field: "name", Message: "The name must be between 3 and 80 characters."}
	}
	return value, nil
}

func generateSenderKey() (string, string, string, error) {
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return "", "", "", err
	}
	key := "snd_" + base64.RawURLEncoding.EncodeToString(random)
	sum := sha256.Sum256([]byte(key))
	return key, hex.EncodeToString(sum[:]), key[:12] + "...", nil
}

func generateInstanceID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "ins_" + hex.EncodeToString(random), nil
}

func generateInstanceToken() (string, string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", "", err
	}
	token := "inst_" + base64.RawURLEncoding.EncodeToString(random)
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:]), nil
}

func senderKeyMatches(expectedHash, candidate string) bool {
	if expectedHash == "" || candidate == "" || len(candidate) > 256 {
		return false
	}
	sum := sha256.Sum256([]byte(candidate))
	actual := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(expectedHash), []byte(actual)) == 1
}

func instanceTokenMatches(expectedHash, candidate string) bool {
	if expectedHash == "" || candidate == "" || len(candidate) > 256 {
		return false
	}
	sum := sha256.Sum256([]byte(candidate))
	actual := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(expectedHash), []byte(actual)) == 1
}

func (s *Service) CreateSender(ctx context.Context, name, description string) (domain.Sender, SenderCredentials, error) {
	displayName, err := normalizeDisplayName(name)
	if err != nil {
		return domain.Sender{}, SenderCredentials{}, err
	}
	id, err := NormalizeName(displayName)
	if err != nil {
		return domain.Sender{}, SenderCredentials{}, &SenderValidationError{Field: "name", Message: "The name does not produce a valid identifier."}
	}
	description = strings.TrimSpace(description)
	if len([]rune(description)) > 250 {
		return domain.Sender{}, SenderCredentials{}, &SenderValidationError{Field: "description", Message: "The description must be at most 250 characters."}
	}
	key, hash, prefix, err := generateSenderKey()
	if err != nil {
		return domain.Sender{}, SenderCredentials{}, err
	}
	now := s.clock.Now()
	item := domain.Sender{ID: id, Name: displayName, Description: description, KeyHash: hash, KeyPrefix: prefix, Status: domain.StatusNeverConnected, CreatedAt: now, UpdatedAt: now}
	if err = s.repo.Create(ctx, item); errors.Is(err, domain.ErrConflict) {
		return domain.Sender{}, SenderCredentials{}, domain.ErrSenderAlreadyExists
	} else if err != nil {
		return domain.Sender{}, SenderCredentials{}, err
	}
	if s.monitoringSink != nil {
		s.monitoringSink.NotifySenderCreated(ctx, item)
	}
	return item, SenderCredentials{SenderKey: key, DisplayedOnce: true}, nil
}

// InitSender is kept as an internal compatibility helper. No HTTP route exposes it.
func (s *Service) InitSender(ctx context.Context, name string) (domain.Sender, error) {
	item, _, err := s.CreateSender(ctx, name, "")
	return item, err
}

func (s *Service) SenderIDAvailable(ctx context.Context, id string) bool {
	if !namePattern.MatchString(id) {
		return false
	}
	_, err := s.repo.Get(ctx, id)
	return errors.Is(err, domain.ErrNotFound)
}

func (s *Service) UpdateSender(ctx context.Context, id, name, description string) (domain.Sender, error) {
	displayName, err := normalizeDisplayName(name)
	if err != nil {
		return domain.Sender{}, err
	}
	description = strings.TrimSpace(description)
	if len([]rune(description)) > 250 {
		return domain.Sender{}, &SenderValidationError{Field: "description", Message: "The description must be at most 250 characters."}
	}
	ctx, release, err := s.repo.LockSender(ctx, id)
	if err != nil {
		return domain.Sender{}, err
	}
	defer release()
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return item, err
	}
	item.Name = displayName
	item.Description = description
	item.UpdatedAt = s.clock.Now()
	return item, s.repo.Update(ctx, item)
}

func (s *Service) RotateSenderKey(ctx context.Context, id string) (domain.Sender, SenderCredentials, time.Time, error) {
	ctx, release, err := s.repo.LockSender(ctx, id)
	if err != nil {
		return domain.Sender{}, SenderCredentials{}, time.Time{}, err
	}
	defer release()
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return item, SenderCredentials{}, time.Time{}, err
	}
	if item.Status == domain.StatusRevoked {
		return item, SenderCredentials{}, time.Time{}, domain.ErrSenderRevoked
	}
	if item.Status == domain.StatusExpired {
		return item, SenderCredentials{}, time.Time{}, domain.ErrExpired
	}
	key, hash, prefix, err := generateSenderKey()
	if err != nil {
		return item, SenderCredentials{}, time.Time{}, err
	}
	now := s.clock.Now()
	item.KeyHash, item.KeyPrefix, item.KeyRotatedAt, item.UpdatedAt = hash, prefix, &now, now
	if err = s.repo.Update(ctx, item); err != nil {
		return item, SenderCredentials{}, time.Time{}, err
	}
	return item, SenderCredentials{SenderKey: key, DisplayedOnce: true}, now, nil
}

func (s *Service) RevokeSender(ctx context.Context, id string) (domain.Sender, error) {
	ctx, release, err := s.repo.LockSender(ctx, id)
	if err != nil {
		return domain.Sender{}, err
	}
	defer release()
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return item, err
	}
	item.KeyHash, item.KeyPrefix = "", ""
	item.Status = domain.StatusRevoked
	item.UpdatedAt = s.clock.Now()
	return item, s.repo.Update(ctx, item)
}

func (s *Service) ReactivateSender(ctx context.Context, id string) (domain.Sender, SenderCredentials, error) {
	ctx, release, err := s.repo.LockSender(ctx, id)
	if err != nil {
		return domain.Sender{}, SenderCredentials{}, err
	}
	defer release()
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return item, SenderCredentials{}, err
	}
	if item.Status != domain.StatusRevoked {
		return item, SenderCredentials{}, domain.ErrConflict
	}
	key, hash, prefix, err := generateSenderKey()
	if err != nil {
		return item, SenderCredentials{}, err
	}
	now := s.clock.Now()
	item.KeyHash, item.KeyPrefix, item.KeyRotatedAt = hash, prefix, &now
	item.Status, item.UpdatedAt, item.LastActivityAt, item.LastHealthcheckAt = domain.StatusNeverConnected, now, nil, nil
	item.InactiveAt, item.ExpiresAt, item.ExpiredAt = nil, nil, nil
	if err = s.repo.Update(ctx, item); err != nil {
		return item, SenderCredentials{}, err
	}
	return item, SenderCredentials{SenderKey: key, DisplayedOnce: true}, nil
}

func authenticateSender(item domain.Sender, key string) error {
	if item.Status == domain.StatusRevoked || item.Status == domain.StatusExpired || !senderKeyMatches(item.KeyHash, key) {
		return domain.ErrInvalidSenderKey
	}
	return nil
}
func (s *Service) Get(ctx context.Context, id string) (domain.Sender, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) InitInstance(ctx context.Context, senderID, senderKey string) (domain.SenderInstance, error) {
	ctx, release, err := s.repo.LockSender(ctx, senderID)
	if err != nil {
		return domain.SenderInstance{}, err
	}
	defer release()
	sender, err := s.repo.Get(ctx, senderID)
	if err != nil {
		return domain.SenderInstance{}, domain.ErrInvalidSenderKey
	}
	if err = authenticateSender(sender, senderKey); err != nil {
		return domain.SenderInstance{}, err
	}
	id, err := generateInstanceID()
	if err != nil {
		return domain.SenderInstance{}, err
	}
	instance := domain.SenderInstance{ID: id, CreatedAt: s.clock.Now()}
	if err = s.repo.RegisterInstance(ctx, senderID, instance); err != nil {
		return domain.SenderInstance{}, err
	}
	return instance, nil
}

func (s *Service) InitInstanceByName(ctx context.Context, senderName string) (domain.Sender, domain.SenderInstance, string, error) {
	senderID, err := NormalizeName(senderName)
	if err != nil {
		return domain.Sender{}, domain.SenderInstance{}, "", err
	}
	if _, getErr := s.repo.Get(ctx, senderID); errors.Is(getErr, domain.ErrNotFound) {
		items, listErr := s.repo.All(ctx)
		if listErr != nil {
			return domain.Sender{}, domain.SenderInstance{}, "", listErr
		}
		matchedID := ""
		for _, item := range items {
			if strings.EqualFold(strings.Join(strings.Fields(item.Name), " "), strings.Join(strings.Fields(senderName), " ")) {
				if matchedID != "" {
					return domain.Sender{}, domain.SenderInstance{}, "", domain.ErrConflict
				}
				matchedID = item.ID
			}
		}
		if matchedID == "" {
			created, _, createErr := s.CreateSender(ctx, senderName, "")
			switch {
			case createErr == nil:
				senderID = created.ID
			case errors.Is(createErr, domain.ErrSenderAlreadyExists):
				// Another initialization with the same name won the creation race. The
				// repository serializes writes per sender, so it is now safe
				// carregar o registro persistido abaixo.
			default:
				return domain.Sender{}, domain.SenderInstance{}, "", createErr
			}
		} else {
			senderID = matchedID
		}
	} else if getErr != nil {
		return domain.Sender{}, domain.SenderInstance{}, "", getErr
	}
	ctx, release, err := s.repo.LockSender(ctx, senderID)
	if err != nil {
		return domain.Sender{}, domain.SenderInstance{}, "", err
	}
	defer release()
	sender, err := s.repo.Get(ctx, senderID)
	if err != nil {
		return domain.Sender{}, domain.SenderInstance{}, "", err
	}
	if sender.Status == domain.StatusRevoked {
		return domain.Sender{}, domain.SenderInstance{}, "", domain.ErrSenderRevoked
	}
	if sender.Status == domain.StatusExpired {
		return domain.Sender{}, domain.SenderInstance{}, "", domain.ErrExpired
	}
	id, err := generateInstanceID()
	if err != nil {
		return domain.Sender{}, domain.SenderInstance{}, "", err
	}
	token, tokenHash, err := generateInstanceToken()
	if err != nil {
		return domain.Sender{}, domain.SenderInstance{}, "", err
	}
	instance := domain.SenderInstance{ID: id, TokenHash: tokenHash, CreatedAt: s.clock.Now()}
	if err = s.repo.RegisterInstance(ctx, senderID, instance); err != nil {
		return domain.Sender{}, domain.SenderInstance{}, "", err
	}
	return sender, instance, token, nil
}

func (s *Service) InitInstanceByKey(ctx context.Context, senderKey string) (domain.Sender, domain.SenderInstance, error) {
	items, err := s.repo.All(ctx)
	if err != nil {
		return domain.Sender{}, domain.SenderInstance{}, err
	}
	for _, sender := range items {
		if authenticateSender(sender, senderKey) != nil {
			continue
		}
		instance, initErr := s.InitInstance(ctx, sender.ID, senderKey)
		return sender, instance, initErr
	}
	return domain.Sender{}, domain.SenderInstance{}, domain.ErrInvalidSenderKey
}

func instanceActivity(instance domain.SenderInstance) time.Time {
	activity := instance.CreatedAt
	if instance.LastActivityAt != nil {
		activity = *instance.LastActivityAt
	}
	if instance.LastHealthcheckAt != nil && instance.LastHealthcheckAt.After(activity) {
		activity = *instance.LastHealthcheckAt
	}
	return activity
}

func instanceStatusAt(instance domain.SenderInstance, now time.Time, inactiveAfter time.Duration) domain.SenderStatus {
	activity := instanceActivity(instance)
	if activity.IsZero() || now.Sub(activity) > inactiveAfter {
		return domain.StatusInactive
	}
	return domain.StatusOnline
}

func instanceExpiresAt(instance domain.SenderInstance, inactiveAfter, deleteAfter time.Duration) time.Time {
	return instanceActivity(instance).Add(inactiveAfter).Add(deleteAfter)
}

func (s *Service) Instances(ctx context.Context, senderID string) ([]domain.SenderInstance, error) {
	items, err := s.repo.ListInstances(ctx, senderID)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	inactiveAfter := time.Duration(s.settings.Get().InactiveAfterSeconds) * time.Second
	for index := range items {
		items[index].Status = instanceStatusAt(items[index], now, inactiveAfter)
	}
	return items, nil
}

func (s *Service) DeleteInstance(ctx context.Context, senderID, instanceID string) error {
	if instanceID != "legacy" && !instancePattern.MatchString(instanceID) {
		return domain.ErrNotFound
	}
	return s.repo.DeleteInstance(ctx, senderID, instanceID)
}

func (s *Service) validateInstance(ctx context.Context, senderID, instanceID string) error {
	if instanceID == "" {
		return nil
	}
	if !instancePattern.MatchString(instanceID) {
		return domain.ErrConflict
	}
	exists, err := s.repo.InstanceExists(ctx, senderID, instanceID)
	if err != nil {
		return err
	}
	if !exists {
		return domain.ErrConflict
	}
	return nil
}

func (s *Service) authenticateInstance(ctx context.Context, senderID, instanceID, token string) error {
	if !instancePattern.MatchString(instanceID) {
		return domain.ErrInvalidInstanceToken
	}
	instance, err := s.repo.GetInstance(ctx, senderID, instanceID)
	if err != nil || !instanceTokenMatches(instance.TokenHash, token) {
		return domain.ErrInvalidInstanceToken
	}
	return nil
}

func (s *Service) ReceiveLog(ctx context.Context, id, senderKey, severity, message string, timestamp *time.Time, metadata map[string]any) (domain.LogEntry, time.Time, error) {
	return s.ReceiveLogWithEvent(ctx, id, senderKey, severity, message, "", "", timestamp, metadata)
}

func (s *Service) ReceiveLogWithEvent(ctx context.Context, id, senderKey, severity, message, event, occurrenceID string, timestamp *time.Time, metadata map[string]any) (domain.LogEntry, time.Time, error) {
	return s.ReceiveLogWithInstanceAndEvent(ctx, id, senderKey, "", severity, message, event, occurrenceID, timestamp, metadata)
}

func (s *Service) ReceiveLogWithInstanceAndEvent(ctx context.Context, id, senderKey, instanceID, severity, message, event, occurrenceID string, timestamp *time.Time, metadata map[string]any) (domain.LogEntry, time.Time, error) {
	return s.ReceiveLogWithInstanceAuthAndEvent(ctx, id, senderKey, instanceID, "", severity, message, event, occurrenceID, timestamp, metadata)
}

func (s *Service) ReceiveLogWithInstanceAuthAndEvent(ctx context.Context, id, senderKey, instanceID, instanceToken, severity, message, event, occurrenceID string, timestamp *time.Time, metadata map[string]any) (domain.LogEntry, time.Time, error) {
	return s.ReceiveLogWithAuthenticatedInstanceAndEvent(ctx, id, senderKey, instanceID, instanceToken, instanceID, severity, message, event, occurrenceID, timestamp, metadata)
}

func (s *Service) ReceiveLogWithAuthenticatedInstanceAndEvent(ctx context.Context, id, senderKey, authenticatedInstanceID, instanceToken, originInstanceID, severity, message, event, occurrenceID string, timestamp *time.Time, metadata map[string]any) (domain.LogEntry, time.Time, error) {
	ctx, release, err := s.repo.LockSender(ctx, id)
	if err != nil {
		return domain.LogEntry{}, time.Time{}, err
	}
	defer release()
	if strings.TrimSpace(message) == "" || int64(len(message)) > s.cfg.MaxMessageSize {
		return domain.LogEntry{}, time.Time{}, fmt.Errorf("invalid message")
	}
	meta, err := json.Marshal(metadata)
	if err != nil || int64(len(meta)) > s.cfg.MaxMetadataSize {
		return domain.LogEntry{}, time.Time{}, fmt.Errorf("invalid metadata")
	}
	sev, err := domain.ParseSeverity(severity)
	if err != nil {
		return domain.LogEntry{}, time.Time{}, err
	}
	sender, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.LogEntry{}, time.Time{}, domain.ErrInvalidSenderKey
	}
	if instanceToken != "" {
		if sender.Status == domain.StatusRevoked || sender.Status == domain.StatusExpired {
			return domain.LogEntry{}, time.Time{}, domain.ErrInvalidInstanceToken
		}
		if err = s.authenticateInstance(ctx, id, authenticatedInstanceID, instanceToken); err != nil {
			return domain.LogEntry{}, time.Time{}, err
		}
		if err = s.validateInstance(ctx, id, originInstanceID); err != nil {
			return domain.LogEntry{}, time.Time{}, err
		}
	} else {
		if err = authenticateSender(sender, senderKey); err != nil {
			return domain.LogEntry{}, time.Time{}, err
		}
		if err = s.validateInstance(ctx, id, originInstanceID); err != nil {
			return domain.LogEntry{}, time.Time{}, err
		}
	}
	if event != "" {
		if !eventKeyPattern.MatchString(event) {
			return domain.LogEntry{}, time.Time{}, domain.ErrInvalidEventKey
		}
	}
	if len([]rune(occurrenceID)) > 200 || strings.ContainsAny(occurrenceID, "\r\n\x00") {
		return domain.LogEntry{}, time.Time{}, domain.ErrInvalidEventOccurrenceID
	}
	fingerprint, err := eventOccurrenceFingerprint(sev, message, event, originInstanceID, timestamp, metadata)
	if err != nil {
		return domain.LogEntry{}, time.Time{}, err
	}
	if occurrenceID != "" {
		existing, occurrenceErr := s.repo.GetEventOccurrence(ctx, id, occurrenceID)
		switch {
		case occurrenceErr == nil && existing.Fingerprint == fingerprint:
			return existing.Entry, existing.ReceivedAt, nil
		case occurrenceErr == nil:
			return domain.LogEntry{}, time.Time{}, domain.ErrEventOccurrenceConflict
		case !errors.Is(occurrenceErr, os.ErrNotExist):
			return domain.LogEntry{}, time.Time{}, occurrenceErr
		}
	}
	previousStatus := sender.Status
	now := s.clock.Now()
	ts := now
	if timestamp != nil {
		ts = *timestamp
	}
	activityAt := now
	if originInstanceID != authenticatedInstanceID && timestamp != nil {
		activityAt = ts
	}
	e := domain.LogEntry{Timestamp: ts, ActivityAt: activityAt, SenderID: id, InstanceID: originInstanceID, Severity: sev, Message: message, Event: event, EventOccurrenceID: occurrenceID, Metadata: metadata}
	count, size, err := s.repo.Append(ctx, id, e, s.settings.Get().LogLimit)
	if err != nil {
		return e, time.Time{}, err
	}
	sender.LogLineCount = count
	sender.LogFileSize = size
	sender.UpdatedAt = now
	if s.cfg.LogCountsAsActivity || sender.Status == domain.StatusNeverConnected {
		sender.LastActivityAt = &now
		sender.Status = domain.StatusOnline
		sender.InactiveAt = nil
		sender.ExpiresAt = nil
	}
	if err = s.repo.Update(ctx, sender); err != nil {
		return e, time.Time{}, err
	}
	if occurrenceID != "" {
		err = s.repo.SaveEventOccurrence(ctx, id, repositories.EventOccurrenceRecord{ID: occurrenceID, Fingerprint: fingerprint, Entry: e, ReceivedAt: now})
		if err != nil {
			return e, time.Time{}, err
		}
	}
	s.Hub.Publish(id, e)
	if s.alertSink != nil {
		s.alertSink.Notify(ctx, sender, e)
	}
	if s.eventSink != nil && e.Event != "" {
		s.eventSink.NotifyEvent(ctx, sender, e)
	}
	if s.monitoringSink != nil {
		s.monitoringSink.Notify(ctx, sender, e)
		if previousStatus != sender.Status {
			s.monitoringSink.NotifySenderStatus(ctx, sender, previousStatus)
		}
	}
	return e, now, nil
}

func eventOccurrenceFingerprint(severity domain.LogSeverity, message, event, originInstanceID string, timestamp *time.Time, metadata map[string]any) (string, error) {
	payload := struct {
		Severity         domain.LogSeverity `json:"severity"`
		Message          string             `json:"message"`
		Event            string             `json:"event,omitempty"`
		OriginInstanceID string             `json:"origin_instance_id,omitempty"`
		Timestamp        *time.Time         `json:"timestamp,omitempty"`
		Metadata         map[string]any     `json:"metadata,omitempty"`
	}{
		Severity: severity, Message: message, Event: event,
		OriginInstanceID: originInstanceID, Timestamp: timestamp, Metadata: metadata,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}
func (s *Service) Health(ctx context.Context, id, senderKey string) (domain.Sender, time.Time, error) {
	return s.HealthWithInstance(ctx, id, senderKey, "")
}

func (s *Service) HealthWithInstance(ctx context.Context, id, senderKey, instanceID string) (domain.Sender, time.Time, error) {
	return s.HealthWithInstanceAuth(ctx, id, senderKey, instanceID, "")
}

func (s *Service) HealthWithInstanceAuth(ctx context.Context, id, senderKey, instanceID, instanceToken string) (domain.Sender, time.Time, error) {
	ctx, release, err := s.repo.LockSender(ctx, id)
	if err != nil {
		return domain.Sender{}, time.Time{}, err
	}
	defer release()
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return item, time.Time{}, domain.ErrInvalidSenderKey
	}
	if instanceToken != "" {
		if item.Status == domain.StatusRevoked || item.Status == domain.StatusExpired {
			return item, time.Time{}, domain.ErrInvalidInstanceToken
		}
		if err = s.authenticateInstance(ctx, id, instanceID, instanceToken); err != nil {
			return item, time.Time{}, err
		}
	} else {
		if err = authenticateSender(item, senderKey); err != nil {
			return item, time.Time{}, err
		}
		if err = s.validateInstance(ctx, id, instanceID); err != nil {
			return item, time.Time{}, err
		}
	}
	now := s.clock.Now()
	if err = s.repo.TouchInstance(ctx, id, instanceID, now, true); err != nil {
		return item, time.Time{}, err
	}
	previousStatus := item.Status
	item.Status = domain.StatusOnline
	item.LastActivityAt = &now
	item.LastHealthcheckAt = &now
	item.UpdatedAt = now
	item.InactiveAt = nil
	item.ExpiresAt = nil
	err = s.repo.Update(ctx, item)
	if err == nil && s.monitoringSink != nil && previousStatus != item.Status {
		s.monitoringSink.NotifySenderStatus(ctx, item, previousStatus)
	}
	return item, now, err
}
func (s *Service) Logs(ctx context.Context, id string, f domain.LogFilters) (domain.LogPage, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return domain.LogPage{}, err
	}
	return s.repo.ListLogs(ctx, id, f)
}
func (s *Service) Senders(ctx context.Context, f domain.SenderFilters) (domain.SenderPage, error) {
	items, err := s.repo.All(ctx)
	if err != nil {
		return domain.SenderPage{}, err
	}
	now := s.clock.Now()
	inactiveAfter := time.Duration(s.settings.Get().InactiveAfterSeconds) * time.Second
	for index := range items {
		instances, countErr := s.repo.RegisteredInstances(ctx, items[index].ID)
		if countErr != nil {
			return domain.SenderPage{}, countErr
		}
		active := 0
		for _, instance := range instances {
			if instanceStatusAt(instance, now, inactiveAfter) == domain.StatusOnline {
				active++
			}
		}
		items[index].InstanceCount = active
	}
	filtered := items[:0]
	for _, v := range items {
		if f.Status != "" && v.Status != f.Status {
			continue
		}
		if f.Name != "" && !strings.Contains(strings.ToLower(v.Name), strings.ToLower(f.Name)) {
			continue
		}
		if f.Search != "" && !strings.Contains(strings.ToLower(v.Name), strings.ToLower(f.Search)) {
			continue
		}
		if f.HasErrors && v.RecentErrorCount == 0 {
			continue
		}
		filtered = append(filtered, v)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		less := senderActivity(filtered[i]).Before(senderActivity(filtered[j]))
		if f.Sort == "name" {
			less = filtered[i].Name < filtered[j].Name
		}
		if f.Sort == "created_at" {
			less = filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
		}
		if f.Order == "desc" {
			return !less
		}
		return less
	})
	if f.GroupByName {
		groups := make(map[string][]domain.Sender)
		groupOrder := make([]string, 0)
		for _, sender := range filtered {
			key := strings.ToLower(strings.TrimSpace(sender.Name))
			if _, exists := groups[key]; !exists {
				groupOrder = append(groupOrder, key)
			}
			groups[key] = append(groups[key], sender)
		}
		total := len(groupOrder)
		start := (f.Page - 1) * f.PageSize
		if start > total {
			start = total
		}
		end := start + f.PageSize
		if end > total {
			end = total
		}
		pageItems := make([]domain.Sender, 0)
		for _, key := range groupOrder[start:end] {
			pageItems = append(pageItems, groups[key]...)
		}
		return domain.SenderPage{
			Items: pageItems,
			Pagination: domain.Pagination{
				Page:       f.Page,
				PageSize:   f.PageSize,
				Returned:   end - start,
				Total:      int64(total),
				TotalPages: int(math.Ceil(float64(total) / float64(f.PageSize))),
			},
		}, nil
	}
	total := len(filtered)
	start := (f.Page - 1) * f.PageSize
	if start > total {
		start = total
	}
	end := start + f.PageSize
	if end > total {
		end = total
	}
	return domain.SenderPage{Items: filtered[start:end], Pagination: domain.Pagination{Page: f.Page, PageSize: f.PageSize, Total: int64(total), TotalPages: int(math.Ceil(float64(total) / float64(f.PageSize)))}}, nil
}
func (s *Service) Summary(ctx context.Context) (map[string]any, error) {
	items, err := s.repo.All(ctx)
	if err != nil {
		return nil, err
	}
	counts := map[string]int64{"total": int64(len(items)), "never_connected": 0, "online": 0, "inactive": 0, "expired": 0, "revoked": 0}
	instanceCounts := map[string]int64{"active": 0, "inactive": 0}
	now := s.clock.Now()
	dayAgo := now.Add(-24 * time.Hour)
	inactiveAfter := time.Duration(s.settings.Get().InactiveAfterSeconds) * time.Second
	var logs, recentLogs, recentErrors, recentFatals int64
	for _, v := range items {
		counts[string(v.Status)]++
		logs += v.LogLineCount
		lastDay, errorsLastDay, fatalsLastDay, countErr := s.repo.RecentLogCounts(ctx, v.ID, dayAgo)
		if countErr != nil {
			return nil, countErr
		}
		recentLogs += lastDay
		recentErrors += errorsLastDay
		recentFatals += fatalsLastDay
		instances, instancesErr := s.repo.RegisteredInstances(ctx, v.ID)
		if instancesErr != nil {
			return nil, instancesErr
		}
		for _, instance := range instances {
			if instanceStatusAt(instance, now, inactiveAfter) == domain.StatusOnline {
				instanceCounts["active"]++
			} else {
				instanceCounts["inactive"]++
			}
		}
	}
	return map[string]any{"senders": counts, "instances": instanceCounts, "logs": map[string]int64{"total": logs, "last_24_hours": recentLogs, "errors_last_24_hours": recentErrors, "fatal_last_24_hours": recentFatals}}, nil
}
func (s *Service) Tick(ctx context.Context) error {
	items, err := s.repo.All(ctx)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	currentSettings := s.settings.Get()
	inactiveAfter := time.Duration(currentSettings.InactiveAfterSeconds) * time.Second
	deleteAfter := time.Duration(currentSettings.DeleteInactiveDays) * 24 * time.Hour
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err = s.tickSender(ctx, item, now, inactiveAfter, deleteAfter, currentSettings.InactivePreservation); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) tickSender(ctx context.Context, item domain.Sender, now time.Time, inactiveAfter, deleteAfter time.Duration, preservation domain.NumberUnitValue) error {
	lockedCtx, release, err := s.repo.LockSender(ctx, item.ID)
	if err != nil {
		return err
	}
	defer release()
	if item.Status == domain.StatusExpired {
		return s.repo.Delete(lockedCtx, item.ID)
	}
	instances, err := s.repo.RegisteredInstances(lockedCtx, item.ID)
	if err != nil {
		return err
	}
	remainingInstances := 0
	for _, instance := range instances {
		if now.Before(instanceExpiresAt(instance, inactiveAfter, deleteAfter)) {
			remainingInstances++
			continue
		}
		if err = s.repo.DeleteInstance(lockedCtx, item.ID, instance.ID); err != nil {
			return err
		}
	}
	if len(instances) > 0 && remainingInstances == 0 {
		return s.repo.Delete(lockedCtx, item.ID)
	}
	if len(instances) == 0 && item.Status == domain.StatusInactive && !now.Before(senderActivity(item).Add(inactiveAfter+deleteAfter)) {
		return s.repo.Delete(lockedCtx, item.ID)
	}
	item, err = s.repo.Get(lockedCtx, item.ID)
	if err != nil {
		return err
	}
	if item.Status == domain.StatusRevoked || item.Status == domain.StatusNeverConnected {
		return nil
	}
	if item.Status == domain.StatusInactive {
		if item.ExpiresAt == nil && item.ExpiredAt == nil {
			return nil
		}
		item.ExpiresAt, item.ExpiredAt, item.UpdatedAt = nil, nil, now
		return s.repo.Update(lockedCtx, item)
	}
	if item.Status != domain.StatusOnline || item.LastActivityAt == nil || now.Sub(*item.LastActivityAt) <= inactiveAfter {
		return nil
	}
	inactive := now
	count, size, compactErr := s.repo.CompactByLimit(lockedCtx, item.ID, preservation)
	if compactErr != nil && !errors.Is(compactErr, domain.ErrLogFileNotFound) {
		return compactErr
	}
	item.Status, item.InactiveAt, item.CompactedAt = domain.StatusInactive, &inactive, &inactive
	item.ExpiresAt, item.ExpiredAt, item.UpdatedAt = nil, nil, now
	item.LogLineCount, item.LogFileSize = count, size
	if err = s.repo.Update(lockedCtx, item); err != nil {
		return err
	}
	if s.monitoringSink != nil {
		s.monitoringSink.NotifySenderStatus(lockedCtx, item, domain.StatusOnline)
	}
	return nil
}
func (s *Service) Restore(ctx context.Context) error {
	items, err := s.repo.All(ctx)
	if err != nil {
		return err
	}
	for _, v := range items {
		if _, err = s.repo.Repair(ctx, v); err != nil {
			return err
		}
	}
	return s.Tick(ctx)
}
func (s *Service) Uptime() time.Duration { return s.clock.Now().Sub(s.started) }

func (s *Service) Settings() domain.Settings { return s.settings.Get() }

func (s *Service) UpdateSettings(value domain.Settings) (domain.Settings, error) {
	return s.settings.Update(value, s.clock.Now())
}

func (s *Service) DeleteSender(ctx context.Context, id string) error {
	ctx, release, err := s.repo.LockSender(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	return s.repo.Delete(ctx, id)
}

func senderActivity(item domain.Sender) time.Time {
	if item.LastActivityAt != nil {
		return *item.LastActivityAt
	}
	return item.CreatedAt
}
