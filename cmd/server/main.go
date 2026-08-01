package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"logtheater/internal/alerts"
	"logtheater/internal/config"
	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
	"logtheater/internal/emailprovider"
	"logtheater/internal/events"
	"logtheater/internal/handler"
	"logtheater/internal/notification"
	"logtheater/internal/repository"
	"logtheater/internal/scheduler"
	"logtheater/internal/service"
	settingsstore "logtheater/internal/settings"
	webassets "logtheater/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	repo := repository.New(cfg.DataDir)
	if err = repo.Init(); err != nil {
		slog.Error("storage initialization failed", "error", err)
		os.Exit(1)
	}
	clock := domain.SystemClock{}
	settings, err := settingsstore.Open(cfg.DataDir, clock.Now())
	if err != nil {
		slog.Error("settings initialization failed", "error", err)
		os.Exit(1)
	}
	emailSettings, err := emailconfig.Open(cfg.DataDir, cfg, clock.Now())
	if err != nil {
		slog.Error("email settings initialization failed", "error", err)
		os.Exit(1)
	}
	alertStore, err := alerts.Open(cfg.DataDir)
	if err != nil {
		slog.Error("alert storage initialization failed", "error", err)
		os.Exit(1)
	}
	eventStore, err := events.Open(cfg.DataDir)
	if err != nil {
		slog.Error("event storage initialization failed", "error", err)
		os.Exit(1)
	}
	svc := service.New(repo, cfg, clock, settings)
	alertService := alerts.NewService(alertStore, repo, emailSettings, clock)
	eventService := events.NewService(eventStore, repo, emailSettings, clock)
	outlookProvider := emailprovider.NewOutlook(emailSettings, cfg.EmailAlertSendTimeout)
	emailTemplate := notification.NewTemplate(cfg.PublicURL)
	recorder := notification.NewRecorder(alertService, eventService)
	dispatcher := notification.NewDispatcher(cfg.EmailAlertQueueSize, cfg.EmailAlertWorkers, cfg.EmailAlertMaxRetries, cfg.EmailAlertSendTimeout, cfg.EmailAlertRetryInterval, outlookProvider, emailTemplate, recorder)
	dispatcher.Start()
	svc.SetAlertSink(notification.NewRuntime(alertService, dispatcher))
	svc.SetEventSink(notification.NewEventRuntime(eventService, dispatcher))
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err = svc.Restore(ctx); err != nil {
		slog.Error("state restoration failed", "error", err)
		os.Exit(1)
	}
	sched := scheduler.New(svc, cfg.CleanupInterval)
	go sched.Run(ctx)
	for i := 0; i < 100 && !sched.Ready(); i++ {
		time.Sleep(time.Millisecond)
	}
	assets, err := webassets.Dist()
	if err != nil {
		slog.Error("frontend assets unavailable", "error", err)
		os.Exit(1)
	}
	api := handler.New(svc, cfg, sched, assets).ConfigureNotifications(alertService, emailSettings, outlookProvider, dispatcher).ConfigureEvents(eventService)
	server := &http.Server{Addr: cfg.Address(), Handler: api.Router(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 0, IdleTimeout: 60 * time.Second}
	done := make(chan error, 1)
	go func() { slog.Info("server started", "address", cfg.Address()); done <- server.ListenAndServe() }()
	select {
	case err = <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	if err = server.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
	if err = dispatcher.Shutdown(shutdownCtx); err != nil {
		slog.Error("email alert dispatcher shutdown failed", "error", err)
	}
	svc.Hub.Close()
}
