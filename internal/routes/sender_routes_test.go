package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"logtheater/internal/domain"
	"logtheater/internal/services"
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
	if response := senderRequest(router, http.MethodPost, "/api/v1/senders", `{"name":"Financial Automation"}`, "", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("unprotected admin creation status=%d", response.Code)
	}
	response := senderRequest(router, http.MethodPost, "/api/v1/senders", `{"name":"Financial Automation","description":"Boletos"}`, "secret", "")
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var created struct {
		Sender      domain.Sender              `json:"sender"`
		Credentials services.SenderCredentials `json:"credentials"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Sender.ID != "financial-automation" || created.Sender.Status != domain.StatusNeverConnected || !created.Credentials.DisplayedOnce {
		t.Fatalf("unexpected creation response: %+v", created)
	}

	duplicate := senderRequest(router, http.MethodPost, "/api/v1/senders", `{"name":"Financial Automation"}`, "secret", "")
	if duplicate.Code != http.StatusConflict || !strings.Contains(duplicate.Body.String(), `"field":"name"`) || !strings.Contains(duplicate.Body.String(), "SENDER_ALREADY_EXISTS") {
		t.Fatalf("duplicate status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}

	logBody := `{"sender":"financial-automation","severity":"INFO","message":"teste"}`
	for name, key := range map[string]string{"missing": "", "wrong": "snd_wrong"} {
		unauthorized := senderRequest(router, http.MethodPost, "/api/v1/logs", logBody, "", key)
		if unauthorized.Code != http.StatusUnauthorized || !strings.Contains(unauthorized.Body.String(), "INVALID_SENDER_KEY") {
			t.Fatalf("%s key status=%d body=%s", name, unauthorized.Code, unauthorized.Body.String())
		}
	}
	if accepted := senderRequest(router, http.MethodPost, "/api/v1/logs", logBody, "", created.Credentials.SenderKey); accepted.Code != http.StatusAccepted {
		t.Fatalf("valid log status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	undefinedLog := `{"sender":"financial-automation","severity":"UNDEFINED","message":"INFO: Uvicorn running"}`
	if accepted := senderRequest(router, http.MethodPost, "/api/v1/logs", undefinedLog, "", created.Credentials.SenderKey); accepted.Code != http.StatusAccepted {
		t.Fatalf("undefined log status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	undefinedLogs := senderRequest(router, http.MethodGet, "/api/v1/senders/financial-automation/logs?severity=UNDEFINED", "", "secret", "")
	if undefinedLogs.Code != http.StatusOK || !strings.Contains(undefinedLogs.Body.String(), "Uvicorn running") {
		t.Fatalf("undefined log listing status=%d body=%s", undefinedLogs.Code, undefinedLogs.Body.String())
	}
	if health := senderRequest(router, http.MethodPost, "/api/v1/senders/financial-automation/health", `{}`, "", created.Credentials.SenderKey); health.Code != http.StatusOK {
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
		Sender      domain.Sender              `json:"sender"`
		Credentials services.SenderCredentials `json:"credentials"`
	}
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rotatedResponse := senderRequest(router, http.MethodPost, "/api/v1/senders/worker-seguro/rotate-key", "", "", "")
	if rotatedResponse.Code != http.StatusOK {
		t.Fatalf("rotate status=%d body=%s", rotatedResponse.Code, rotatedResponse.Body.String())
	}
	var rotated struct {
		Credentials services.SenderCredentials `json:"credentials"`
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
		Credentials services.SenderCredentials `json:"credentials"`
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
	nameOnly := senderRequest(router, http.MethodPost, "/api/v1/instances/init", `{"sender_name":"Worker Paralelo"}`, "", "")
	if nameOnly.Code != http.StatusCreated {
		t.Fatalf("name-only init status=%d body=%s", nameOnly.Code, nameOnly.Body.String())
	}
	var nameInstance struct {
		Sender        string `json:"sender"`
		SenderID      string `json:"sender_id"`
		InstanceID    string `json:"instance_id"`
		InstanceToken string `json:"instance_token"`
	}
	if err := json.Unmarshal(nameOnly.Body.Bytes(), &nameInstance); err != nil {
		t.Fatal(err)
	}
	if nameInstance.SenderID != "worker-paralelo" || nameInstance.Sender != nameInstance.SenderID || !strings.HasPrefix(nameInstance.InstanceID, "ins_") || !strings.HasPrefix(nameInstance.InstanceToken, "inst_") {
		t.Fatalf("unexpected name-only credentials: %+v", nameInstance)
	}
	postWithInstanceToken := func(token, message string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bytes.NewBufferString(`{"sender_id":"worker-paralelo","severity":"INFO","message":"`+message+`"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Sender-Instance-ID", nameInstance.InstanceID)
		request.Header.Set("X-Sender-Instance-Token", token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	if rejected := postWithInstanceToken("inst_wrong", "rejeitado"); rejected.Code != http.StatusUnauthorized || !strings.Contains(rejected.Body.String(), "INVALID_INSTANCE_TOKEN") {
		t.Fatalf("wrong instance token status=%d body=%s", rejected.Code, rejected.Body.String())
	}
	if accepted := postWithInstanceToken(nameInstance.InstanceToken, "por nome"); accepted.Code != http.StatusAccepted {
		t.Fatalf("instance-token log status=%d body=%s", accepted.Code, accepted.Body.String())
	} else if !strings.Contains(accepted.Body.String(), `"sender_id":"worker-paralelo"`) {
		t.Fatalf("log response did not return sender_id: %s", accepted.Body.String())
	}
	newExecutionResponse := senderRequest(router, http.MethodPost, "/api/v1/instances/init", `{"sender_name":"Worker Paralelo"}`, "", "")
	if newExecutionResponse.Code != http.StatusCreated {
		t.Fatalf("new execution init status=%d body=%s", newExecutionResponse.Code, newExecutionResponse.Body.String())
	}
	var newExecution struct {
		InstanceID    string `json:"instance_id"`
		InstanceToken string `json:"instance_token"`
	}
	if err := json.Unmarshal(newExecutionResponse.Body.Bytes(), &newExecution); err != nil {
		t.Fatal(err)
	}
	replayedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bytes.NewBufferString(`{"sender_id":"worker-paralelo","severity":"UNDEFINED","message":"encerramento pendente"}`))
	replayedRequest.Header.Set("Content-Type", "application/json")
	replayedRequest.Header.Set("X-Sender-Instance-ID", newExecution.InstanceID)
	replayedRequest.Header.Set("X-Sender-Instance-Token", newExecution.InstanceToken)
	replayedRequest.Header.Set("X-LogHill-Origin-Instance-ID", nameInstance.InstanceID)
	replayedResponse := httptest.NewRecorder()
	router.ServeHTTP(replayedResponse, replayedRequest)
	if replayedResponse.Code != http.StatusAccepted || !strings.Contains(replayedResponse.Body.String(), `"instance_id":"`+nameInstance.InstanceID+`"`) {
		t.Fatalf("replayed log status=%d body=%s", replayedResponse.Code, replayedResponse.Body.String())
	}
	originLogs := senderRequest(router, http.MethodGet, "/api/v1/senders/worker-paralelo/logs?instance_id="+nameInstance.InstanceID, "", "", "")
	currentLogs := senderRequest(router, http.MethodGet, "/api/v1/senders/worker-paralelo/logs?instance_id="+newExecution.InstanceID, "", "", "")
	if !strings.Contains(originLogs.Body.String(), "encerramento pendente") || strings.Contains(currentLogs.Body.String(), "encerramento pendente") {
		t.Fatalf("replayed log was not kept in its origin instance: origin=%s current=%s", originLogs.Body.String(), currentLogs.Body.String())
	}
	invalidOriginRequest := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bytes.NewBufferString(`{"sender_id":"worker-paralelo","severity":"INFO","message":"origem inválida"}`))
	invalidOriginRequest.Header.Set("Content-Type", "application/json")
	invalidOriginRequest.Header.Set("X-Sender-Instance-ID", newExecution.InstanceID)
	invalidOriginRequest.Header.Set("X-Sender-Instance-Token", newExecution.InstanceToken)
	invalidOriginRequest.Header.Set("X-LogHill-Origin-Instance-ID", "ins_00000000000000000000000000000000")
	invalidOriginResponse := httptest.NewRecorder()
	router.ServeHTTP(invalidOriginResponse, invalidOriginRequest)
	if invalidOriginResponse.Code != http.StatusConflict {
		t.Fatalf("invalid origin status=%d body=%s", invalidOriginResponse.Code, invalidOriginResponse.Body.String())
	}
	healthRequest := httptest.NewRequest(http.MethodPost, "/api/v1/senders/worker-paralelo/health", bytes.NewBufferString(`{}`))
	healthRequest.Header.Set("Content-Type", "application/json")
	healthRequest.Header.Set("X-Sender-Instance-ID", nameInstance.InstanceID)
	healthRequest.Header.Set("X-Sender-Instance-Token", nameInstance.InstanceToken)
	healthResponse := httptest.NewRecorder()
	router.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("instance-token health status=%d body=%s", healthResponse.Code, healthResponse.Body.String())
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
	if strings.Contains(instances.Body.String(), nameInstance.InstanceToken) || strings.Contains(instances.Body.String(), "token_hash") {
		t.Fatalf("instances listing exposed secret material: %s", instances.Body.String())
	}
}

