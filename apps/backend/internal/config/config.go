package config

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host, Port, DataDir                                            string
	PublicURL                                                      string
	MaxLogLines, CompactTarget, CompactKeep, MaxPageSize           int
	MaxBodySize, MaxMessageSize, MaxMetadataSize                   int64
	InactiveAfter, DeleteAfter, CleanupInterval                    time.Duration
	HealthcheckInterval, SSEHeartbeat                              time.Duration
	LogCountsAsActivity, CORS, RateLimit                           bool
	AllowedOrigins                                                 []string
	RateRequests                                                   int
	RateWindow                                                     time.Duration
	AuthEnabled                                                    bool
	AppPassword                                                    string
	SSEBuffer, SSEMaxClients                                       int
	ShutdownTimeout                                                time.Duration
	EmailProvider                                                  string
	OutlookTenantID, OutlookClientID, OutlookClientSecret          string
	OutlookSenderEmail, OutlookSenderName                          string
	OutlookEnabled, EmailManagedByEnvironment                      bool
	OutlookEnabledManaged                                          bool
	SMTPHost, SMTPUsername, SMTPPassword, SMTPFrom, SMTPSenderName string
	SMTPPort                                                       int
	SMTPEnabled, SMTPManagedByEnvironment                          bool
	EmailSettingsEncryptionKey                                     string
	EmailAlertQueueSize, EmailAlertWorkers, EmailAlertMaxRetries   int
	EmailAlertSendTimeout, EmailAlertRetryInterval                 time.Duration
	ExecutionHistoryRetentionDays, ExecutionHistoryMaxRecords      int
	ExecutionHistoryCleanupInterval                                time.Duration
}

