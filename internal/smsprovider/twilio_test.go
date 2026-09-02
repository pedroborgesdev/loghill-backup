package smsprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"logtheater/internal/domain"
)

type fakeRenderer struct{}

func (fakeRenderer) RenderEventText(_ domain.Notification, template string) string {
	return strings.ReplaceAll(template, "{{sender.name}}", "Worker A")
}

func TestTwilioSendsFormWithBasicAuthentication(t *testing.T) {
	var authorization, formValue string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		_ = r.ParseForm()
		formValue = r.Form.Get("To") + "|" + r.Form.Get("From") + "|" + r.Form.Get("Body")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	provider := NewTwilioWithEndpoint(true, "AC123", "secret", "+5511999999999", server.URL, server.Client(), fakeRenderer{})
	value := domain.Notification{Event: domain.EventDefinition{PhoneNumbers: []string{"+5511888888888"}, SMSTemplate: "Failure on {{sender.name}}"}}
	if err := provider.Send(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if authorization == "" || formValue != "+5511888888888|+5511999999999|Failure on Worker A" {
		t.Fatalf("unexpected request: auth=%q form=%q", authorization, formValue)
	}
}

func TestTwilioDoesNotLeakProviderResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":21211,"message":"invalid +5511888888888 token secret"}`))
	}))
	defer server.Close()
	provider := NewTwilioWithEndpoint(true, "AC123", "secret", "+5511999999999", server.URL, server.Client(), fakeRenderer{})
	err := provider.Send(context.Background(), domain.Notification{Event: domain.EventDefinition{PhoneNumbers: []string{"+5511888888888"}, SMSTemplate: "Failure"}})
	if err == nil || strings.Contains(err.Error(), "+5511888888888") || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "21211") {
		t.Fatalf("unsafe or unexpected error: %v", err)
	}
}
