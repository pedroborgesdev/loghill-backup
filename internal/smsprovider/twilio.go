package smsprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"logtheater/internal/domain"
)

const defaultAPIBase = "https://api.twilio.com"

type TextRenderer interface {
	RenderEventText(domain.Notification, string) string
}

type Twilio struct {
	enabled    bool
	accountSID string
	authToken  string
	from       string
	apiBase    string
	client     *http.Client
	renderer   TextRenderer
}

func NewTwilio(enabled bool, accountSID, authToken, from string, client *http.Client, renderer TextRenderer) *Twilio {
	return NewTwilioWithEndpoint(enabled, accountSID, authToken, from, defaultAPIBase, client, renderer)
}

func NewTwilioWithEndpoint(enabled bool, accountSID, authToken, from, apiBase string, client *http.Client, renderer TextRenderer) *Twilio {
	return &Twilio{enabled: enabled, accountSID: strings.TrimSpace(accountSID), authToken: authToken, from: strings.TrimSpace(from), apiBase: strings.TrimRight(apiBase, "/"), client: client, renderer: renderer}
}

func (t *Twilio) Send(ctx context.Context, value domain.Notification) error {
	if !t.enabled || t.accountSID == "" || t.authToken == "" || t.from == "" {
		return errors.New("The SMS provider is not configured or enabled.")
	}
	if t.client == nil || t.renderer == nil {
		return errors.New("The SMS provider is unavailable.")
	}
	message := t.renderer.RenderEventText(value, value.Event.SMSTemplate)
	if strings.TrimSpace(message) == "" || utf8.RuneCountInString(message) > 1600 {
		return errors.New("The SMS message is empty or exceeds 1,600 characters.")
	}
	for _, recipient := range value.Event.PhoneNumbers {
		if err := t.sendOne(ctx, recipient, message); err != nil {
			return err
		}
	}
	return nil
}

func (t *Twilio) sendOne(ctx context.Context, recipient, message string) error {
	form := url.Values{"To": {recipient}, "From": {t.from}, "Body": {message}}
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", t.apiBase, url.PathEscape(t.accountSID))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("Unable to prepare the SMS delivery.")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(t.accountSID, t.authToken)
	response, err := t.client.Do(request)
	if err != nil {
		return errors.New("The SMS provider did not respond.")
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil
	}
	var body struct {
		Code int `json:"code"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&body)
	if body.Code > 0 {
		return fmt.Errorf("The SMS provider rejected the delivery (code %d).", body.Code)
	}
	return fmt.Errorf("The SMS provider rejected the delivery (HTTP %d).", response.StatusCode)
}
