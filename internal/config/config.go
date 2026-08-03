package config

import (
	"fmt"
	"net/url"
	"os"
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
	APIKeyEnabled, AdminAuthEnabled                                bool
	APIKey, AdminAPIKey                                            string
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
		RateWindow: envDuration("RATE_LIMIT_WINDOW", time.Minute), APIKeyEnabled: envBool("API_KEY_ENABLED", false),
		AdminAuthEnabled: envBool("ADMIN_AUTH_ENABLED", false), APIKey: os.Getenv("API_KEY"), AdminAPIKey: os.Getenv("ADMIN_API_KEY"),
		SSEBuffer: envInt("SSE_CLIENT_BUFFER", 100), SSEMaxClients: envInt("SSE_MAX_CLIENTS_PER_SENDER", 100),
		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		PublicURL:       strings.TrimRight(env("APP_PUBLIC_URL", "http://localhost:8080"), "/"),
		EmailProvider:   emailProvider, OutlookEnabled: outlookEnabled, OutlookEnabledManaged: outlookEnabledManaged,
		OutlookTenantID: outlookTenantID, OutlookClientID: outlookClientID,
		OutlookClientSecret: outlookClientSecret, OutlookSenderEmail: outlookSenderEmail,
		OutlookSenderName: env("OUTLOOK_SENDER_NAME", "LogHill"), EmailSettingsEncryptionKey: env("EMAIL_SETTINGS_ENCRYPTION_KEY", "NHYVqkvuHied51HKZjaREtdzdbGsKsOkU+62pzs+Q7Q="),
		SMTPHost: smtpHost, SMTPPort: smtpPort, SMTPUsername: smtpUsername, SMTPPassword: smtpPassword,
		SMTPFrom: smtpFrom, SMTPSenderName: env("SMTP_SENDER_NAME", "LogHill"), SMTPEnabled: smtpEnabled,
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
	if c.APIKeyEnabled && c.APIKey == "" {
		return c, fmt.Errorf("API_KEY is required when API_KEY_ENABLED=true")
	}
	if c.AdminAuthEnabled && c.AdminAPIKey == "" {
		return c, fmt.Errorf("ADMIN_API_KEY is required when ADMIN_AUTH_ENABLED=true")
	}
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
