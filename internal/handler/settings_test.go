package handler

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"logtheater/internal/config"
	"logtheater/internal/domain"
	"logtheater/internal/repository"
	"logtheater/internal/scheduler"
	"logtheater/internal/service"
	settingsstore "logtheater/internal/settings"
)

func settingsRouter(t *testing.T, adminEnabled bool) http.Handler {
	t.Helper()
	dataDir := t.TempDir()
	cfg := config.Config{
		DataDir:          dataDir,
		MaxBodySize:      1024 * 1024,
		MaxMessageSize:   1024,
		MaxMetadataSize:  1024,
		MaxPageSize:      100,
		MaxLogLines:      100_000,
		SSEBuffer:        10,
		SSEMaxClients:    10,
		SSEHeartbeat:     time.Second,
		AuthEnabled: adminEnabled,
		AppPassword: "secret",
	}
	repo := repository.New(dataDir)
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	store, err := settingsstore.Open(dataDir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(repo, cfg, domain.SystemClock{}, store)
	sched := scheduler.New(svc, time.Hour)
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok"), Mode: fs.FileMode(0644)}}
	return New(svc, cfg, sched, assets).Router()
}

func settingsRequest(router http.Handler, method, body, key string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/api/v1/settings", bytes.NewBufferString(body))
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

func TestSettingsEndpointsLoadAndUpdateWithoutRestart(t *testing.T) {
	router := settingsRouter(t, false)
	response := settingsRequest(router, http.MethodGet, "", "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", response.Code, response.Body.String())
	}
	var initial domain.Settings
	if err := json.Unmarshal(response.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.LogLimit.Value != 10_000 || initial.InactivePreservation.Value != 2_000 {
		t.Fatalf("unexpected defaults: %+v", initial)
	}

	body := `{"log_limit":{"value":500,"unit":"mb"},"inactive_preservation":{"value":2000,"unit":"lines"}}`
	response = settingsRequest(router, http.MethodPut, body, "")
	if response.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", response.Code, response.Body.String())
	}
	response = settingsRequest(router, http.MethodGet, "", "")
	var updated domain.Settings
	if err := json.Unmarshal(response.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.LogLimit.Value != 500 || updated.LogLimit.Unit != domain.StorageMB {
		t.Fatalf("updated settings were not immediately visible: %+v", updated)
	}
}

func TestSettingsEndpointRejectsInvalidBodies(t *testing.T) {
	router := settingsRouter(t, false)
	tests := []struct {
		name   string
		body   string
		status int
	}{
		{name: "decimal", body: `{"log_limit":{"value":1.5,"unit":"lines"},"inactive_preservation":{"value":1,"unit":"lines"}}`, status: 400},
		{name: "unknown", body: `{"log_limit":{"value":10,"unit":"lines"},"inactive_preservation":{"value":1,"unit":"lines"},"extra":true}`, status: 400},
		{name: "incomplete", body: `{"log_limit":{"value":10,"unit":"lines"}}`, status: 400},
		{name: "negative", body: `{"log_limit":{"value":-1,"unit":"lines"},"inactive_preservation":{"value":0,"unit":"lines"}}`, status: 422},
		{name: "above maximum", body: `{"log_limit":{"value":10001,"unit":"lines"},"inactive_preservation":{"value":1,"unit":"lines"}}`, status: 422},
		{name: "invalid unit", body: `{"log_limit":{"value":10,"unit":"gb"},"inactive_preservation":{"value":1,"unit":"lines"}}`, status: 422},
		{name: "preservation over maximum", body: `{"log_limit":{"value":10,"unit":"lines"},"inactive_preservation":{"value":11,"unit":"lines"}}`, status: 422},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := settingsRequest(router, http.MethodPut, test.body, "")
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var body map[string]map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body["error"]["code"] != "INVALID_SETTINGS" {
				t.Fatalf("unexpected error body: %s, %v", response.Body.String(), err)
			}
		})
	}
}

func TestSettingsEndpointsRequireAdminAuthorization(t *testing.T) {
	router := settingsRouter(t, true)
	if response := settingsRequest(router, http.MethodGet, "", "wrong"); response.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", response.Code)
	}
	if response := settingsRequest(router, http.MethodGet, "", "secret"); response.Code != http.StatusOK {
		t.Fatalf("expected authorized request, got %d", response.Code)
	}
}