func Load() (Config, error) {
	loadDotEnv(".env")
	emailProvider := strings.ToLower(strings.TrimSpace(env("EMAIL_PROVIDER", "outlook")))
	if emailProvider == "o365" {
		emailProvider = "outlook"
	}
	outlookSenderEmail := strings.TrimSpace(os.Getenv("OUTLOOK_SENDER_EMAIL"))
	if outlookSenderEmail == "" {
		outlookSenderEmail = firstEnvironment("EMAIL_FROM_ADDR", "EMAIL_USER")
	}
	outlookTenantID := firstEnvironment("OUTLOOK_TENANT_ID", "O365_TENANT_ID")
	outlookClientID := firstEnvironment("OUTLOOK_CLIENT_ID", "O365_CLIENT_ID")
	outlookClientSecret := firstEnvironment("OUTLOOK_CLIENT_SECRET", "O365_CLIENT_SECRET")
	outlookEnabled := envBool("OUTLOOK_ENABLED", false)
	_, outlookEnabledManaged := os.LookupEnv("OUTLOOK_ENABLED")
	if !outlookEnabledManaged && emailProvider == "outlook" {
		outlookEnabled = outlookTenantID != "" && outlookClientID != "" && outlookClientSecret != "" && outlookSenderEmail != ""
	}
	smtpHost := strings.TrimSpace(env("SMTP_HOST", "smtp.gmail.com"))
	smtpPort := envInt("SMTP_PORT", 587)
	smtpUsername := strings.TrimSpace(os.Getenv("SMTP_USERNAME"))
	smtpPassword := os.Getenv("SMTP_PASSWORD")
	smtpFrom := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	smtpEnabled := envBool("SMTP_ENABLED", false)
	if _, managed := os.LookupEnv("SMTP_ENABLED"); !managed && emailProvider == "gmail" {
		smtpEnabled = smtpHost != "" && smtpPort > 0 && smtpUsername != "" && smtpPassword != "" && smtpFrom != ""
	}
	appPassword := firstEnvironment("APP_PASSWORD", "ADMIN_API_KEY")
	authEnabled := appPassword != ""
	if raw, ok := os.LookupEnv("APP_AUTH_ENABLED"); ok {
		parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return Config{}, fmt.Errorf("APP_AUTH_ENABLED must be a boolean")
		}
		authEnabled = parsed
	} else if _, ok := os.LookupEnv("ADMIN_AUTH_ENABLED"); ok {
		authEnabled = envBool("ADMIN_AUTH_ENABLED", false)
	}
	c := Config{
		Host: env("APP_HOST", "0.0.0.0"), Port: env("APP_PORT", "8080"), DataDir: env("DATA_DIR", "./data"),
		MaxLogLines: envInt("MAX_LOG_LINES", 100000), CompactTarget: envInt("LOG_COMPACT_TARGET_LINES", 95000),
		CompactKeep: envInt("COMPACT_KEEP_LINES", 2000), MaxPageSize: envInt("MAX_PAGE_SIZE", 1000),
		MaxBodySize: envInt64("MAX_BODY_SIZE", 1048576), MaxMessageSize: envInt64("MAX_MESSAGE_SIZE", 262144),
		MaxMetadataSize: envInt64("MAX_METADATA_SIZE", 262144), InactiveAfter: envDuration("INACTIVE_AFTER", 5*time.Minute),
		DeleteAfter: envDuration("DELETE_AFTER", 168*time.Hour), CleanupInterval: envDuration("CLEANUP_INTERVAL", time.Minute),
		HealthcheckInterval: envDuration("HEALTHCHECK_INTERVAL", time.Minute), SSEHeartbeat: envDuration("SSE_HEARTBEAT_INTERVAL", 20*time.Second),
		LogCountsAsActivity: envBool("LOG_COUNTS_AS_ACTIVITY", true), CORS: envBool("CORS_ENABLED", false),
		RateLimit: envBool("RATE_LIMIT_ENABLED", false), RateRequests: envInt("RATE_LIMIT_REQUESTS", 120),
		RateWindow:  envDuration("RATE_LIMIT_WINDOW", time.Minute),
		AuthEnabled: authEnabled, AppPassword: appPassword,
		SSEBuffer: envInt("SSE_CLIENT_BUFFER", 100), SSEMaxClients: envInt("SSE_MAX_CLIENTS_PER_SENDER", 100),
		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		PublicURL:       strings.TrimRight(env("APP_PUBLIC_URL", "http://localhost:8080"), "/"),
		EmailProvider:   emailProvider, OutlookEnabled: outlookEnabled, OutlookEnabledManaged: outlookEnabledManaged,
		OutlookTenantID: outlookTenantID, OutlookClientID: outlookClientID,
		OutlookClientSecret: outlookClientSecret, OutlookSenderEmail: outlookSenderEmail,
		OutlookSenderName: env("OUTLOOK_SENDER_NAME", "LogMate"), EmailSettingsEncryptionKey: strings.TrimSpace(os.Getenv("EMAIL_SETTINGS_ENCRYPTION_KEY")),
		SMTPHost: smtpHost, SMTPPort: smtpPort, SMTPUsername: smtpUsername, SMTPPassword: smtpPassword,
		SMTPFrom: smtpFrom, SMTPSenderName: env("SMTP_SENDER_NAME", "LogMate"), SMTPEnabled: smtpEnabled,
		EmailAlertQueueSize: envInt("EMAIL_ALERT_QUEUE_SIZE", 1000), EmailAlertWorkers: envInt("EMAIL_ALERT_WORKERS", 2),
		EmailAlertMaxRetries: envInt("EMAIL_ALERT_MAX_RETRIES", 3), EmailAlertSendTimeout: envDuration("EMAIL_ALERT_SEND_TIMEOUT", 30*time.Second),
		EmailAlertRetryInterval:       envDuration("EMAIL_ALERT_RETRY_INTERVAL", 5*time.Second),
		ExecutionHistoryRetentionDays: envInt("EXECUTION_HISTORY_RETENTION_DAYS", 90), ExecutionHistoryMaxRecords: envInt("EXECUTION_HISTORY_MAX_RECORDS", 100000),
		ExecutionHistoryCleanupInterval: envDuration("EXECUTION_HISTORY_CLEANUP_INTERVAL", time.Hour),
	}
	c.EmailManagedByEnvironment = anyEnvironmentSet(
		"OUTLOOK_ENABLED", "OUTLOOK_TENANT_ID", "OUTLOOK_CLIENT_ID",
		"OUTLOOK_CLIENT_SECRET", "OUTLOOK_SENDER_EMAIL", "OUTLOOK_SENDER_NAME", "EMAIL_FROM_ADDR", "EMAIL_USER",
		"O365_TENANT_ID", "O365_CLIENT_ID", "O365_CLIENT_SECRET",
	)
	c.SMTPManagedByEnvironment = anyEnvironmentSet("SMTP_ENABLED", "SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_SENDER_NAME")
	if v := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS")); v != "" {
		for _, item := range strings.Split(v, ",") {
			c.AllowedOrigins = append(c.AllowedOrigins, strings.TrimSpace(item))
		}
	}
	if c.CompactTarget >= c.MaxLogLines || c.CompactTarget < 1 || c.CompactKeep < 0 || c.MaxPageSize < 1 {
		return c, fmt.Errorf("invalid compaction or pagination configuration")
	}
	if c.AuthEnabled && c.AppPassword == "" {
		return c, fmt.Errorf("APP_PASSWORD is required when authentication is enabled")
	}
	encryptionKey, err := resolveEmailEncryptionKey(c.DataDir, c.EmailSettingsEncryptionKey)
	if err != nil {
		return c, err
	}
	c.EmailSettingsEncryptionKey = encryptionKey
	if c.EmailProvider != "outlook" && c.EmailProvider != "gmail" {
		return c, fmt.Errorf("EMAIL_PROVIDER must be outlook or gmail")
	}
	if c.SMTPPort < 1 || c.SMTPPort > 65535 {
		return c, fmt.Errorf("SMTP_PORT must be between 1 and 65535")
	}
	publicURL, publicURLError := url.ParseRequestURI(c.PublicURL)
	if publicURLError != nil || (publicURL.Scheme != "http" && publicURL.Scheme != "https") || publicURL.Host == "" {
		return c, fmt.Errorf("APP_PUBLIC_URL must be an absolute http or https URL")
	}
	if c.EmailAlertQueueSize < 1 || c.EmailAlertWorkers < 1 || c.EmailAlertMaxRetries < 0 || c.EmailAlertSendTimeout <= 0 || c.EmailAlertRetryInterval < 0 {
		return c, fmt.Errorf("invalid email alert queue configuration")
	}
	if c.ExecutionHistoryRetentionDays < 1 || c.ExecutionHistoryMaxRecords < 1 || c.ExecutionHistoryCleanupInterval <= 0 {
		return c, fmt.Errorf("invalid execution history configuration")
	}
	return c, nil
}

