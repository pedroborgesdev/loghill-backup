package notification

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"logtheater/internal/domain"
)

type fakeProvider struct {
	mu             sync.Mutex
	calls          int
	failFor        int
	waitForContext bool
}

func (p *fakeProvider) Provider() domain.EmailProviderType   { return domain.EmailProviderOutlook }
func (p *fakeProvider) TestConnection(context.Context) error { return nil }
func (p *fakeProvider) Send(ctx context.Context, _ domain.EmailMessage) error {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if p.waitForContext {
		<-ctx.Done()
		return ctx.Err()
	}
	if call <= p.failFor {
		return errors.New("temporary")
	}
	return nil
}
func (p *fakeProvider) count() int { p.mu.Lock(); defer p.mu.Unlock(); return p.calls }

type fakeRenderer struct{}

func (fakeRenderer) Render(value domain.Notification) (domain.EmailMessage, error) {
	return domain.EmailMessage{To: value.Alert.Recipients, Subject: "test", Text: "test", HTML: "test"}, nil
}

type fakeWebhookSender struct {
	mu    sync.Mutex
	calls int
}

func (s *fakeWebhookSender) Send(context.Context, domain.Notification) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return nil
}

func (s *fakeWebhookSender) count() int { s.mu.Lock(); defer s.mu.Unlock(); return s.calls }

type recordedDelivery struct {
	status  domain.DeliveryStatus
	test    bool
	message string
}
type fakeRecorder struct {
	mu     sync.Mutex
	values []recordedDelivery
	done   chan struct{}
	once   sync.Once
}

func newRecorder() *fakeRecorder                       { return &fakeRecorder{done: make(chan struct{})} }
func (r *fakeRecorder) MarkPending(string, bool) error { return nil }
func (r *fakeRecorder) RecordDelivery(_ string, test bool, status domain.DeliveryStatus, message string) error {
	r.mu.Lock()
	r.values = append(r.values, recordedDelivery{status: status, test: test, message: message})
	r.mu.Unlock()
	if status != domain.DeliveryPending {
		r.once.Do(func() { close(r.done) })
	}
	return nil
}

func testNotification() domain.Notification {
	return domain.Notification{Alert: domain.EmailAlert{ID: "alert-1", Recipients: []string{"dev@example.com"}}, Sender: domain.Sender{ID: "worker-1"}, Entry: domain.LogEntry{Severity: domain.Error}}
}

