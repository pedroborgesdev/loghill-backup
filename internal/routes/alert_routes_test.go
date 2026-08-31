package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"logtheater/internal/alerts"
	"logtheater/internal/config"
	"logtheater/internal/controllers"
	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
	"logtheater/internal/notification"
	"logtheater/internal/repositories"
	"logtheater/internal/scheduler"
	"logtheater/internal/services"
	settingsstore "logtheater/internal/settings"
)

type handlerEmailProvider struct{}

func (handlerEmailProvider) Provider() domain.EmailProviderType              { return domain.EmailProviderOutlook }
func (handlerEmailProvider) TestConnection(context.Context) error            { return nil }
func (handlerEmailProvider) Send(context.Context, domain.EmailMessage) error { return nil }

type alertHTTPFixture struct {
	router     http.Handler
	sender     domain.Sender
	dispatcher *notification.Dispatcher
}

func newAlertHTTPFixture(t *testing.T, admin bool) alertHTTPFixture {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, MaxBodySize: 1024 * 1024, MaxMessageSize: 1024, MaxMetadataSize: 1024, MaxPageSize: 100, MaxLogLines: 1000, SSEBuffer: 10, SSEMaxClients: 10, SSEHeartbeat: time.Second, AuthEnabled: admin, AppPassword: "admin-secret", EmailManagedByEnvironment: true, OutlookEnabled: true, OutlookTenantID: "tenant", OutlookClientID: "client", OutlookClientSecret: "secret", OutlookSenderEmail: "logs@example.com", OutlookSenderName: "LogHill", EmailAlertSendTimeout: time.Second}
	repo := repositories.New(dir)
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
	svc := services.New(repo, cfg, domain.SystemClock{}, settings)
	sender, err := svc.InitSender(context.Background(), "worker")
	if err != nil {
		t.Fatal(err)
	}
	alertService := alerts.NewService(alertStore, repo, emailSettings, domain.SystemClock{})
	provider := handlerEmailProvider{}
	dispatcher := notification.NewDispatcher(10, 1, 0, time.Second, 0, provider, notification.NewTemplate("http://localhost"), alertService)
	notificationService := services.NewNotificationService(alertService, nil, svc, emailSettings, provider, dispatcher, "http://localhost", time.Second)
	dispatcher.Start()
	svc.SetAlertSink(notification.NewRuntime(alertService, dispatcher))
	sched := scheduler.New(svc, time.Hour)
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok"), Mode: fs.FileMode(0644)}}
	controller := controllers.New(svc, cfg, sched, assets).ConfigureNotifications(alertService, notificationService)
	return alertHTTPFixture{router: New(controller, cfg).Router(), sender: sender, dispatcher: dispatcher}
}

func alertRequest(router http.Handler, method, path, body, key string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set("X-API-Key", key)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestAlertAndEmailSettingsEndpoints(t *testing.T) {
	fixture := newAlertHTTPFixture(t, false)
	defer fixture.dispatcher.Shutdown(context.Background())
	body := `{"name":"Erros críticos","sender_ids":["` + fixture.sender.ID + `"],"severities":["ERROR","FATAL"],"recipients":["dev@example.com"],"provider":"outlook","enabled":true}`
	response := alertRequest(fixture.router, http.MethodPost, "/api/v1/alerts", body, "")
	if response.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", response.Code, response.Body.String())
	}
	var created domain.EmailAlert
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodGet, "/api/v1/alerts", "", 200},
		{http.MethodGet, "/api/v1/alerts/" + created.ID, "", 200},
		{http.MethodPatch, "/api/v1/alerts/" + created.ID + "/status", `{"enabled":false}`, 200},
		{http.MethodPost, "/api/v1/alerts/" + created.ID + "/test", "", 202},
		{http.MethodGet, "/api/v1/settings/email", "", 200},
		{http.MethodPost, "/api/v1/settings/email/test-connection", "", 200},
		{http.MethodPost, "/api/v1/settings/email/send-test", `{"recipient":"dev@example.com"}`, 200},
	} {
		response = alertRequest(fixture.router, request.method, request.path, request.body, "")
		if response.Code != request.status {
			t.Fatalf("%s %s=%d %s", request.method, request.path, response.Code, response.Body.String())
		}
		if bytes.Contains(response.Body.Bytes(), []byte(`"client_secret":`)) || bytes.Contains(response.Body.Bytes(), []byte(`"access_token":`)) || bytes.Contains(response.Body.Bytes(), []byte(`"refresh_token":`)) {
			t.Fatalf("secret leaked from %s: %s", request.path, response.Body.String())
		}
	}
	response = alertRequest(fixture.router, http.MethodPost, "/api/v1/alerts", `{"name":"Gmail","sender_ids":["`+fixture.sender.ID+`"],"severities":["ERROR"],"recipients":["dev@example.com"],"provider":"gmail","enabled":false}`, "")
	if response.Code != http.StatusCreated {
		t.Fatalf("gmail=%d %s", response.Code, response.Body.String())
	}
	response = alertRequest(fixture.router, http.MethodDelete, "/api/v1/alerts/"+created.ID, "", "")
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete=%d", response.Code)
	}
}

func TestDeleteSenderReportsAndRemovesAlertDependencies(t *testing.T) {
	fixture := newAlertHTTPFixture(t, false)
	defer fixture.dispatcher.Shutdown(context.Background())
	body := `{"name":"Rule dependente","sender_ids":["` + fixture.sender.ID + `"],"severities":["ERROR"],"recipients":["dev@example.com"],"provider":"outlook","enabled":true}`
	createdResponse := alertRequest(fixture.router, http.MethodPost, "/api/v1/alerts", body, "")
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created domain.EmailAlert
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	blocked := alertRequest(fixture.router, http.MethodDelete, "/api/v1/senders/"+fixture.sender.ID, "", "")
	if blocked.Code != http.StatusConflict || !strings.Contains(blocked.Body.String(), "SENDER_HAS_ALERTS") || !strings.Contains(blocked.Body.String(), `"alert_rules":1`) {
		t.Fatalf("missing dependency conflict: %d %s", blocked.Code, blocked.Body.String())
	}
	removed := alertRequest(fixture.router, http.MethodDelete, "/api/v1/senders/"+fixture.sender.ID+"?remove_from_alerts=true", "", "")
	if removed.Code != http.StatusNoContent {
		t.Fatalf("delete=%d %s", removed.Code, removed.Body.String())
	}
	alertResponse := alertRequest(fixture.router, http.MethodGet, "/api/v1/alerts/"+created.ID, "", "")
	var updated domain.EmailAlert
	if err := json.Unmarshal(alertResponse.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Enabled || len(updated.SenderIDs) != 0 {
		t.Fatalf("dependent rule was not disabled: %+v", updated)
	}
}

func TestAlertEndpointsRequireAdminKey(t *testing.T) {
	fixture := newAlertHTTPFixture(t, true)
	defer fixture.dispatcher.Shutdown(context.Background())
	if response := alertRequest(fixture.router, http.MethodGet, "/api/v1/alerts", "", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
	if response := alertRequest(fixture.router, http.MethodGet, "/api/v1/alerts", "", "admin-secret"); response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
}