func (c Config) Address() string { return c.Host + ":" + c.Port }

const emailEncryptionKeyFile = "email-encryption.key"

func resolveEmailEncryptionKey(dataDir, fromEnv string) (string, error) {
	if key, ok := validEmailEncryptionKey(fromEnv); ok {
		return key, nil
	}
	path := filepath.Join(dataDir, emailEncryptionKeyFile)
	if raw, err := os.ReadFile(path); err == nil {
		if key, ok := validEmailEncryptionKey(string(raw)); ok {
			return key, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate EMAIL_SETTINGS_ENCRYPTION_KEY: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(buf)
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return "", fmt.Errorf("create data dir for encryption key: %w", err)
	}
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("persist EMAIL_SETTINGS_ENCRYPTION_KEY: %w", err)
	}
	return encoded, nil
}

func validEmailEncryptionKey(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return "", false
	}
	return value, true
}

func loadDotEnv(paths ...string) {
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" {
			continue
		}
		candidates := []string{path}
		if !filepath.IsAbs(path) {
			if cwd, err := os.Getwd(); err == nil {
				candidates = append([]string{filepath.Join(cwd, path)}, candidates...)
			}
		}
		for _, candidate := range candidates {
			if seen[candidate] {
				continue
			}
			seen[candidate] = true
			_ = applyDotEnvFile(candidate)
		}
	}
}

func applyDotEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if value[0] == '"' && value[len(value)-1] == '"' {
				value = strings.ReplaceAll(value[1:len(value)-1], `\"`, `"`)
			} else if value[0] == '\'' && value[len(value)-1] == '\'' {
				value = value[1 : len(value)-1]
			}
		}
		_ = os.Setenv(key, value)
	}
	return scanner.Err()
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func envInt(k string, d int) int {
	v, e := strconv.Atoi(env(k, strconv.Itoa(d)))
	if e != nil {
		return d
	}
	return v
}
func envInt64(k string, d int64) int64 {
	v, e := strconv.ParseInt(env(k, strconv.FormatInt(d, 10)), 10, 64)
	if e != nil {
		return d
	}
	return v
}
func envBool(k string, d bool) bool {
	v, e := strconv.ParseBool(env(k, strconv.FormatBool(d)))
	if e != nil {
		return d
	}
	return v
}
func envDuration(k string, d time.Duration) time.Duration {
	v, e := time.ParseDuration(env(k, d.String()))
	if e != nil {
		return d
	}
	return v
}

func anyEnvironmentSet(keys ...string) bool {
	for _, key := range keys {
		if _, exists := os.LookupEnv(key); exists {
			return true
		}
	}
	return false
}

func firstEnvironment(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
