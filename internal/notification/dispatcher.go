package notification

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"logtheater/internal/domain"
	"logtheater/internal/emailprovider"
)

var (
	ErrQueueFull   = errors.New("email alert queue is full")
	ErrQueueClosed = errors.New("email alert queue is closed")
)

type NotificationDispatcher interface {
	Dispatch(context.Context, domain.Notification) error
}

type DeliveryRecorder interface {
	MarkPending(id string, test bool) error
	RecordDelivery(id string, test bool, status domain.DeliveryStatus, message string) error
}

type executionDeliveryRecorder interface {
	MarkExecutionProcessing(domain.Notification)
	RecordExecutionDelivery(domain.Notification, domain.DeliveryStatus, string, int)
}

type Renderer interface {
	Render(domain.Notification) (domain.EmailMessage, error)
}

type WebhookSender interface {
	Send(context.Context, domain.Notification) error
}

type SMSSender interface {
	Send(context.Context, domain.Notification) error
}

type Dispatcher struct {
	outbox        *OutboxStore
	wake          chan struct{}
	stop          chan struct{}
	rejections    chan domain.Notification
	provider      emailprovider.Provider
	renderer      Renderer
	recorder      DeliveryRecorder
	webhookSender WebhookSender
	smsSender     SMSSender
	workers       int
	maxRetries    int
	sendTimeout   time.Duration
	retryInterval time.Duration
	leaseDuration time.Duration
	workerOwner   string
	queueMu       sync.RWMutex
	closed        bool
	workerWG      sync.WaitGroup
	startOnce     sync.Once
	closeOnce     sync.Once
	ctx           context.Context
	cancel        context.CancelFunc
}

func (d *Dispatcher) SetWebhookSender(sender WebhookSender) *Dispatcher {
	d.webhookSender = sender
	return d
}

func (d *Dispatcher) SetSMSSender(sender SMSSender) *Dispatcher {
	d.smsSender = sender
	return d
}

func NewDispatcher(dataDir string, size, workers, maxRetries int, timeout, retryInterval time.Duration, provider emailprovider.Provider, renderer Renderer, recorder DeliveryRecorder) (*Dispatcher, error) {
	outbox, err := OpenOutbox(dataDir, size, time.Now)
	if err != nil {
		return nil, err
	}
	workerOwner, err := newOutboxID()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	leaseDuration := time.Duration(maxRetries+1)*timeout + time.Duration(maxRetries)*retryInterval + time.Minute
	return &Dispatcher{outbox: outbox, wake: make(chan struct{}, 1), stop: make(chan struct{}), rejections: make(chan domain.Notification, size), workers: workers, maxRetries: maxRetries, sendTimeout: timeout, retryInterval: retryInterval, leaseDuration: leaseDuration, workerOwner: workerOwner, provider: provider, renderer: renderer, recorder: recorder, ctx: ctx, cancel: cancel}, nil
}

func (d *Dispatcher) Start() {
	d.startOnce.Do(func() {
		d.workerWG.Add(1)
		go d.rejectionWorker()
		for index := 0; index < d.workers; index++ {
			d.workerWG.Add(1)
			go d.worker()
		}
	})
}

func (d *Dispatcher) ReportRejected(value domain.Notification) {
	_ = d.TryReportRejected(value)
}

func (d *Dispatcher) TryReportRejected(value domain.Notification) bool {
	d.queueMu.RLock()
	defer d.queueMu.RUnlock()
	if d.closed {
		return false
	}
	select {
	case d.rejections <- value:
		return true
	default:
		return false
	}
}

func (d *Dispatcher) Enqueue(value domain.Notification) error {
	return d.Dispatch(context.Background(), value)
}

func (d *Dispatcher) Dispatch(ctx context.Context, value domain.Notification) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.queueMu.RLock()
	if d.closed {
		d.queueMu.RUnlock()
		return ErrQueueClosed
	}
	_, err := d.outbox.Enqueue(ctx, value)
	d.queueMu.RUnlock()
	if err != nil {
		return err
	}
	select {
	case d.wake <- struct{}{}:
	default:
	}
	return nil
}

func (d *Dispatcher) Shutdown(ctx context.Context) error {
	d.closeOnce.Do(func() {
		d.queueMu.Lock()
		d.closed = true
		close(d.stop)
		close(d.rejections)
		d.queueMu.Unlock()
	})
	done := make(chan struct{})
	go func() {
		d.workerWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		d.cancel()
		return ctx.Err()
	}
}

func (d *Dispatcher) rejectionWorker() {
	defer d.workerWG.Done()
	for value := range d.rejections {
		message := "The notification queue is full; the log was preserved, but the action was not queued."
		_ = d.recorder.RecordDelivery(notificationID(value), value.Test, domain.DeliveryFailed, message)
		if recorder, ok := d.recorder.(executionDeliveryRecorder); ok {
			recorder.RecordExecutionDelivery(value, domain.DeliveryFailed, message, 0)
		}
	}
}