func TestInitInstanceByNameCreatesMissingSender(t *testing.T) {
	router := settingsRouter(t, false)

	firstResponse := senderRequest(router, http.MethodPost, "/api/v1/instances/init", `{"sender_name":"Worker Automático"}`, "", "")
	if firstResponse.Code != http.StatusCreated {
		t.Fatalf("first init status=%d body=%s", firstResponse.Code, firstResponse.Body.String())
	}
	var first struct {
		SenderID      string `json:"sender_id"`
		InstanceID    string `json:"instance_id"`
		InstanceToken string `json:"instance_token"`
		SenderKey     string `json:"sender_key"`
	}
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.SenderID != "worker-automatico" || !strings.HasPrefix(first.InstanceID, "ins_") || !strings.HasPrefix(first.InstanceToken, "inst_") {
		t.Fatalf("unexpected automatic sender credentials: %+v", first)
	}
	if first.SenderKey != "" || strings.Contains(firstResponse.Body.String(), "sender_key") {
		t.Fatalf("automatic initialization exposed sender key: %s", firstResponse.Body.String())
	}

	secondResponse := senderRequest(router, http.MethodPost, "/api/v1/instances/init", `{"sender_name":"Worker Automático"}`, "", "")
	if secondResponse.Code != http.StatusCreated {
		t.Fatalf("second init status=%d body=%s", secondResponse.Code, secondResponse.Body.String())
	}
	var second struct {
		SenderID   string `json:"sender_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.Unmarshal(secondResponse.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second.SenderID != first.SenderID || second.InstanceID == first.InstanceID {
		t.Fatalf("second initialization did not reuse sender with a new instance: first=%+v second=%+v", first, second)
	}

	listing := senderRequest(router, http.MethodGet, "/api/v1/senders?page=1&page_size=100", "", "", "")
	if listing.Code != http.StatusOK || strings.Count(listing.Body.String(), `"id":"worker-automatico"`) != 1 {
		t.Fatalf("automatic sender was not created exactly once: status=%d body=%s", listing.Code, listing.Body.String())
	}
}
