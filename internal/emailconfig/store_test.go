package emailconfig

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"logtheater/internal/config"
	"logtheater/internal/domain"
)

func TestSecretIsEncryptedHiddenAndPreserved(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{EmailSettingsEncryptionKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))}
	store, err := Open(dir, cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	input := Input{Provider: domain.EmailProviderOutlook, Enabled: true, Outlook: OutlookInput{TenantID: "tenant", ClientID: "client", ClientSecret: "super-secret", SenderEmail: "logs@example.com", SenderName: "LogHill"}}
	safe, err := store.Update(input, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(safe)
	if bytes.Contains(encoded, []byte("super-secret")) || !safe.Outlook.ClientSecretConfigured {
		t.Fatalf("secret leaked in safe response: %s", encoded)
	}
	persisted, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(persisted, []byte("super-secret")) {
		t.Fatal("secret was stored in plaintext")
	}
	input.Outlook.ClientSecret = ""
	if _, err = store.Update(input, time.Now()); err != nil {
		t.Fatal(err)
	}
	runtime, err := store.Runtime()
	if err != nil || runtime.ClientSecret != "super-secret" {
		t.Fatalf("secret was not preserved: %+v %v", runtime, err)
	}
	reopened, err := Open(dir, cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	runtime, err = reopened.Runtime()
	if err != nil || runtime.ClientSecret != "super-secret" {
		t.Fatalf("secret was not restored: %+v %v", runtime, err)
	}
}

func TestGmailAndPlaintextSecretAreRejected(t *testing.T) {
	store, err := Open(t.TempDir(), config.Config{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Update(Input{Provider: domain.EmailProviderGmail}, time.Now())
	if err == nil {
		t.Fatal("gmail should be rejected")
	}
	_, err = store.Update(Input{Provider: domain.EmailProviderOutlook, Outlook: OutlookInput{ClientSecret: "secret"}}, time.Now())
	if err == nil {
		t.Fatal("secret without encryption key should be rejected")
	}
}
