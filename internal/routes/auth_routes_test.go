package routes

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"logtheater/internal/auth"
	"logtheater/internal/config"
	"logtheater/internal/controllers"
	"logtheater/internal/domain"
	"logtheater/internal/repositories"
	"logtheater/internal/scheduler"
	"logtheater/internal/services"
	settingsstore "logtheater/internal/settings"
)

func TestAuthLoginSessionAndLogout(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Config{
		DataDir: dataDir, MaxBodySize: 1024 * 1024, MaxMessageSize: 1024, MaxMetadataSize: 1024,
		MaxPageSize: 100, MaxLogLines: 1000, SSEBuffer: 10, SSEMaxClients: 10, SSEHeartbeat: time.Second,
		AuthEnabled: true, AppPassword: "s3cret",
	}
	repo := repositories.New(dataDir)
	if err := repo.Init(); err != nil {
		t.Fatal(err)
	}
	store, err := settingsstore.Open(dataDir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc := services.New(repo, cfg, domain.SystemClock{}, store)
	sched := scheduler.New(svc, time.Hour)
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok"), Mode: fs.FileMode(0644)}}
	sessions := auth.NewManager(cfg.AppPassword, time.Hour, false)
	controller := controllers.New(svc, cfg, sched, assets).ConfigureAuth(sessions)
	router := New(controller, cfg).Router()

	unauthorized := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	unauthorizedRecorder := httptest.NewRecorder()
	router.ServeHTTP(unauthorizedRecorder, unauthorized)
	if unauthorizedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", unauthorizedRecorder.Code)
	}

	body, _ := json.Marshal(map[string]string{"password": "s3cret"})
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	login.Header.Set("Content-Type", "application/json")
	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, login)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginRecorder.Code, loginRecorder.Body.String())
	}
	cookie := loginRecorder.Result().Cookies()
	if len(cookie) == 0 {
		t.Fatal("expected session cookie")
	}

	authorized := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	authorized.AddCookie(cookie[0])
	authorizedRecorder := httptest.NewRecorder()
	router.ServeHTTP(authorizedRecorder, authorized)
	if authorizedRecorder.Code != http.StatusOK {
		t.Fatalf("expected authorized settings, got %d", authorizedRecorder.Code)
	}

	logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logout.AddCookie(cookie[0])
	logoutRecorder := httptest.NewRecorder()
	router.ServeHTTP(logoutRecorder, logout)
	if logoutRecorder.Code != http.StatusOK {
		t.Fatalf("logout failed: %d", logoutRecorder.Code)
	}
}
