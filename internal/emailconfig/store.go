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

type Stored struct {
	Provider       domain.EmailProviderType `json:"provider"`
	Enabled        bool                     `json:"enabled"`
	Outlook        OutlookStored            `json:"outlook"`
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

type Input struct {
	Provider domain.EmailProviderType `json:"provider"`
	Enabled  bool                     `json:"enabled"`
	Outlook  OutlookInput             `json:"outlook"`
}

type OutlookSafe struct {
	TenantID               string `json:"tenant_id"`
	ClientID               string `json:"client_id"`
	ClientSecretConfigured bool   `json:"client_secret_configured"`
	SenderEmail            string `json:"sender_email"`
	SenderName             string `json:"sender_name"`
	ManagedByEnvironment   bool   `json:"managed_by_environment"`
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
	Providers      []ProviderStatus         `json:"providers"`
	UpdatedAt      time.Time                `json:"updated_at"`
	LastTestAt     *time.Time               `json:"last_test_at"`
	LastTestStatus string                   `json:"last_test_status,omitempty"`
	LastTestError  string                   `json:"last_test_error,omitempty"`
}

type Runtime struct {
	Enabled      bool
	TenantID     string
	ClientID     string
	ClientSecret string
	SenderEmail  string
	SenderName   string
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
		store.stored = Stored{
			Provider:  domain.EmailProviderOutlook,
			Outlook:   OutlookStored{SenderName: "LogHill"},
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
	if store.stored.Provider != domain.EmailProviderOutlook {
		return nil, fmt.Errorf("stored email provider is not available")
	}
	if store.stored.Outlook.ClientSecretEncrypted != "" {
		if _, err = store.decrypt(store.stored.Outlook.ClientSecretEncrypted); err != nil {
			return nil, fmt.Errorf("decrypt stored email credential: %w", err)
		}
	}
	return store, nil
}

func (s *Store) Runtime() (Runtime, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runtimeUnlocked()
}

func (s *Store) runtimeUnlocked() (Runtime, error) {
	secret := s.cfg.OutlookClientSecret
	if secret == "" && s.stored.Outlook.ClientSecretEncrypted != "" {
		var err error
		secret, err = s.decrypt(s.stored.Outlook.ClientSecretEncrypted)
		if err != nil {
			return Runtime{}, err
		}
	}
	value := Runtime{
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
	runtime, err := s.runtimeUnlocked()
	if err != nil {
		return Safe{}, err
	}
	configured := runtime.TenantID != "" && runtime.ClientID != "" && runtime.ClientSecret != "" && runtime.SenderEmail != ""
	return Safe{
		Provider:   domain.EmailProviderOutlook,
		Enabled:    runtime.Enabled,
		Configured: configured,
		Outlook: OutlookSafe{
			TenantID: runtime.TenantID, ClientID: runtime.ClientID,
			ClientSecretConfigured: runtime.ClientSecret != "", SenderEmail: runtime.SenderEmail,
			SenderName: runtime.SenderName, ManagedByEnvironment: s.cfg.EmailManagedByEnvironment,
		},
		Providers: []ProviderStatus{
			{ID: domain.EmailProviderOutlook, Enabled: runtime.Enabled, Available: true},
			{ID: domain.EmailProviderGmail, Enabled: false, Available: false},
		},
		UpdatedAt: s.stored.UpdatedAt, LastTestAt: s.stored.LastTestAt,
		LastTestStatus: s.stored.LastTestStatus, LastTestError: s.stored.LastTestError,
	}, nil
}

func (s *Store) IsReady() bool {
	runtime, err := s.Runtime()
	return err == nil && runtime.Enabled && runtime.TenantID != "" && runtime.ClientID != "" && runtime.ClientSecret != "" && runtime.SenderEmail != ""
}

func (s *Store) Update(input Input, now time.Time) (Safe, error) {
	if input.Provider == domain.EmailProviderGmail {
		return Safe{}, &ValidationError{Code: "EMAIL_PROVIDER_NOT_AVAILABLE", Field: "provider", Message: "O provedor Gmail ainda não está disponível."}
	}
	if input.Provider != domain.EmailProviderOutlook {
		return Safe{}, &ValidationError{Code: "INVALID_EMAIL_SETTINGS", Field: "provider", Message: "Provedor de e-mail inválido."}
	}
	if s.cfg.EmailManagedByEnvironment {
		return Safe{}, &ValidationError{Code: "EMAIL_SETTINGS_MANAGED", Field: "outlook", Message: "A configuração do Outlook é gerenciada por variáveis de ambiente."}
	}
	input.Outlook.TenantID = strings.TrimSpace(input.Outlook.TenantID)
	input.Outlook.ClientID = strings.TrimSpace(input.Outlook.ClientID)
	input.Outlook.SenderName = strings.TrimSpace(input.Outlook.SenderName)
	if strings.ContainsAny(input.Outlook.SenderName, "\r\n") {
		return Safe{}, &ValidationError{Code: "INVALID_EMAIL_SETTINGS", Field: "outlook.sender_name", Message: "O nome do remetente contém caracteres inválidos."}
	}
	sender, valid := validation.EmailAddress(input.Outlook.SenderEmail)
	if input.Outlook.SenderEmail != "" && !valid {
		return Safe{}, &ValidationError{Code: "INVALID_EMAIL_SETTINGS", Field: "outlook.sender_email", Message: "Informe um e-mail remetente válido."}
	}
	if len(input.Outlook.SenderName) > 100 {
		return Safe{}, &ValidationError{Code: "INVALID_EMAIL_SETTINGS", Field: "outlook.sender_name", Message: "O nome do remetente deve possuir no máximo 100 caracteres."}
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
			return Safe{}, &ValidationError{Code: "EMAIL_ENCRYPTION_KEY_REQUIRED", Field: "outlook.client_secret", Message: "Configure EMAIL_SETTINGS_ENCRYPTION_KEY para salvar a credencial pela interface."}
		}
		encrypted, err := s.encrypt(input.Outlook.ClientSecret)
		if err != nil {
			return Safe{}, err
		}
		next.Outlook.ClientSecretEncrypted = encrypted
	}
	secretAvailable := next.Outlook.ClientSecretEncrypted != "" || s.cfg.OutlookClientSecret != ""
	if input.Enabled && (next.Outlook.TenantID == "" || next.Outlook.ClientID == "" || !secretAvailable || next.Outlook.SenderEmail == "") {
		return Safe{}, &ValidationError{Code: "OUTLOOK_NOT_CONFIGURED", Field: "outlook", Message: "Complete as credenciais e o remetente antes de habilitar o Outlook."}
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
	configured := runtime.TenantID != "" && runtime.ClientID != "" && runtime.ClientSecret != "" && runtime.SenderEmail != ""
	return Safe{
		Provider: domain.EmailProviderOutlook, Enabled: runtime.Enabled, Configured: configured,
		Outlook:   OutlookSafe{TenantID: runtime.TenantID, ClientID: runtime.ClientID, ClientSecretConfigured: runtime.ClientSecret != "", SenderEmail: runtime.SenderEmail, SenderName: runtime.SenderName, ManagedByEnvironment: s.cfg.EmailManagedByEnvironment},
		Providers: []ProviderStatus{{ID: domain.EmailProviderOutlook, Enabled: runtime.Enabled, Available: true}, {ID: domain.EmailProviderGmail, Available: false}},
		UpdatedAt: s.stored.UpdatedAt, LastTestAt: s.stored.LastTestAt, LastTestStatus: s.stored.LastTestStatus, LastTestError: s.stored.LastTestError,
	}, nil
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
