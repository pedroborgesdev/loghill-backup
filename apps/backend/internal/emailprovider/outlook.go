package emailprovider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/mail"
	"net/textproto"
	"net/url"
	"strings"
	"sync"
	"time"

	"logmate/internal/domain"
	"logmate/internal/emailconfig"
	"logmate/internal/validation"
)

var ErrNotConfigured = errors.New("outlook is not configured")

type Provider interface {
	Send(context.Context, domain.EmailMessage) error
	TestConnection(context.Context) error
	Provider() domain.EmailProviderType
}

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

type tokenValue struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type Outlook struct {
	settings  *emailconfig.Store
	client    *http.Client
	tokenBase string
	graphBase string
	mu        sync.Mutex
	token     string
	expiresAt time.Time
	signature string
}

func NewOutlook(settings *emailconfig.Store, timeout time.Duration) *Outlook {
	return NewOutlookWithEndpoints(settings, &http.Client{Timeout: timeout}, "https://login.microsoftonline.com", "https://graph.microsoft.com")
}

func NewOutlookWithEndpoints(settings *emailconfig.Store, client *http.Client, tokenBase, graphBase string) *Outlook {
	return &Outlook{settings: settings, client: client, tokenBase: strings.TrimRight(tokenBase, "/"), graphBase: strings.TrimRight(graphBase, "/")}
}

func (o *Outlook) Provider() domain.EmailProviderType { return domain.EmailProviderOutlook }

func (o *Outlook) TestConnection(ctx context.Context) error {
	runtime, err := o.settings.Runtime()
	if err != nil {
		return &Error{Code: "OUTLOOK_CONFIGURATION_ERROR", Message: "Unable to read the secure Outlook configuration."}
	}
	if !complete(runtime) {
		return ErrNotConfigured
	}
	// A manual test must reflect permission changes made in Microsoft Entra,
	// rather than reusing a token issued before admin consent was granted.
	o.invalidateToken()
	token, err := o.accessToken(ctx, runtime)
	if err != nil {
		return err
	}
	return validateMailSendRole(token)
}

func (o *Outlook) Send(ctx context.Context, message domain.EmailMessage) error {
	runtime, err := o.settings.Runtime()
	if err != nil {
		return &Error{Code: "OUTLOOK_CONFIGURATION_ERROR", Message: "Unable to read the secure Outlook configuration."}
	}
	if !runtime.Enabled || !complete(runtime) {
		return ErrNotConfigured
	}
	token, err := o.accessToken(ctx, runtime)
	if err != nil {
		return err
	}
	body, err := buildMIME(runtime, message)
	if err != nil {
		return err
	}
	endpoint := o.graphBase + "/v1.0/users/" + url.PathEscape(runtime.SenderEmail) + "/sendMail"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(base64.StdEncoding.EncodeToString(body)))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "text/plain")
	response, err := o.client.Do(request)
	if err != nil {
		if timedOut(err) {
			return &Error{Code: "OUTLOOK_TIMEOUT", Message: "The email service did not respond within the expected time."}
		}
		return &Error{Code: "OUTLOOK_REQUEST_FAILED", Message: "Outlook did not respond to the delivery attempt."}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			o.invalidateToken()
		}
		switch response.StatusCode {
		case http.StatusForbidden:
			return &Error{Code: "OUTLOOK_SEND_FORBIDDEN", Message: "Outlook authenticated but did not authorize delivery. Grant the Mail.Send application permission with administrator consent and confirm that the sender mailbox is within the allowed Exchange scope."}
		case http.StatusNotFound:
			return &Error{Code: "OUTLOOK_MAILBOX_NOT_FOUND", Message: "The sender mailbox was not found in Microsoft 365. Confirm the email address and that it has an Exchange Online mailbox."}
		case http.StatusTooManyRequests:
			return &Error{Code: "OUTLOOK_THROTTLED", Message: "Microsoft 365 temporarily throttled deliveries. Wait a few moments and try again."}
		default:
			return &Error{Code: "OUTLOOK_SEND_FAILED", Message: fmt.Sprintf("Outlook rejected delivery (HTTP %d).", response.StatusCode)}
		}
	}
	return nil
}