func TestDispatcherRetriesAndShutsDown(t *testing.T) {
	provider := &fakeProvider{failFor: 2}
	recorder := newRecorder()
	dispatcher, err := NewDispatcher(t.TempDir(), 2, 1, 2, time.Second, time.Millisecond, provider, fakeRenderer{}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.Start()
	if err := dispatcher.Enqueue(testNotification()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-recorder.done:
	case <-time.After(time.Second):
		t.Fatal("delivery timed out")
	}
	if provider.count() != 3 {
		t.Fatalf("expected 3 attempts, got %d", provider.count())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dispatcher.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDispatcherTimeoutQueueFullAndGracefulCancellation(t *testing.T) {
	provider := &fakeProvider{waitForContext: true}
	recorder := newRecorder()
	dispatcher, err := NewDispatcher(t.TempDir(), 1, 1, 1, 10*time.Millisecond, 0, provider, fakeRenderer{}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Enqueue(testNotification()); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.Enqueue(testNotification()); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected full queue, got %v", err)
	}
	dispatcher.Start()
	select {
	case <-recorder.done:
	case <-time.After(time.Second):
		t.Fatal("timeout was not recorded")
	}
	if provider.count() != 2 {
		t.Fatalf("expected timeout retry, got %d", provider.count())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dispatcher.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDispatcherRecoversPersistedNotificationAfterRestart(t *testing.T) {
	dir := t.TempDir()
	firstRecorder := newRecorder()
	first, err := NewDispatcher(dir, 2, 1, 0, time.Second, 0, &fakeProvider{}, fakeRenderer{}, firstRecorder)
	if err != nil {
		t.Fatal(err)
	}
	if err = first.Enqueue(testNotification()); err != nil {
		t.Fatal(err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = first.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}

	provider := &fakeProvider{}
	recorder := newRecorder()
	second, err := NewDispatcher(dir, 2, 1, 0, time.Second, 0, provider, fakeRenderer{}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	second.Start()
	select {
	case <-recorder.done:
	case <-time.After(time.Second):
		t.Fatal("persisted notification was not recovered")
	}
	if provider.count() != 1 {
		t.Fatalf("expected one recovered delivery, got %d", provider.count())
	}
	if err = second.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
}

func TestDispatcherDeliversWebhookThroughDurableWorker(t *testing.T) {
	email := &fakeProvider{}
	webhook := &fakeWebhookSender{}
	recorder := newRecorder()
	dispatcher, err := NewDispatcher(t.TempDir(), 2, 1, 0, time.Second, 0, email, fakeRenderer{}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.SetWebhookSender(webhook).Start()
	value := domain.Notification{SourceType: domain.NotificationSourceEvent, SourceID: "evt-webhook", Event: domain.EventDefinition{ID: "evt-webhook", ActionType: domain.EventActionWebhook, WebhookURL: "https://hooks.example.com/logmate"}, Sender: domain.Sender{ID: "worker-1"}, Entry: domain.LogEntry{Event: "finished", Severity: domain.Info}}
	if err = dispatcher.Dispatch(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	select {
	case <-recorder.done:
	case <-time.After(time.Second):
		t.Fatal("webhook delivery timed out")
	}
	if webhook.count() != 1 || email.count() != 0 {
		t.Fatalf("webhook calls=%d email calls=%d", webhook.count(), email.count())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = dispatcher.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDispatcherRoutesHTTPRequestThroughDurableWorker(t *testing.T) {
	email := &fakeProvider{}
	httpSender := &fakeWebhookSender{}
	recorder := newRecorder()
	dispatcher, err := NewDispatcher(t.TempDir(), 2, 1, 0, time.Second, 0, email, fakeRenderer{}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.SetWebhookSender(httpSender).Start()
	value := domain.Notification{SourceType: domain.NotificationSourceEvent, SourceID: "evt-http", Event: domain.EventDefinition{ID: "evt-http", ActionType: domain.EventActionHTTP, HTTPRequest: &domain.HTTPRequestConfig{Method: "POST", URL: "https://api.example.com"}}, Sender: domain.Sender{ID: "worker-1"}, Entry: domain.LogEntry{Event: "finished", Severity: domain.Info}}
	if err = dispatcher.Dispatch(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	select {
	case <-recorder.done:
	case <-time.After(time.Second):
		t.Fatal("HTTP delivery timed out")
	}
	if httpSender.count() != 1 || email.count() != 0 {
		t.Fatalf("HTTP calls=%d email calls=%d", httpSender.count(), email.count())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = dispatcher.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDispatcherRejectsUnavailableEventActionWithoutSendingEmail(t *testing.T) {
	email := &fakeProvider{}
	recorder := newRecorder()
	dispatcher, err := NewDispatcher(t.TempDir(), 2, 1, 0, time.Second, 0, email, fakeRenderer{}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.Start()
	value := domain.Notification{SourceType: domain.NotificationSourceEvent, SourceID: "evt-retired", Event: domain.EventDefinition{ID: "evt-retired", ActionType: "retired"}, Sender: domain.Sender{ID: "worker-1"}, Entry: domain.LogEntry{Event: "finished", Severity: domain.Info}}
	if err = dispatcher.Dispatch(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	select {
	case <-recorder.done:
	case <-time.After(time.Second):
		t.Fatal("retired action rejection timed out")
	}
	if email.count() != 0 {
		t.Fatalf("retired action unexpectedly sent %d emails", email.count())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = dispatcher.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateEscapesHTMLAndKeepsFullBody(t *testing.T) {
	renderer := NewTemplate("https://logs.example.com/base")
	message, err := renderer.Render(domain.Notification{Alert: domain.EmailAlert{Name: "Critical", Recipients: []string{"dev@example.com"}}, Sender: domain.Sender{ID: "worker-1", Name: "worker", Status: domain.StatusOnline}, Entry: domain.LogEntry{Timestamp: time.Now(), Severity: domain.Error, Message: "<script>alert('x')</script>", Metadata: map[string]any{"html": "<img>"}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(message.HTML, "<script>") || strings.Contains(message.HTML, "<img>") || !strings.Contains(message.HTML, "&lt;script&gt;") || !strings.Contains(message.HTML, `\u003cimg\u003e`) {
		t.Fatalf("unsafe html: %s", message.HTML)
	}
	if !strings.Contains(message.Text, "<script>") || !strings.Contains(message.HTML, "/senders/worker-1?severity=ERROR") {
		t.Fatal("plain body or link missing")
	}
	if strings.Contains(message.Subject, "[") || !strings.HasPrefix(message.Subject, "LogMate — Error detected on worker:") {
		t.Fatalf("subject is not readable: %q", message.Subject)
	}
	for _, expected := range []string{`align="center"`, "Observability center", "An error was detected", "#f59e0b", "/logmate.png"} {
		if !strings.Contains(message.HTML, expected) {
			t.Fatalf("themed email is missing %q", expected)
		}
	}
	if strings.Contains(message.HTML, "ZgotmplZ") {
		t.Fatal("dynamic email styles or URLs were rejected by html/template")
	}
}

func TestTemplateRendersBeautifulProviderTestWithoutBracketedSubject(t *testing.T) {
	renderer := NewTemplate("https://logs.example.com")
	message, err := renderer.RenderProviderTest("dev@example.com", domain.EmailProviderOutlook)
	if err != nil {
		t.Fatal(err)
	}
	if message.Subject != "LogMate email test — configuration complete" || strings.Contains(message.Subject, "[") {
		t.Fatalf("unexpected subject: %q", message.Subject)
	}
	if len(message.To) != 1 || message.To[0] != "dev@example.com" {
		t.Fatalf("unexpected recipients: %v", message.To)
	}
	for _, expected := range []string{"Your email is ready", "Microsoft 365 / Outlook", "Configuration validated", "Open LogMate", "https://logs.example.com/logmate.png"} {
		if !strings.Contains(message.HTML, expected) {
			t.Fatalf("provider test email is missing %q", expected)
		}
	}
	if !strings.Contains(message.Text, "The LogMate integration with Outlook is working") {
		t.Fatal("provider test plain-text fallback is incomplete")
	}
}

func TestTemplateRendersGmailProviderTestWithoutOutlookReferences(t *testing.T) {
	renderer := NewTemplate("https://logs.example.com")
	message, err := renderer.RenderProviderTest("dev@example.com", domain.EmailProviderGmail)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Gmail", "SMTP with STARTTLS", "Gmail via SMTP"} {
		if !strings.Contains(message.HTML, expected) && !strings.Contains(message.Text, expected) {
			t.Fatalf("gmail provider test email is missing %q", expected)
		}
	}
	if strings.Contains(message.HTML, "Outlook") || strings.Contains(message.HTML, "Microsoft 365") || strings.Contains(message.HTML, "Microsoft Graph") || strings.Contains(message.Text, "Outlook") || strings.Contains(message.Text, "Microsoft 365") {
		t.Fatal("gmail provider test email contains an Outlook reference")
	}
}

func TestEventTemplateRendersRestrictedVariablesAndEscapesHTML(t *testing.T) {
	renderer := NewTemplate("https://logs.example.com")
	value := domain.Notification{SourceType: domain.NotificationSourceEvent, Event: domain.EventDefinition{ID: "evt_1", Name: "Processing completed", Key: "processing_completed", Recipients: []string{"dev@example.com"}, SubjectTemplate: "Finalizado — {{metadata.protocol}} — {{sender.name}}\r\nInjected", MessageTemplate: "{{log.message}}\nAusente: {{metadata.ausente}}"}, Sender: domain.Sender{ID: "worker", Name: "Worker", Status: domain.StatusOnline}, Entry: domain.LogEntry{Timestamp: time.Now(), Severity: domain.Info, Message: "<script>alert('x')</script>", Event: "processing_completed", Metadata: map[string]any{"protocol": "ABC-123"}}}
	message, err := renderer.Render(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(message.Subject, "\r") || strings.Contains(message.Subject, "\n") || !strings.Contains(message.Subject, "ABC-123") {
		t.Fatalf("unsafe subject: %q", message.Subject)
	}
	if strings.Contains(message.HTML, "<script>") || !strings.Contains(message.HTML, "&lt;script&gt;") {
		t.Fatal("event message was not escaped")
	}
	if !strings.Contains(message.Text, "Ausente: ") || strings.Contains(message.Text, "metadata.ausente") {
		t.Fatal("missing metadata was not rendered empty")
	}
	if !strings.Contains(message.HTML, "/senders/worker?event_key=processing_completed") {
		t.Fatal("event log link missing")
	}
}
