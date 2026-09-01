package emailconfig

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"logtheater/internal/config"
	"logtheater/internal/domain"
	"logtheater/internal/validation"
)

type OutlookStored struct {
	TenantID              string `json:"tenant_id"`
	ClientID              string `json:"client_id"`
	ClientSecretEncrypted string `json:"client_secret_encrypted,omitempty"`
	SenderEmail           string `json:"sender_email"`
	SenderName            string `json:"sender_name"`
}

type GmailStored struct {
	Host              string `json:"host"`
	Port              int    `json:"port"`
	Username          string `json:"username"`
	PasswordEncrypted string `json:"password_encrypted,omitempty"`
	From              string `json:"from"`
	SenderName        string `json:"sender_name"`
}

type Stored struct {
	Provider       domain.EmailProviderType `json:"provider"`
	Enabled        bool                     `json:"enabled"`
	Outlook        OutlookStored            `json:"outlook"`
	Gmail          GmailStored              `json:"gmail"`
	UpdatedAt      time.Time                `json:"updated_at"`
	LastTestAt     *time.Time               `json:"last_test_at"`
	LastTestStatus string                   `json:"last_test_status,omitempty"`
	LastTestError  string                   `json:"last_test_error,omitempty"`
}

type OutlookInput struct {
	TenantID     string `json:"tenant_id"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	SenderEmail  string `json:"sender_email"`
	SenderName   string `json:"sender_name"`
}

type GmailInput struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password,omitempty"`
	From       string `json:"from"`
	SenderName string `json:"sender_name"`
}

type Input struct {
	Provider domain.EmailProviderType `json:"provider"`
	Enabled  bool                     `json:"enabled"`
	Outlook  OutlookInput             `json:"outlook"`
	Gmail    GmailInput               `json:"gmail"`
}

type OutlookSafe struct {
	TenantID               string `json:"tenant_id"`
	ClientID               string `json:"client_id"`
	ClientSecretConfigured bool   `json:"client_secret_configured"`
	SenderEmail            string `json:"sender_email"`
	SenderName             string `json:"sender_name"`
	ManagedByEnvironment   bool   `json:"managed_by_environment"`
}

type GmailSafe struct {
	Host                 string `json:"host"`
	Port                 int    `json:"port"`
	Username             string `json:"username"`
	PasswordConfigured   bool   `json:"password_configured"`
	From                 string `json:"from"`
	SenderName           string `json:"sender_name"`
	ManagedByEnvironment bool   `json:"managed_by_environment"`
}

type ProviderStatus struct {
	ID        domain.EmailProviderType `json:"id"`
	Enabled   bool                     `json:"enabled"`
	Available bool                     `json:"available"`
}

type Safe struct {
	Provider       domain.EmailProviderType `json:"provider"`
	Enabled        bool                     `json:"enabled"`
	Configured     bool                     `json:"configured"`
	Outlook        OutlookSafe              `json:"outlook"`
	Gmail          GmailSafe                `json:"gmail"`
	Providers      []ProviderStatus         `json:"providers"`
	UpdatedAt      time.Time                `json:"updated_at"`
	LastTestAt     *time.Time               `json:"last_test_at"`
	LastTestStatus string                   `json:"last_test_status,omitempty"`
	LastTestError  string                   `json:"last_test_error,omitempty"`
}

type Runtime struct {
	Provider     domain.EmailProviderType
	Enabled      bool
	TenantID     string
	ClientID     string
	ClientSecret string
	SenderEmail  string
	SenderName   string
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
}

type ValidationError struct {
	Code, Field, Message string
}

func (e *ValidationError) Error() string { return e.Message }

type Store struct {
	mu     sync.RWMutex
	path   string
	cfg    config.Config
	key    []byte
	stored Stored
}

