package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"logtheater/internal/domain"
	"logtheater/internal/service"
)

func senderRequest(router http.Handler, method, path, body, adminKey, senderKey string) *httptest.ResponseRecorder {
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

func TestAdministrativeSenderCreationAndAuthenticatedIngestion(t *testing.T) {
	router := settingsRouter(t, true)
	if response := senderRequest(router, http.MethodPost, "/api/v1/senders/init", `{"name":"old"}`, "secret", ""); response.Code != http.StatusNotFound {
		t.Fatalf("legacy init route status=%d body=%s", response.Code, response.Body.String())
	}
	if response := senderRequest(router, http.MethodPost, "/api/v1/senders", `{"name":"Automação Financeira"}`, "", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unprotected admin creation status=%d", response.Code)
	}
	response := senderRequest(router, http.MethodPost, "/api/v1/senders", `{"name":"Automação Financeira","description":"Boletos"}`, "secret", "")
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var created struct {
		Sender      domain.Sender             `json:"sender"`
		Credentials service.SenderCredentials `json:"credentials"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Sender.ID != "automacao-financeira" || created.Sender.Status != domain.StatusNeverConnected || !created.Credentials.DisplayedOnce {
		t.Fatalf("unexpected creation response: %+v", created)
	}

	duplicate := senderRequest(router, http.MethodPost, "/api/v1/senders", `{"name":"Automação Financeira"}`, "secret", "")
	if duplicate.Code != http.StatusConflict || !strings.Contains(duplicate.Body.String(), `"field":"name"`) || !strings.Contains(duplicate.Body.String(), "SENDER_ALREADY_EXISTS") {
		t.Fatalf("duplicate status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}

	logBody := `{"sender":"automacao-financeira","severity":"INFO","message":"teste"}`
	for name, key := range map[string]string{"missing": "", "wrong": "snd_wrong"} {
		unauthorized := senderRequest(router, http.MethodPost, "/api/v1/logs", logBody, "", key)
		if unauthorized.Code != http.StatusUnauthorized || !strings.Contains(unauthorized.Body.String(), "INVALID_SENDER_KEY") {
			t.Fatalf("%s key status=%d body=%s", name, unauthorized.Code, unauthorized.Body.String())
		}
	}
	if accepted := senderRequest(router, http.MethodPost, "/api/v1/logs", logBody, "", created.Credentials.SenderKey); accepted.Code != http.StatusAccepted {
		t.Fatalf("valid log status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	if health := senderRequest(router, http.MethodPost, "/api/v1/senders/automacao-financeira/health", `{}`, "", created.Credentials.SenderKey); health.Code != http.StatusOK {
		t.Fatalf("valid health status=%d body=%s", health.Code, health.Body.String())
	}

	listing := senderRequest(router, http.MethodGet, "/api/v1/senders?page=1&page_size=20", "", "secret", "")
	if strings.Contains(listing.Body.String(), created.Credentials.SenderKey) || strings.Contains(listing.Body.String(), "key_hash") {
		t.Fatalf("listing exposed secret material: %s", listing.Body.String())
	}
}

func TestRotateRevokeAndReactivateSenderEndpoints(t *testing.T) {
	router := settingsRouter(t, false)
	createdResponse := senderRequest(router, http.MethodPost, "/api/v1/senders", `{"name":"Worker Seguro"}`, "", "")
	var created struct {
		Sender      domain.Sender             `json:"sender"`
		Credentials service.SenderCredentials `json:"credentials"`
	}
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rotatedResponse := senderRequest(router, http.MethodPost, "/api/v1/senders/worker-seguro/rotate-key", "", "", "")
	if rotatedResponse.Code != http.StatusOK {
		t.Fatalf("rotate status=%d body=%s", rotatedResponse.Code, rotatedResponse.Body.String())
	}
	var rotated struct {
		Credentials service.SenderCredentials `json:"credentials"`
	}
	if err := json.Unmarshal(rotatedResponse.Body.Bytes(), &rotated); err != nil {
		t.Fatal(err)
	}
	if old := senderRequest(router, http.MethodPost, "/api/v1/senders/worker-seguro/health", `{}`, "", created.Credentials.SenderKey); old.Code != http.StatusUnauthorized {
		t.Fatalf("old key status=%d", old.Code)
	}
	if revoke := senderRequest(router, http.MethodPost, "/api/v1/senders/worker-seguro/revoke", "", "", ""); revoke.Code != http.StatusOK {
		t.Fatalf("revoke status=%d", revoke.Code)
	}
	if revoked := senderRequest(router, http.MethodPost, "/api/v1/senders/worker-seguro/health", `{}`, "", rotated.Credentials.SenderKey); revoked.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key status=%d", revoked.Code)
	}
	if reactivate := senderRequest(router, http.MethodPost, "/api/v1/senders/worker-seguro/reactivate", "", "", ""); reactivate.Code != http.StatusOK || !strings.Contains(reactivate.Body.String(), `"sender_key":"snd_`) {
		t.Fatalf("reactivate status=%d body=%s", reactivate.Code, reactivate.Body.String())
	}
}

func TestSenderInstancesSeparateLogsWithoutCreatingNewSenders(t *testing.T) {
	router := settingsRouter(t, false)
	createdResponse := senderRequest(router, http.MethodPost, "/api/v1/senders", `{"name":"Worker Paralelo"}`, "", "")
	var created struct {
		Credentials service.SenderCredentials `json:"credentials"`
	}
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	initInstance := func() string {
		response := senderRequest(router, http.MethodPost, "/api/v1/senders/worker-paralelo/instances/init", `{}`, "", created.Credentials.SenderKey)
		if response.Code != http.StatusCreated {
			t.Fatalf("instance init status=%d body=%s", response.Code, response.Body.String())
		}
		var value struct {
			InstanceID string `json:"instance_id"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value.InstanceID
	}
	first, second := initInstance(), initInstance()
	if first == second || !strings.HasPrefix(first, "ins_") || !strings.HasPrefix(second, "ins_") {
		t.Fatalf("instance IDs are not unique: %q %q", first, second)
	}
	keyOnly := senderRequest(router, http.MethodPost, "/api/v1/instances/init", `{}`, "", created.Credentials.SenderKey)
	if keyOnly.Code != http.StatusCreated || !strings.Contains(keyOnly.Body.String(), `"sender":"worker-paralelo"`) || !strings.Contains(keyOnly.Body.String(), `"instance_id":"ins_`) {
		t.Fatalf("key-only init status=%d body=%s", keyOnly.Code, keyOnly.Body.String())
	}
	postLog := func(instanceID, message string) {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bytes.NewBufferString(`{"sender":"worker-paralelo","severity":"INFO","message":"`+message+`"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Sender-Key", created.Credentials.SenderKey)
		request.Header.Set("X-Sender-Instance-ID", instanceID)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("log status=%d body=%s", response.Code, response.Body.String())
		}
	}
	postLog(first, "primeira")
	postLog(second, "segunda")
	firstLogs := senderRequest(router, http.MethodGet, "/api/v1/senders/worker-paralelo/logs?instance_id="+first, "", "", "")
	if !strings.Contains(firstLogs.Body.String(), "primeira") || strings.Contains(firstLogs.Body.String(), "segunda") {
		t.Fatalf("first instance mixed logs: %s", firstLogs.Body.String())
	}
	instances := senderRequest(router, http.MethodGet, "/api/v1/senders/worker-paralelo/instances", "", "", "")
	if instances.Code != http.StatusOK || !strings.Contains(instances.Body.String(), first) || !strings.Contains(instances.Body.String(), second) {
		t.Fatalf("instances status=%d body=%s", instances.Code, instances.Body.String())
	}
}