func (d *Dispatcher) worker() {
	defer d.workerWG.Done()
	for {
		select {
		case <-d.stop:
			return
		default:
		}
		job, ok, err := d.outbox.Claim(d.ctx, d.workerOwner, d.leaseDuration)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			slog.Error("could not claim notification outbox job", "error", err)
			d.waitForWork()
			continue
		}
		if !ok {
			d.waitForWork()
			continue
		}
		if d.deliverSafely(job.Notification) {
			if err = d.outbox.Complete(context.Background(), job.ID, d.workerOwner); err != nil {
				slog.Error("could not complete notification outbox job", "job_id", job.ID, "error", err)
			}
			continue
		}
		if err = d.outbox.Retry(context.Background(), job.ID, d.workerOwner, "Delivery interrupted by shutdown.", time.Now()); err != nil {
			slog.Error("could not release interrupted notification outbox job", "job_id", job.ID, "error", err)
		}
	}
}

func (d *Dispatcher) waitForWork() {
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-d.stop:
	case <-d.wake:
	case <-timer.C:
	case <-d.ctx.Done():
	}
}

func (d *Dispatcher) deliverSafely(value domain.Notification) (completed bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message := "Internal failure while preparing the notification."
			_ = d.recorder.RecordDelivery(notificationID(value), value.Test, domain.DeliveryFailed, message)
			if recorder, ok := d.recorder.(executionDeliveryRecorder); ok {
				recorder.RecordExecutionDelivery(value, domain.DeliveryFailed, message, 1)
			}
			slog.Error("email notification worker recovered", "source_type", notificationSource(value), "source_id", notificationID(value))
			completed = true
		}
	}()
	if err := d.recorder.MarkPending(notificationID(value), value.Test); err != nil {
		slog.Error("could not mark email notification pending", "source_type", notificationSource(value), "source_id", notificationID(value), "error", err)
	}
	if recorder, ok := d.recorder.(executionDeliveryRecorder); ok {
		recorder.MarkExecutionProcessing(value)
	}
	var send func(context.Context) error
	if value.Event.ActionType == domain.EventActionWebhook {
		if d.webhookSender == nil {
			return d.recordTerminalFailure(value, "The webhook sender is unavailable.", 1)
		}
		send = func(ctx context.Context) error { return d.webhookSender.Send(ctx, value) }
	} else if value.Event.ActionType == domain.EventActionSMS {
		if d.smsSender == nil {
			return d.recordTerminalFailure(value, "The SMS sender is unavailable.", 1)
		}
		send = func(ctx context.Context) error { return d.smsSender.Send(ctx, value) }
	} else {
		message, err := d.renderer.Render(value)
		if err != nil {
			return d.recordTerminalFailure(value, "Unable to render the email.", 1)
		}
		send = func(ctx context.Context) error { return d.provider.Send(ctx, message) }
	}
	var err error
	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(d.ctx, d.sendTimeout)
		err = send(ctx)
		cancel()
		if err == nil {
			_ = d.recorder.RecordDelivery(notificationID(value), value.Test, domain.DeliverySent, "")
			if recorder, ok := d.recorder.(executionDeliveryRecorder); ok {
				recorder.RecordExecutionDelivery(value, domain.DeliverySent, "", attempt+1)
			}
			slog.Info("email notification delivered", "source_type", notificationSource(value), "source_id", notificationID(value), "sender_id", value.Sender.ID, "severity", value.Entry.Severity, "test", value.Test, "attempt", attempt+1)
			return true
		}
		if attempt < d.maxRetries && d.retryInterval > 0 {
			slog.Warn("retrying email notification delivery", "source_type", notificationSource(value), "source_id", notificationID(value), "sender_id", value.Sender.ID, "severity", value.Entry.Severity, "attempt", attempt+2)
			timer := time.NewTimer(d.retryInterval)
			select {
			case <-timer.C:
			case <-d.ctx.Done():
				timer.Stop()
				return false
			}
		}
	}
	safe := safeDeliveryError(err)
	_ = d.recorder.RecordDelivery(notificationID(value), value.Test, domain.DeliveryFailed, safe)
	if recorder, ok := d.recorder.(executionDeliveryRecorder); ok {
		recorder.RecordExecutionDelivery(value, domain.DeliveryFailed, safe, d.maxRetries+1)
	}
	slog.Error("email notification delivery failed", "source_type", notificationSource(value), "source_id", notificationID(value), "sender_id", value.Sender.ID, "severity", value.Entry.Severity, "test", value.Test, "attempts", d.maxRetries+1, "error", safe)
	return true
}

func (d *Dispatcher) recordTerminalFailure(value domain.Notification, message string, attempts int) bool {
	_ = d.recorder.RecordDelivery(notificationID(value), value.Test, domain.DeliveryFailed, message)
	if recorder, ok := d.recorder.(executionDeliveryRecorder); ok {
		recorder.RecordExecutionDelivery(value, domain.DeliveryFailed, message, attempts)
	}
	return true
}

func notificationID(value domain.Notification) string {
	if value.SourceID != "" {
		return value.SourceID
	}
	if value.SourceType == domain.NotificationSourceEvent || value.Event.ID != "" {
		return value.Event.ID
	}
	return value.Alert.ID
}

func notificationSource(value domain.Notification) domain.NotificationSourceType {
	if value.SourceType != "" {
		return value.SourceType
	}
	if value.Event.ID != "" {
		return domain.NotificationSourceEvent
	}
	return domain.NotificationSourceAlert
}

func safeDeliveryError(err error) string {
	var providerError *emailprovider.Error
	if errors.As(err, &providerError) {
		return providerError.Message
	}
	if errors.Is(err, emailprovider.ErrNotConfigured) {
		return "The Outlook provider is not configured or enabled."
	}
	if err == nil {
		return "Unknown delivery failure."
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 240 {
		message = message[:240]
	}
	return message
}