func Open(dataDir string, cfg config.Config, now time.Time) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, err
	}
	store := &Store{path: filepath.Join(dataDir, "email-settings.json"), cfg: cfg}
	if cfg.EmailSettingsEncryptionKey != "" {
		key, err := base64.StdEncoding.DecodeString(cfg.EmailSettingsEncryptionKey)
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("EMAIL_SETTINGS_ENCRYPTION_KEY must be a base64 encoded 32-byte key")
		}
		store.key = key
	}
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		provider := domain.EmailProviderOutlook
		if cfg.EmailProvider == "gmail" {
			provider = domain.EmailProviderGmail
		}
		store.stored = Stored{
			Provider:  provider,
			Outlook:   OutlookStored{SenderName: "LogHill"},
			Gmail:     GmailStored{Host: "smtp.gmail.com", Port: 587, SenderName: "LogHill"},
			UpdatedAt: now,
		}
		if err = store.writeAtomic(store.stored); err != nil {
			return nil, err
		}
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &store.stored); err != nil {
		return nil, fmt.Errorf("decode email settings: %w", err)
	}
	if store.stored.Provider != domain.EmailProviderOutlook && store.stored.Provider != domain.EmailProviderGmail {
		return nil, fmt.Errorf("stored email provider is not available")
	}
	if store.stored.Outlook.ClientSecretEncrypted != "" {
		if _, err = store.decrypt(store.stored.Outlook.ClientSecretEncrypted); err != nil {
			return nil, fmt.Errorf("decrypt stored email credential: %w", err)
		}
	}
	if store.stored.Gmail.PasswordEncrypted != "" {
		if _, err = store.decrypt(store.stored.Gmail.PasswordEncrypted); err != nil {
			return nil, fmt.Errorf("decrypt stored SMTP credential: %w", err)
		}
	}
	if store.stored.Gmail.Host == "" {
		store.stored.Gmail.Host = "smtp.gmail.com"
	}
	if store.stored.Gmail.Port == 0 {
		store.stored.Gmail.Port = 587
	}
	if store.stored.Gmail.SenderName == "" {
		store.stored.Gmail.SenderName = "LogHill"
	}
	return store, nil
}

func (s *Store) Runtime() (Runtime, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runtimeUnlocked()
}

func (s *Store) runtimeUnlocked() (Runtime, error) {
	if s.stored.Provider == domain.EmailProviderGmail || s.cfg.EmailProvider == "gmail" && s.cfg.SMTPManagedByEnvironment {
		password := s.cfg.SMTPPassword
		if password == "" && s.stored.Gmail.PasswordEncrypted != "" {
			var err error
			password, err = s.decrypt(s.stored.Gmail.PasswordEncrypted)
			if err != nil {
				return Runtime{}, err
			}
		}
		value := Runtime{Provider: domain.EmailProviderGmail, Enabled: s.stored.Enabled, SMTPHost: s.stored.Gmail.Host, SMTPPort: s.stored.Gmail.Port, SMTPUsername: s.stored.Gmail.Username, SMTPPassword: password, SenderEmail: s.stored.Gmail.From, SenderName: s.stored.Gmail.SenderName}
		if s.cfg.SMTPManagedByEnvironment {
			value.Enabled = s.cfg.SMTPEnabled
			if s.cfg.SMTPHost != "" {
				value.SMTPHost = s.cfg.SMTPHost
			}
			if s.cfg.SMTPPort > 0 {
				value.SMTPPort = s.cfg.SMTPPort
			}
			if s.cfg.SMTPUsername != "" {
				value.SMTPUsername = s.cfg.SMTPUsername
			}
			if s.cfg.SMTPPassword != "" {
				value.SMTPPassword = s.cfg.SMTPPassword
			}
			if s.cfg.SMTPFrom != "" {
				value.SenderEmail = s.cfg.SMTPFrom
			}
			if s.cfg.SMTPSenderName != "" {
				value.SenderName = s.cfg.SMTPSenderName
			}
		}
		return value, nil
	}
	secret := s.cfg.OutlookClientSecret
	if secret == "" && s.stored.Outlook.ClientSecretEncrypted != "" {
		var err error
		secret, err = s.decrypt(s.stored.Outlook.ClientSecretEncrypted)
		if err != nil {
			return Runtime{}, err
		}
	}
	value := Runtime{
		Provider:     domain.EmailProviderOutlook,
		Enabled:      s.stored.Enabled,
		TenantID:     s.stored.Outlook.TenantID,
		ClientID:     s.stored.Outlook.ClientID,
		ClientSecret: secret,
		SenderEmail:  s.stored.Outlook.SenderEmail,
		SenderName:   s.stored.Outlook.SenderName,
	}
	if s.cfg.EmailManagedByEnvironment {
		if s.cfg.OutlookEnabledManaged || (s.cfg.OutlookEnabled && s.cfg.OutlookTenantID != "" && s.cfg.OutlookClientID != "" && s.cfg.OutlookClientSecret != "" && s.cfg.OutlookSenderEmail != "") {
			value.Enabled = s.cfg.OutlookEnabled
		}
		if s.cfg.OutlookTenantID != "" {
			value.TenantID = s.cfg.OutlookTenantID
		}
		if s.cfg.OutlookClientID != "" {
			value.ClientID = s.cfg.OutlookClientID
		}
		if s.cfg.OutlookClientSecret != "" {
			value.ClientSecret = s.cfg.OutlookClientSecret
		}
		if s.cfg.OutlookSenderEmail != "" {
			value.SenderEmail = s.cfg.OutlookSenderEmail
		}
		if s.cfg.OutlookSenderName != "" {
			value.SenderName = s.cfg.OutlookSenderName
		}
	}
	return value, nil
}

