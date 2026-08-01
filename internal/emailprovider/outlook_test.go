package emailprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"logtheater/internal/config"
	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
)

func testJWT(t *testing.T, roles ...string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"roles": roles})
	if err != nil {
		t.Fatal(err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func outlookSettings(t *testing.T) *emailconfig.Store {
	t.Helper()
	cfg := config.Config{EmailManagedByEnvironment: true, OutlookEnabled: true, OutlookTenantID: "tenant", OutlookClientID: "client", OutlookClientSecret: "secret", OutlookSenderEmail: "logs@example.com", OutlookSenderName: "LogHill"}
	store, err := emailconfig.Open(t.TempDir(), cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestOutlookAuthenticatesCachesTokenAndSendsMultipart(t *testing.T) {
	var tokenCalls, sendCalls atomic.Int32
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/oauth2/v2.0/token"):
			tokenCalls.Add(1)
			_ = request.ParseForm()
			if request.Form.Get("grant_type") != "client_credentials" || request.Form.Get("scope") != "https://graph.microsoft.com/.default" || request.Form.Get("client_secret") != "secret" {
				t.Errorf("unexpected token form: %v", request.Form)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"access_token":"token-value","expires_in":3600,"token_type":"Bearer"}`))
		case strings.HasSuffix(request.URL.Path, "/sendMail"):
			sendCalls.Add(1)
			if request.Header.Get("Authorization") != "Bearer token-value" || request.Header.Get("Content-Type") != "text/plain" {
				t.Errorf("unexpected send headers: %v", request.Header)
			}
			body, _ := io.ReadAll(request.Body)
			decoded, err := base64.StdEncoding.DecodeString(string(body))
			if err != nil {
				t.Error(err)
			}
			received = string(decoded)
			writer.WriteHeader(http.StatusAccepted)
		default:
			t.Errorf("unexpected path %s", request.URL.Path)
			writer.WriteHeader(404)
		}
	}))
	defer server.Close()
	provider := NewOutlookWithEndpoints(outlookSettings(t), server.Client(), server.URL, server.URL)
	message := domain.EmailMessage{To: []string{"dev@example.com"}, Subject: "[ERROR] worker — falha", Text: "texto simples", HTML: "<strong>html</strong>"}
	if err := provider.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if err := provider.Send(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if tokenCalls.Load() != 1 || sendCalls.Load() != 2 {
		t.Fatalf("token=%d sends=%d", tokenCalls.Load(), sendCalls.Load())
	}
	if !strings.Contains(received, "multipart/alternative") || !strings.Contains(received, base64.StdEncoding.EncodeToString([]byte(message.Text))) || !strings.Contains(received, base64.StdEncoding.EncodeToString([]byte(message.HTML))) {
		t.Fatalf("multipart alternatives missing:\n%s", received)
	}
}

func TestOutlookSanitizesAuthenticationFailureAndRejectsHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, `{"error":"secret details"}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	provider := NewOutlookWithEndpoints(outlookSettings(t), server.Client(), server.URL, server.URL)
	err := provider.TestConnection(context.Background())
	if err == nil || strings.Contains(err.Error(), "secret details") {
		t.Fatalf("unsafe error: %v", err)
	}
	runtime, _ := provider.settings.Runtime()
	if _, err = buildMIME(runtime, domain.EmailMessage{To: []string{"dev@example.com"}, Subject: "safe\r\nBcc: victim@example.com", Text: "x", HTML: "x"}); err == nil {
		t.Fatal("header injection should be rejected")
	}
	if _, err = url.Parse(provider.graphBase); err != nil {
		t.Fatal(err)
	}
}

func TestOutlookConnectionChecksMailSendApplicationPermission(t *testing.T) {
	tests := []struct {
		name      string
		roles     []string
		wantCode  string
		wantError bool
	}{
		{name: "permission present", roles: []string{"User.Read.All", "Mail.Send"}},
		{name: "permission missing", roles: []string{"User.Read.All"}, wantCode: "OUTLOOK_MAIL_SEND_PERMISSION_MISSING", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(writer).Encode(tokenValue{AccessToken: testJWT(t, test.roles...), ExpiresIn: 3600, TokenType: "Bearer"})
			}))
			defer server.Close()
			provider := NewOutlookWithEndpoints(outlookSettings(t), server.Client(), server.URL, server.URL)
			err := provider.TestConnection(context.Background())
			if !test.wantError {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var providerError *Error
			if !errors.As(err, &providerError) || providerError.Code != test.wantCode || !strings.Contains(providerError.Message, "Mail.Send") {
				t.Fatalf("unexpected permission error: %#v", err)
			}
		})
	}
}

func TestOutlookSendExplainsForbiddenPermission(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/oauth2/v2.0/token") {
			writer.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(writer).Encode(tokenValue{AccessToken: "opaque-token", ExpiresIn: 3600, TokenType: "Bearer"})
			return
		}
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	provider := NewOutlookWithEndpoints(outlookSettings(t), server.Client(), server.URL, server.URL)
	err := provider.Send(context.Background(), domain.EmailMessage{To: []string{"dev@example.com"}, Subject: "teste", Text: "texto", HTML: "<p>texto</p>"})
	var providerError *Error
	if !errors.As(err, &providerError) || providerError.Code != "OUTLOOK_SEND_FORBIDDEN" {
		t.Fatalf("unexpected error: %#v", err)
	}
	if !strings.Contains(providerError.Message, "Mail.Send") || !strings.Contains(providerError.Message, "escopo permitido") {
		t.Fatalf("error is not actionable: %q", providerError.Message)
	}
}
