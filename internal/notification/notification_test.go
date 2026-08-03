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
	dispatcher := NewDispatcher(2, 1, 2, time.Second, time.Millisecond, provider, fakeRenderer{}, recorder)
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
	dispatcher := NewDispatcher(1, 1, 1, 10*time.Millisecond, 0, provider, fakeRenderer{}, recorder)
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
	if strings.Contains(message.Subject, "[") || !strings.HasPrefix(message.Subject, "LogHill — Erro detectado em worker:") {
		t.Fatalf("subject is not readable: %q", message.Subject)
	}
	for _, expected := range []string{`align="center"`, "Central de observabilidade", "Um erro foi detectado", "#f59e0b", "/loghill.png"} {
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
	if message.Subject != "Teste de e-mail do LogHill — configuração concluída" || strings.Contains(message.Subject, "[") {
		t.Fatalf("unexpected subject: %q", message.Subject)
	}
	if len(message.To) != 1 || message.To[0] != "dev@example.com" {
		t.Fatalf("unexpected recipients: %v", message.To)
	}
	for _, expected := range []string{"Tudo certo com o seu e-mail", "Microsoft 365 / Outlook", "Configuração validada", "Abrir o LogHill", "https://logs.example.com/loghill.png"} {
		if !strings.Contains(message.HTML, expected) {
			t.Fatalf("provider test email is missing %q", expected)
		}
	}
	if !strings.Contains(message.Text, "A integração do LogHill com o Outlook está funcionando") {
		t.Fatal("provider test plain-text fallback is incomplete")
	}
}

func TestTemplateRendersGmailProviderTestWithoutOutlookReferences(t *testing.T) {
	renderer := NewTemplate("https://logs.example.com")
	message, err := renderer.RenderProviderTest("dev@example.com", domain.EmailProviderGmail)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Gmail", "SMTP com STARTTLS", "Gmail via SMTP"} {
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
	value := domain.Notification{SourceType: domain.NotificationSourceEvent, Event: domain.EventDefinition{ID: "evt_1", Name: "Processamento finalizado", Key: "processamento_finalizado", Recipients: []string{"dev@example.com"}, SubjectTemplate: "Finalizado — {{metadata.protocolo}} — {{sender.name}}\r\nInjected", MessageTemplate: "{{log.message}}\nAusente: {{metadata.ausente}}"}, Sender: domain.Sender{ID: "worker", Name: "Worker", Status: domain.StatusOnline}, Entry: domain.LogEntry{Timestamp: time.Now(), Severity: domain.Info, Message: "<script>alert('x')</script>", Event: "processamento_finalizado", Metadata: map[string]any{"protocolo": "ABC-123"}}}
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
	if !strings.Contains(message.HTML, "/senders/worker?event_key=processamento_finalizado") {
		t.Fatal("event log link missing")
	}
}
