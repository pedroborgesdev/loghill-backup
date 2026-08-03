package service

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
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"logtheater/internal/config"
	"logtheater/internal/domain"
	"logtheater/internal/repository"
	settingsstore "logtheater/internal/settings"
	"logtheater/internal/storage"
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
	repo           *repository.FileRepository
	cfg            config.Config
	clock          domain.Clock
	settings       *settingsstore.Store
	Hub            *Hub
	started        time.Time
	locks          *storage.LockManager
	alertSink      LogAlertSink
	eventSink      LogEventSink
	monitoringSink LogMonitoringSink
}

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

func New(repo *repository.FileRepository, cfg config.Config, clock domain.Clock, settings *settingsstore.Store) *Service {
	return &Service{repo: repo, cfg: cfg, clock: clock, settings: settings, Hub: NewHub(cfg.SSEMaxClients, cfg.SSEBuffer), started: clock.Now(), locks: repo.Locks()}
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
		return "", &SenderValidationError{Field: "name", Message: "O nome deve possuir entre 3 e 80 caracteres."}
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

func senderKeyMatches(expectedHash, candidate string) bool {
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
		return domain.Sender{}, SenderCredentials{}, &SenderValidationError{Field: "name", Message: "O nome não gera um identificador válido."}
	}
	description = strings.TrimSpace(description)
	if len([]rune(description)) > 250 {
		return domain.Sender{}, SenderCredentials{}, &SenderValidationError{Field: "description", Message: "A descrição deve possuir no máximo 250 caracteres."}
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
		return domain.Sender{}, &SenderValidationError{Field: "description", Message: "A descrição deve possuir no máximo 250 caracteres."}
	}
	lock := s.locks.Get(id)
	lock.Lock()
	defer lock.Unlock()
	ctx = storage.ContextWithSenderLock(ctx, id)
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
	lock := s.locks.Get(id)
	lock.Lock()
	defer lock.Unlock()
	ctx = storage.ContextWithSenderLock(ctx, id)
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
	lock := s.locks.Get(id)
	lock.Lock()
	defer lock.Unlock()
	ctx = storage.ContextWithSenderLock(ctx, id)
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
	lock := s.locks.Get(id)
	lock.Lock()
	defer lock.Unlock()
	ctx = storage.ContextWithSenderLock(ctx, id)
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
	lock := s.locks.Get(senderID)
	lock.Lock()
	defer lock.Unlock()
	ctx = storage.ContextWithSenderLock(ctx, senderID)
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

func (s *Service) Instances(ctx context.Context, senderID string) ([]domain.SenderInstance, error) {
	items, err := s.repo.ListInstances(ctx, senderID)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	for index := range items {
		activity := items[index].CreatedAt
		if items[index].LastActivityAt != nil {
			activity = *items[index].LastActivityAt
		}
		if items[index].LastHealthcheckAt != nil && items[index].LastHealthcheckAt.After(activity) {
			activity = *items[index].LastHealthcheckAt
		}
		items[index].Status = domain.StatusOnline
		if activity.IsZero() || now.Sub(activity) > time.Duration(s.settings.Get().InactiveAfterSeconds)*time.Second {
			items[index].Status = domain.StatusInactive
		}
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

func (s *Service) ReceiveLog(ctx context.Context, id, senderKey, severity, message string, timestamp *time.Time, metadata map[string]any) (domain.LogEntry, time.Time, error) {
	return s.ReceiveLogWithEvent(ctx, id, senderKey, severity, message, "", "", timestamp, metadata)
}

func (s *Service) ReceiveLogWithEvent(ctx context.Context, id, senderKey, severity, message, event, occurrenceID string, timestamp *time.Time, metadata map[string]any) (domain.LogEntry, time.Time, error) {
	return s.ReceiveLogWithInstanceAndEvent(ctx, id, senderKey, "", severity, message, event, occurrenceID, timestamp, metadata)
}

func (s *Service) ReceiveLogWithInstanceAndEvent(ctx context.Context, id, senderKey, instanceID, severity, message, event, occurrenceID string, timestamp *time.Time, metadata map[string]any) (domain.LogEntry, time.Time, error) {
	lock := s.locks.Get(id)
	lock.Lock()
	defer lock.Unlock()
	ctx = storage.ContextWithSenderLock(ctx, id)
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
	if err = authenticateSender(sender, senderKey); err != nil {
		return domain.LogEntry{}, time.Time{}, err
	}
	if err = s.validateInstance(ctx, id, instanceID); err != nil {
		return domain.LogEntry{}, time.Time{}, err
	}
	if event != "" {
		if !eventKeyPattern.MatchString(event) {
			return domain.LogEntry{}, time.Time{}, domain.ErrInvalidEventKey
		}
	}
	if len([]rune(occurrenceID)) > 200 || strings.ContainsAny(occurrenceID, "\r\n\x00") {
		return domain.LogEntry{}, time.Time{}, domain.ErrInvalidEventOccurrenceID
	}
	previousStatus := sender.Status
	now := s.clock.Now()
	ts := now
	if timestamp != nil {
		ts = *timestamp
	}
	e := domain.LogEntry{Timestamp: ts, SenderID: id, InstanceID: instanceID, Severity: sev, Message: message, Event: event, EventOccurrenceID: occurrenceID, Metadata: metadata}
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
func (s *Service) Health(ctx context.Context, id, senderKey string) (domain.Sender, time.Time, error) {
	return s.HealthWithInstance(ctx, id, senderKey, "")
}

func (s *Service) HealthWithInstance(ctx context.Context, id, senderKey, instanceID string) (domain.Sender, time.Time, error) {
	lock := s.locks.Get(id)
	lock.Lock()
	defer lock.Unlock()
	ctx = storage.ContextWithSenderLock(ctx, id)
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return item, time.Time{}, domain.ErrInvalidSenderKey
	}
	if err = authenticateSender(item, senderKey); err != nil {
		return item, time.Time{}, err
	}
	if err = s.validateInstance(ctx, id, instanceID); err != nil {
		return item, time.Time{}, err
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
	for index := range items {
		count, countErr := s.repo.InstanceCount(ctx, items[index].ID)
		if countErr != nil {
			return domain.SenderPage{}, countErr
		}
		if count == 0 && items[index].LogLineCount > 0 {
			count = 1
		}
		items[index].InstanceCount = count
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
	var logs int64
	for _, v := range items {
		counts[string(v.Status)]++
		logs += v.LogLineCount
	}
	return map[string]any{"senders": counts, "logs": map[string]int64{"total": logs, "last_24_hours": 0, "errors_last_24_hours": 0, "fatal_last_24_hours": 0}}, nil
}
func (s *Service) Tick(ctx context.Context) error {
	items, err := s.repo.All(ctx)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	currentSettings := s.settings.Get()
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if item.Status == domain.StatusExpired || item.Status == domain.StatusRevoked || item.Status == domain.StatusNeverConnected {
			continue
		}
		lock := s.locks.Get(item.ID)
		lock.Lock()
		ctx = storage.ContextWithSenderLock(ctx, item.ID)
		if item.Status == domain.StatusInactive && item.InactiveAt != nil {
			expires := item.InactiveAt.Add(time.Duration(currentSettings.DeleteInactiveDays) * 24 * time.Hour)
			if item.ExpiresAt == nil || !item.ExpiresAt.Equal(expires) {
				item.ExpiresAt = &expires
				item.UpdatedAt = now
				if err = s.repo.Update(ctx, item); err != nil {
					lock.Unlock()
					return err
				}
			}
		}
		if item.Status == domain.StatusInactive && item.ExpiresAt != nil && !now.Before(*item.ExpiresAt) {
			if err = s.repo.DeleteLogs(ctx, item.ID); err != nil {
				lock.Unlock()
				return err
			}
			item.Status = domain.StatusExpired
			item.ExpiredAt = &now
			item.UpdatedAt = now
			item.LogLineCount = 0
			item.LogFileSize = 0
			if err = s.repo.Update(ctx, item); err != nil {
				lock.Unlock()
				return err
			}
			lock.Unlock()
			continue
		}
		if item.Status == domain.StatusOnline && item.LastActivityAt != nil && now.Sub(*item.LastActivityAt) > time.Duration(currentSettings.InactiveAfterSeconds)*time.Second {
			inactive := now
			expires := now.Add(time.Duration(currentSettings.DeleteInactiveDays) * 24 * time.Hour)
			count, size, e := s.repo.CompactByLimit(ctx, item.ID, currentSettings.InactivePreservation)
			if e != nil && !errors.Is(e, domain.ErrLogFileNotFound) {
				lock.Unlock()
				return e
			}
			item.Status = domain.StatusInactive
			item.InactiveAt = &inactive
			item.CompactedAt = &inactive
			item.ExpiresAt = &expires
			item.UpdatedAt = now
			item.LogLineCount = count
			item.LogFileSize = size
			if err = s.repo.Update(ctx, item); err != nil {
				lock.Unlock()
				return err
			}
			if s.monitoringSink != nil {
				s.monitoringSink.NotifySenderStatus(ctx, item, domain.StatusOnline)
			}
		}
		lock.Unlock()
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
	lock := s.locks.Get(id)
	lock.Lock()
	defer lock.Unlock()
	ctx = storage.ContextWithSenderLock(ctx, id)
	return s.repo.Delete(ctx, id)
}

func senderActivity(item domain.Sender) time.Time {
	if item.LastActivityAt != nil {
		return *item.LastActivityAt
	}
	return item.CreatedAt
}
