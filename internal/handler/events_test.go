package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"logtheater/internal/alerts"
	"logtheater/internal/config"
	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
	"logtheater/internal/events"
	"logtheater/internal/notification"
	"logtheater/internal/repository"
	"logtheater/internal/scheduler"
	"logtheater/internal/service"
	settingsstore "logtheater/internal/settings"
)

type eventHTTPFixture struct {
	router     http.Handler
	sender     domain.Sender
	senderKey  string
	events     *events.Service
	dispatcher *notification.Dispatcher
}

func newEventHTTPFixture(t *testing.T, admin bool) eventHTTPFixture {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, MaxBodySize: 1024 * 1024, MaxMessageSize: 1024, MaxMetadataSize: 1024, MaxPageSize: 100, MaxLogLines: 1000, SSEBuffer: 10, SSEMaxClients: 10, SSEHeartbeat: time.Second, AdminAuthEnabled: admin, AdminAPIKey: "admin-secret", EmailManagedByEnvironment: true, OutlookEnabled: true, OutlookTenantID: "tenant", OutlookClientID: "client", OutlookClientSecret: "secret", OutlookSenderEmail: "logs@example.com", OutlookSenderName: "LogHill", EmailAlertSendTimeout: time.Second}
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
	eventStore, err := events.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(repo, cfg, domain.SystemClock{}, settings)
	sender, credentials, err := svc.CreateSender(context.Background(), "Worker Event", "")
	if err != nil {
		t.Fatal(err)
	}
	alertService := alerts.NewService(alertStore, repo, emailSettings, domain.SystemClock{})
	eventService := events.NewService(eventStore, repo, emailSettings, domain.SystemClock{})
	provider := handlerEmailProvider{}
	dispatcher := notification.NewDispatcher(10, 1, 0, time.Second, 0, provider, notification.NewTemplate("http://localhost"), notification.NewRecorder(alertService, eventService))
	dispatcher.Start()
	svc.SetAlertSink(notification.NewRuntime(alertService, dispatcher))
	svc.SetEventSink(notification.NewEventRuntime(eventService, dispatcher))
	sched := scheduler.New(svc, time.Hour)
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok"), Mode: fs.FileMode(0644)}}
	api := New(svc, cfg, sched, assets).ConfigureNotifications(alertService, emailSettings, provider, dispatcher).ConfigureEvents(eventService)
	return eventHTTPFixture{router: api.Router(), sender: sender, senderKey: credentials.SenderKey, events: eventService, dispatcher: dispatcher}
}

func eventRequest(router http.Handler, method, path, body, adminKey, senderKey string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if adminKey != "" {
		request.Header.Set("X-API-Key", adminKey)
	}
	if senderKey != "" {
		request.Header.Set("X-Sender-Key", senderKey)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestEventEndpointsAndLogIngestion(t *testing.T) {
	fixture := newEventHTTPFixture(t, false)
	defer fixture.dispatcher.Shutdown(context.Background())
	body := `{"name":"Processamento finalizado","key":"processamento_finalizado","sender_ids":["` + fixture.sender.ID + `"],"action_type":"email","recipients":["dev@example.com"],"subject_template":"Finalizado — {{sender.name}}","message_template":"Protocolo: {{metadata.protocolo}}\n{{log.message}}","enabled":true}`
	response := eventRequest(fixture.router, http.MethodPost, "/api/v1/events", body, "", "")
	if response.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", response.Code, response.Body.String())
	}
	var created domain.EventDefinition
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if response = eventRequest(fixture.router, http.MethodPost, "/api/v1/events", body, "", ""); response.Code != http.StatusConflict {
		t.Fatalf("duplicate=%d %s", response.Code, response.Body.String())
	}
	if response = eventRequest(fixture.router, http.MethodGet, "/api/v1/events/check-key?key=processamento_finalizado", "", "", ""); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"available":false`)) {
		t.Fatalf("check=%d %s", response.Code, response.Body.String())
	}
	updatedBody := `{"name":"Outro nome","key":"outra_chave","sender_ids":["` + fixture.sender.ID + `"],"action_type":"email","recipients":["dev@example.com"],"subject_template":"Finalizado","message_template":"Mensagem","enabled":true}`
	if response = eventRequest(fixture.router, http.MethodPut, "/api/v1/events/"+created.ID, updatedBody, "", ""); response.Code != http.StatusConflict {
		t.Fatalf("immutable=%d %s", response.Code, response.Body.String())
	}
	validLog := `{"sender":"` + fixture.sender.ID + `","severity":"INFO","message":"concluído","event":"processamento_finalizado","metadata":{"protocolo":"ABC-123"}}`
	if response = eventRequest(fixture.router, http.MethodPost, "/api/v1/logs", validLog, "", fixture.senderKey); response.Code != http.StatusAccepted {
		t.Fatalf("log=%d %s", response.Code, response.Body.String())
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, _ := fixture.events.Get(created.ID)
		if current.TriggerCount == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	current, _ := fixture.events.Get(created.ID)
	if current.TriggerCount != 1 {
		t.Fatalf("event was not dispatched: %+v", current)
	}
	unknownLog := `{"sender":"` + fixture.sender.ID + `","severity":"ERROR","message":"continua válido","event":"evento_desconhecido"}`
	if response = eventRequest(fixture.router, http.MethodPost, "/api/v1/logs", unknownLog, "", fixture.senderKey); response.Code != http.StatusAccepted {
		t.Fatalf("unknown=%d %s", response.Code, response.Body.String())
	}
	invalidLog := `{"sender":"` + fixture.sender.ID + `","severity":"INFO","message":"inválido","event":"../evento"}`
	if response = eventRequest(fixture.router, http.MethodPost, "/api/v1/logs", invalidLog, "", fixture.senderKey); response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(`"field":"event"`)) {
		t.Fatalf("invalid=%d %s", response.Code, response.Body.String())
	}
	if response = eventRequest(fixture.router, http.MethodPost, "/api/v1/events/"+created.ID+"/test", `{"recipient":"qa@example.com"}`, "", ""); response.Code != http.StatusAccepted {
		t.Fatalf("test=%d %s", response.Code, response.Body.String())
	}
	if response = eventRequest(fixture.router, http.MethodDelete, "/api/v1/events/"+created.ID, "", "", ""); response.Code != http.StatusNoContent {
		t.Fatalf("delete=%d %s", response.Code, response.Body.String())
	}
}

func TestEventEndpointsRequireAdminAuthentication(t *testing.T) {
	fixture := newEventHTTPFixture(t, true)
	defer fixture.dispatcher.Shutdown(context.Background())
	if response := eventRequest(fixture.router, http.MethodGet, "/api/v1/events", "", "", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("expected auth, got %d", response.Code)
	}
	if response := eventRequest(fixture.router, http.MethodGet, "/api/v1/events", "", "admin-secret", ""); response.Code != http.StatusOK {
		t.Fatalf("admin=%d %s", response.Code, response.Body.String())
	}
}