func buildMIME(runtime emailconfig.Runtime, message domain.EmailMessage) ([]byte, error) {
	sender, valid := validation.EmailAddress(runtime.SenderEmail)
	if !valid || strings.ContainsAny(runtime.SenderName, "\r\n") || strings.ContainsAny(message.Subject, "\r\n") || len(message.To) == 0 {
		return nil, &Error{Code: "OUTLOOK_INVALID_MESSAGE", Message: "The email message contains invalid headers."}
	}
	recipients := make([]string, 0, len(message.To))
	for _, raw := range message.To {
		recipient, recipientValid := validation.EmailAddress(raw)
		if !recipientValid {
			return nil, &Error{Code: "OUTLOOK_INVALID_MESSAGE", Message: "The message has an invalid recipient."}
		}
		recipients = append(recipients, recipient)
	}
	var content bytes.Buffer
	parts := multipart.NewWriter(&content)
	plainHeader := textproto.MIMEHeader{}
	plainHeader.Set("Content-Type", `text/plain; charset="UTF-8"`)
	plainHeader.Set("Content-Transfer-Encoding", "base64")
	plain, err := parts.CreatePart(plainHeader)
	if err != nil {
		return nil, err
	}
	if _, err = plain.Write([]byte(base64.StdEncoding.EncodeToString([]byte(message.Text)))); err != nil {
		return nil, err
	}
	htmlHeader := textproto.MIMEHeader{}
	htmlHeader.Set("Content-Type", `text/html; charset="UTF-8"`)
	htmlHeader.Set("Content-Transfer-Encoding", "base64")
	htmlPart, err := parts.CreatePart(htmlHeader)
	if err != nil {
		return nil, err
	}
	if _, err = htmlPart.Write([]byte(base64.StdEncoding.EncodeToString([]byte(message.HTML)))); err != nil {
		return nil, err
	}
	if err = parts.Close(); err != nil {
		return nil, err
	}
	from := (&mail.Address{Name: runtime.SenderName, Address: sender}).String()
	var result bytes.Buffer
	result.WriteString("From: " + from + "\r\n")
	result.WriteString("To: " + strings.Join(recipients, ", ") + "\r\n")
	result.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", message.Subject) + "\r\n")
	result.WriteString("MIME-Version: 1.0\r\n")
	result.WriteString("Content-Type: " + mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": parts.Boundary()}) + "\r\n\r\n")
	result.Write(content.Bytes())
	return result.Bytes(), nil
}

func (o *Outlook) accessToken(ctx context.Context, runtime emailconfig.Runtime) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	signature := credentialSignature(runtime)
	if o.token != "" && o.signature == signature && time.Now().Add(time.Minute).Before(o.expiresAt) {
		return o.token, nil
	}
	form := url.Values{}
	form.Set("client_id", runtime.ClientID)
	form.Set("client_secret", runtime.ClientSecret)
	form.Set("grant_type", "client_credentials")
	form.Set("scope", "https://graph.microsoft.com/.default")
	endpoint := o.tokenBase + "/" + url.PathEscape(runtime.TenantID) + "/oauth2/v2.0/token"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := o.client.Do(request)
	if err != nil {
		if timedOut(err) {
			return "", &Error{Code: "OUTLOOK_TIMEOUT", Message: "Microsoft 365 did not respond within the expected time."}
		}
		return "", &Error{Code: "OUTLOOK_AUTH_FAILED", Message: "Unable to authenticate with Microsoft 365."}
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if readErr != nil {
		return "", &Error{Code: "OUTLOOK_AUTH_FAILED", Message: "The Microsoft 365 authentication response is invalid."}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", &Error{Code: "OUTLOOK_AUTH_FAILED", Message: fmt.Sprintf("O Microsoft 365 recusou as credenciais (HTTP %d).", response.StatusCode)}
	}
	var value tokenValue
	if json.Unmarshal(data, &value) != nil || value.AccessToken == "" || value.ExpiresIn < 1 {
		return "", &Error{Code: "OUTLOOK_AUTH_FAILED", Message: "The Microsoft 365 authentication response is invalid."}
	}
	o.token = value.AccessToken
	o.signature = signature
	o.expiresAt = time.Now().Add(time.Duration(value.ExpiresIn) * time.Second)
	return o.token, nil
}

func timedOut(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}

func validateMailSendRole(token string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		// Some Microsoft cloud configurations may issue an opaque token. In
		// that case authorization is confirmed only by the sendMail request.
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims struct {
		Roles []string `json:"roles"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return nil
	}
	for _, role := range claims.Roles {
		if strings.EqualFold(role, "Mail.Send") {
			return nil
		}
	}
	return &Error{Code: "OUTLOOK_MAIL_SEND_PERMISSION_MISSING", Message: "The credentials are valid, but the application token does not include the Mail.Send application permission. Add the permission in Microsoft Graph and grant administrator consent."}
}

func (o *Outlook) invalidateToken() {
	o.mu.Lock()
	o.token = ""
	o.expiresAt = time.Time{}
	o.signature = ""
	o.mu.Unlock()
}

func complete(value emailconfig.Runtime) bool {
	return value.TenantID != "" && value.ClientID != "" && value.ClientSecret != "" && value.SenderEmail != ""
}

func credentialSignature(value emailconfig.Runtime) string {
	sum := sha256.Sum256([]byte(value.TenantID + "\x00" + value.ClientID + "\x00" + value.ClientSecret))
	return hex.EncodeToString(sum[:])
}
