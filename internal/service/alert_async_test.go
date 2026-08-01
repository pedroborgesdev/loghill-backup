package service_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"logtheater/internal/alerts"
	"logtheater/internal/config"
	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
	"logtheater/internal/notification"
	"logtheater/internal/repository"
	"logtheater/internal/service"
	settingsstore "logtheater/internal/settings"
)

type blockingProvider struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingProvider) Provider() domain.EmailProviderType   { return domain.EmailProviderOutlook }
func (p *blockingProvider) TestConnection(context.Context) error { return nil }
func (p *blockingProvider) Send(context.Context, domain.EmailMessage) error {
	p.once.Do(func() { close(p.started) })
	<-p.release
	return nil
}

func TestReceiveLogDoesNotWaitForEmailDelivery(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	cfg := config.Config{DataDir: dir, MaxMessageSize: 1024, MaxMetadataSize: 1024, SSEBuffer: 10, SSEMaxClients: 10, EmailManagedByEnvironment: true, OutlookEnabled: true, OutlookTenantID: "tenant", OutlookClientID: "client", OutlookClientSecret: "secret", OutlookSenderEmail: "logs@example.com", OutlookSenderName: "LogHill"}
	repo := repository.New(dir)
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	settings, err := settingsstore.Open(dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	emailSettings, err := emailconfig.Open(dir, cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	alertStore, err := alerts.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	logService := service.New(repo, cfg, domain.SystemClock{}, settings)
	alertService := alerts.NewService(alertStore, repo, emailSettings, domain.SystemClock{})
	provider := &blockingProvider{started: make(chan struct{}), release: make(chan struct{})}
	dispatcher := notification.NewDispatcher(10, 1, 0, time.Second, 0, provider, notification.NewTemplate("http://localhost:8080"), alertService)
	dispatcher.Start()
	logService.SetAlertSink(notification.NewRuntime(alertService, dispatcher))
	sender, credentials, err := logService.CreateSender(context.Background(), "worker", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = alertService.Create(context.Background(), domain.AlertInput{Name: "Erros críticos", SenderIDs: []string{sender.ID}, Severities: []domain.LogSeverity{domain.Error}, Recipients: []string{"dev@example.com"}, Provider: domain.EmailProviderOutlook, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now()
	if _, _, err = logService.ReceiveLog(context.Background(), sender.ID, credentials.SenderKey, "ERROR", "falha", nil, nil); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(startedAt); elapsed >= 500*time.Millisecond {
		t.Fatalf("log ingestion waited for email: %s", elapsed)
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("email worker did not start")
	}
	close(provider.release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = dispatcher.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}
