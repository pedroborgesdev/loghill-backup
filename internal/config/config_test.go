package config

import "testing"

func TestO365EnvironmentAliases(t *testing.T) {
	t.Setenv("EMAIL_PROVIDER", "o365")
	t.Setenv("OUTLOOK_ENABLED", "true")
	t.Setenv("OUTLOOK_TENANT_ID", "")
	t.Setenv("OUTLOOK_CLIENT_ID", "")
	t.Setenv("OUTLOOK_CLIENT_SECRET", "")
	t.Setenv("OUTLOOK_SENDER_EMAIL", "")
	t.Setenv("O365_TENANT_ID", "legacy-tenant")
	t.Setenv("O365_CLIENT_ID", "legacy-client")
	t.Setenv("O365_CLIENT_SECRET", "legacy-secret")
	t.Setenv("EMAIL_FROM_ADDR", "logs@example.com")
	cfg, err := Load()
	if err != nil { t.Fatal(err) }
	if cfg.EmailProvider != "outlook" || cfg.OutlookTenantID != "legacy-tenant" || cfg.OutlookClientID != "legacy-client" || cfg.OutlookClientSecret != "legacy-secret" || cfg.OutlookSenderEmail != "logs@example.com" || !cfg.OutlookEnabled { t.Fatalf("aliases not loaded: %+v", cfg) }
}

func TestPublicURLMustBeSafeAndAbsolute(t *testing.T) {
	t.Setenv("APP_PUBLIC_URL", "javascript:alert(1)")
	if _, err := Load(); err == nil { t.Fatal("unsafe public URL should be rejected") }
}

