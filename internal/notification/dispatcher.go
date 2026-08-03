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

type Dispatcher struct {
	queue         chan domain.Notification
	rejections    chan domain.Notification
	provider      emailprovider.Provider
	renderer      Renderer
	recorder      DeliveryRecorder
	workers       int
	maxRetries    int
	sendTimeout   time.Duration
	retryInterval time.Duration
	queueMu       sync.RWMutex
	closed        bool
	workerWG      sync.WaitGroup
	startOnce     sync.Once
	closeOnce     sync.Once
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewDispatcher(size, workers, maxRetries int, timeout, retryInterval time.Duration, provider emailprovider.Provider, renderer Renderer, recorder DeliveryRecorder) *Dispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &Dispatcher{queue: make(chan domain.Notification, size), rejections: make(chan domain.Notification, size), workers: workers, maxRetries: maxRetries, sendTimeout: timeout, retryInterval: retryInterval, provider: provider, renderer: renderer, recorder: recorder, ctx: ctx, cancel: cancel}
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
	d.queueMu.RLock()
	defer d.queueMu.RUnlock()
	if d.closed {
		return ErrQueueClosed
	}
	select {
	case d.queue <- value:
		return nil
	default:
		return ErrQueueFull
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context, value domain.Notification) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return d.Enqueue(value)
}

func (d *Dispatcher) Shutdown(ctx context.Context) error {
	d.closeOnce.Do(func() {
		d.queueMu.Lock()
		d.closed = true
		close(d.queue)
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
		message := "A fila de notificações está cheia; o log foi preservado, mas o e-mail não foi enfileirado."
		_ = d.recorder.RecordDelivery(notificationID(value), value.Test, domain.DeliveryFailed, message)
		if recorder, ok := d.recorder.(executionDeliveryRecorder); ok {
			recorder.RecordExecutionDelivery(value, domain.DeliveryFailed, message, 0)
		}
	}
}

func (d *Dispatcher) worker() {
	defer d.workerWG.Done()
	for value := range d.queue {
		d.deliverSafely(value)
	}
}

func (d *Dispatcher) deliverSafely(value domain.Notification) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message := "Falha interna ao preparar a notificação."
			_ = d.recorder.RecordDelivery(notificationID(value), value.Test, domain.DeliveryFailed, message)
			if recorder, ok := d.recorder.(executionDeliveryRecorder); ok {
				recorder.RecordExecutionDelivery(value, domain.DeliveryFailed, message, 1)
			}
			slog.Error("email notification worker recovered", "source_type", notificationSource(value), "source_id", notificationID(value))
		}
	}()
	if err := d.recorder.MarkPending(notificationID(value), value.Test); err != nil {
		slog.Error("could not mark email notification pending", "source_type", notificationSource(value), "source_id", notificationID(value), "error", err)
	}
	if recorder, ok := d.recorder.(executionDeliveryRecorder); ok {
		recorder.MarkExecutionProcessing(value)
	}
	message, err := d.renderer.Render(value)
	if err != nil {
		_ = d.recorder.RecordDelivery(notificationID(value), value.Test, domain.DeliveryFailed, "Não foi possível renderizar o e-mail.")
		if recorder, ok := d.recorder.(executionDeliveryRecorder); ok {
			recorder.RecordExecutionDelivery(value, domain.DeliveryFailed, "Não foi possível renderizar o e-mail.", 1)
		}
		return
	}
	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(d.ctx, d.sendTimeout)
		err = d.provider.Send(ctx, message)
		cancel()
		if err == nil {
			_ = d.recorder.RecordDelivery(notificationID(value), value.Test, domain.DeliverySent, "")
			if recorder, ok := d.recorder.(executionDeliveryRecorder); ok {
				recorder.RecordExecutionDelivery(value, domain.DeliverySent, "", attempt+1)
			}
			slog.Info("email notification delivered", "source_type", notificationSource(value), "source_id", notificationID(value), "sender_id", value.Sender.ID, "severity", value.Entry.Severity, "test", value.Test, "attempt", attempt+1)
			return
		}
		if attempt < d.maxRetries && d.retryInterval > 0 {
			slog.Warn("retrying email notification delivery", "source_type", notificationSource(value), "source_id", notificationID(value), "sender_id", value.Sender.ID, "severity", value.Entry.Severity, "attempt", attempt+2)
			timer := time.NewTimer(d.retryInterval)
			select {
			case <-timer.C:
			case <-d.ctx.Done():
				timer.Stop()
				return
			}
		}
	}
	safe := safeDeliveryError(err)
	_ = d.recorder.RecordDelivery(notificationID(value), value.Test, domain.DeliveryFailed, safe)
	if recorder, ok := d.recorder.(executionDeliveryRecorder); ok {
		recorder.RecordExecutionDelivery(value, domain.DeliveryFailed, safe, d.maxRetries+1)
	}
	slog.Error("email notification delivery failed", "source_type", notificationSource(value), "source_id", notificationID(value), "sender_id", value.Sender.ID, "severity", value.Entry.Severity, "test", value.Test, "attempts", d.maxRetries+1, "error", safe)
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
		return "O provedor Outlook não está configurado ou habilitado."
	}
	if err == nil {
		return "Falha desconhecida no envio."
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 240 {
		message = message[:240]
	}
	return message
}
