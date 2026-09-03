package webhook

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"logtheater/internal/domain"
)

func TestValidateURLRejectsSSRFAndUnsafeForms(t *testing.T) {
	for _, value := range []string{
		"http://example.com/hook",
		"https://user:secret@example.com/hook",
		"https://127.0.0.1/hook",
		"https://10.0.0.1/hook",
		"https://169.254.169.254/latest/meta-data",
		"https://100.100.100.200/latest/meta-data",
		"https://[::1]/hook",
		"javascript:alert(1)",
	} {
		if err := ValidateURL(value); err == nil {
			t.Fatalf("unsafe URL accepted: %s", value)
		}
	}
	if err := ValidateURL("https://hooks.example.com/logmate"); err != nil {
		t.Fatalf("public HTTPS URL rejected: %v", err)
	}
	if !errors.Is(ValidateURL("https://127.0.0.1/hook"), ErrUnsafeTarget) {
		t.Fatal("private literal should return the unsafe-target error")
	}
}

func TestValidateRequestConfigSupportsStandardMethodsAndCredentials(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace} {
		config := domain.HTTPRequestConfig{Method: method, URL: "https://api.example.com/resource", Headers: map[string]string{"Authorization": "Bearer token", "X-Trace": "{{metadata.trace}}"}, Cookies: map[string]string{"session": "{{metadata.session}}"}, Body: `{"message":"{{log.message}}"}`}
		if err := ValidateRequestConfig(config); err != nil {
			t.Fatalf("method %s was rejected: %v", method, err)
		}
	}
}

func TestValidateRequestConfigRejectsUnsafeOrOversizedValues(t *testing.T) {
	tests := []domain.HTTPRequestConfig{
		{Method: "INVALID", URL: "https://example.com"},
		{Method: http.MethodPost, URL: "http://example.com"},
		{Method: http.MethodPost, URL: "https://127.0.0.1/private"},
		{Method: http.MethodPost, URL: "https://example.com", Headers: map[string]string{"Host": "other.example.com"}},
		{Method: http.MethodPost, URL: "https://example.com", Headers: map[string]string{"X-Test": "ok\r\ninjected"}},
		{Method: http.MethodPost, URL: "https://example.com", Headers: map[string]string{"Bad Header": "value"}},
		{Method: http.MethodPost, URL: "https://example.com", Cookies: map[string]string{"bad cookie": "value"}},
		{Method: http.MethodPost, URL: "https://example.com", Body: strings.Repeat("x", 64*1024+1)},
	}
	for index, config := range tests {
		if err := ValidateRequestConfig(config); err == nil {
			t.Fatalf("unsafe HTTP configuration %d was accepted", index)
		}
	}
}