func (s *Store) Safe() (Safe, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.safeUnlocked()
}

func (s *Store) IsReady() bool {
	runtime, err := s.Runtime()
	return err == nil && runtime.Enabled && runtimeConfigured(runtime)
}

func (s *Store) Update(input Input, now time.Time) (Safe, error) {
	if input.Provider == domain.EmailProviderGmail {
		return s.updateGmail(input, now)
	}
	if input.Provider != domain.EmailProviderOutlook {
		return Safe{}, &ValidationError{Code: "INVALID_EMAIL_SETTINGS", Field: "provider", Message: "Invalid email provider."}
	}
	if s.cfg.EmailManagedByEnvironment {
		return Safe{}, &ValidationError{Code: "EMAIL_SETTINGS_MANAGED", Field: "outlook", Message: "The Outlook configuration is managed through environment variables."}
	}
	input.Outlook.TenantID = strings.TrimSpace(input.Outlook.TenantID)
	input.Outlook.ClientID = strings.TrimSpace(input.Outlook.ClientID)
	input.Outlook.SenderName = strings.TrimSpace(input.Outlook.SenderName)
	if strings.ContainsAny(input.Outlook.SenderName, "\r\n") {
		return Safe{}, &ValidationError{Code: "INVALID_EMAIL_SETTINGS", Field: "outlook.sender_name", Message: "The sender name contains invalid characters."}
	}
	sender, valid := validation.EmailAddress(input.Outlook.SenderEmail)
	if input.Outlook.SenderEmail != "" && !valid {
		return Safe{}, &ValidationError{Code: "INVALID_EMAIL_SETTINGS", Field: "outlook.sender_email", Message: "Enter a valid sender email address."}
	}
	if len(input.Outlook.SenderName) > 100 {
		return Safe{}, &ValidationError{Code: "INVALID_EMAIL_SETTINGS", Field: "outlook.sender_name", Message: "The sender name must be at most 100 characters."}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.stored
	next.Provider = input.Provider
	next.Enabled = input.Enabled
	next.Outlook.TenantID = input.Outlook.TenantID
	next.Outlook.ClientID = input.Outlook.ClientID
	next.Outlook.SenderEmail = sender
	next.Outlook.SenderName = input.Outlook.SenderName
	if input.Outlook.ClientSecret != "" {
		if len(s.key) == 0 {
			return Safe{}, &ValidationError{Code: "EMAIL_ENCRYPTION_KEY_REQUIRED", Field: "outlook.client_secret", Message: "Configure EMAIL_SETTINGS_ENCRYPTION_KEY to save the credential through the interface."}
		}
		encrypted, err := s.encrypt(input.Outlook.ClientSecret)
		if err != nil {
			return Safe{}, err
		}
		next.Outlook.ClientSecretEncrypted = encrypted
	}
	secretAvailable := next.Outlook.ClientSecretEncrypted != "" || s.cfg.OutlookClientSecret != ""
	if input.Enabled && (next.Outlook.TenantID == "" || next.Outlook.ClientID == "" || !secretAvailable || next.Outlook.SenderEmail == "") {
		return Safe{}, &ValidationError{Code: "OUTLOOK_NOT_CONFIGURED", Field: "outlook", Message: "Complete the credentials and sender before enabling Outlook."}
	}
	next.UpdatedAt = now
	if err := s.writeAtomic(next); err != nil {
		return Safe{}, err
	}
	s.stored = next
	return s.safeUnlocked()
}

func (s *Store) updateGmail(input Input, now time.Time) (Safe, error) {
	if s.cfg.SMTPManagedByEnvironment {
		return Safe{}, &ValidationError{Code: "EMAIL_SETTINGS_MANAGED", Field: "gmail", Message: "The SMTP configuration is managed through environment variables."}
	}
	input.Gmail.Host = strings.TrimSpace(input.Gmail.Host)
	input.Gmail.Username = strings.TrimSpace(input.Gmail.Username)
	input.Gmail.SenderName = strings.TrimSpace(input.Gmail.SenderName)
	if input.Gmail.Host == "" {
		input.Gmail.Host = "smtp.gmail.com"
	}
	if input.Gmail.Port == 0 {
		input.Gmail.Port = 587
	}
	if input.Gmail.Port < 1 || input.Gmail.Port > 65535 {
		return Safe{}, &ValidationError{Code: "INVALID_EMAIL_SETTINGS", Field: "gmail.port", Message: "Enter a valid SMTP port."}
	}
	if strings.ContainsAny(input.Gmail.Host+input.Gmail.Username+input.Gmail.SenderName, "\r\n") || len(input.Gmail.SenderName) > 100 {
		return Safe{}, &ValidationError{Code: "INVALID_EMAIL_SETTINGS", Field: "gmail", Message: "The SMTP configuration contains invalid characters."}
	}
	from, valid := validation.EmailAddress(input.Gmail.From)
	if input.Gmail.From != "" && !valid {
		return Safe{}, &ValidationError{Code: "INVALID_EMAIL_SETTINGS", Field: "gmail.from", Message: "Enter a valid sender email address."}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.stored
	next.Provider = domain.EmailProviderGmail
	next.Enabled = input.Enabled
	next.Gmail = GmailStored{Host: input.Gmail.Host, Port: input.Gmail.Port, Username: input.Gmail.Username, PasswordEncrypted: next.Gmail.PasswordEncrypted, From: from, SenderName: input.Gmail.SenderName}
	if input.Gmail.Password != "" {
		if len(s.key) == 0 {
			return Safe{}, &ValidationError{Code: "EMAIL_ENCRYPTION_KEY_REQUIRED", Field: "gmail.password", Message: "Configure EMAIL_SETTINGS_ENCRYPTION_KEY to save the password through the interface."}
		}
		encrypted, err := s.encrypt(input.Gmail.Password)
		if err != nil {
			return Safe{}, err
		}
		next.Gmail.PasswordEncrypted = encrypted
	}
	passwordAvailable := next.Gmail.PasswordEncrypted != "" || s.cfg.SMTPPassword != ""
	if input.Enabled && (next.Gmail.Username == "" || !passwordAvailable || next.Gmail.From == "") {
		return Safe{}, &ValidationError{Code: "GMAIL_NOT_CONFIGURED", Field: "gmail", Message: "Complete the server, username, app password, and sender before enabling Gmail."}
	}
	next.UpdatedAt = now
	if err := s.writeAtomic(next); err != nil {
		return Safe{}, err
	}
	s.stored = next
	return s.safeUnlocked()
}

func (s *Store) RecordTest(success bool, message string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.stored
	next.LastTestAt = &now
	if success {
		next.LastTestStatus = "success"
		next.LastTestError = ""
	} else {
		next.LastTestStatus = "failed"
		next.LastTestError = message
	}
	if err := s.writeAtomic(next); err != nil {
		return err
	}
	s.stored = next
	return nil
}

func (s *Store) safeUnlocked() (Safe, error) {
	runtime, err := s.runtimeUnlocked()
	if err != nil {
		return Safe{}, err
	}
	configured := runtimeConfigured(runtime)
	outlookSecret := s.stored.Outlook.ClientSecretEncrypted != "" || s.cfg.OutlookClientSecret != ""
	gmailPassword := s.stored.Gmail.PasswordEncrypted != "" || s.cfg.SMTPPassword != ""
	outlook := OutlookSafe{TenantID: s.stored.Outlook.TenantID, ClientID: s.stored.Outlook.ClientID, ClientSecretConfigured: outlookSecret, SenderEmail: s.stored.Outlook.SenderEmail, SenderName: s.stored.Outlook.SenderName, ManagedByEnvironment: s.cfg.EmailManagedByEnvironment}
	if s.cfg.EmailManagedByEnvironment {
		if s.cfg.OutlookTenantID != "" {
			outlook.TenantID = s.cfg.OutlookTenantID
		}
		if s.cfg.OutlookClientID != "" {
			outlook.ClientID = s.cfg.OutlookClientID
		}
		if s.cfg.OutlookSenderEmail != "" {
			outlook.SenderEmail = s.cfg.OutlookSenderEmail
		}
		if s.cfg.OutlookSenderName != "" {
			outlook.SenderName = s.cfg.OutlookSenderName
		}
	}
	gmail := GmailSafe{Host: s.stored.Gmail.Host, Port: s.stored.Gmail.Port, Username: s.stored.Gmail.Username, PasswordConfigured: gmailPassword, From: s.stored.Gmail.From, SenderName: s.stored.Gmail.SenderName, ManagedByEnvironment: s.cfg.SMTPManagedByEnvironment}
	if s.cfg.SMTPManagedByEnvironment {
		gmail.Host, gmail.Port, gmail.Username, gmail.From, gmail.SenderName = s.cfg.SMTPHost, s.cfg.SMTPPort, s.cfg.SMTPUsername, s.cfg.SMTPFrom, s.cfg.SMTPSenderName
	}
	return Safe{
		Provider: runtime.Provider, Enabled: runtime.Enabled, Configured: configured,
		Outlook: outlook, Gmail: gmail,
		Providers: []ProviderStatus{{ID: domain.EmailProviderOutlook, Enabled: runtime.Provider == domain.EmailProviderOutlook && runtime.Enabled, Available: true}, {ID: domain.EmailProviderGmail, Enabled: runtime.Provider == domain.EmailProviderGmail && runtime.Enabled, Available: true}},
		UpdatedAt: s.stored.UpdatedAt, LastTestAt: s.stored.LastTestAt, LastTestStatus: s.stored.LastTestStatus, LastTestError: s.stored.LastTestError,
	}, nil
}

func runtimeConfigured(value Runtime) bool {
	if value.Provider == domain.EmailProviderGmail {
		return value.SMTPHost != "" && value.SMTPPort > 0 && value.SMTPUsername != "" && value.SMTPPassword != "" && value.SenderEmail != ""
	}
	return value.TenantID != "" && value.ClientID != "" && value.ClientSecret != "" && value.SenderEmail != ""
}

func (s *Store) encrypt(value string) (string, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(value), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (s *Store) decrypt(value string) (string, error) {
	if len(s.key) == 0 {
		return "", errors.New("email settings encryption key is unavailable")
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("invalid encrypted email credential")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *Store) writeAtomic(value Stored) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary := s.path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err = os.Rename(temporary, s.path); err != nil {
		_ = os.Remove(temporary)
	}
	return err
}
