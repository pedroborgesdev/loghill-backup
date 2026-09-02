package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"logtheater/internal/domain"
)

var (
	ErrInvalidURL   = errors.New("invalid webhook URL")
	ErrUnsafeTarget = errors.New("webhook target is not allowed")
)

type Client struct {
	resolver *net.Resolver
}

func NewClient() *Client { return &Client{resolver: net.DefaultResolver} }

func ValidateURL(raw string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return ErrInvalidURL
	}
	if parsed.Port() != "" {
		if _, err = net.LookupPort("tcp", parsed.Port()); err != nil {
			return ErrInvalidURL
		}
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && !publicIP(ip) {
		return ErrUnsafeTarget
	}
	return nil
}

func (c *Client) Send(ctx context.Context, value domain.Notification) error {
	if err := ValidateURL(value.Event.WebhookURL); err != nil {
		return err
	}
	parsed, _ := url.Parse(value.Event.WebhookURL)
	ips, err := c.resolver.LookupIPAddr(ctx, parsed.Hostname())
	if err != nil || len(ips) == 0 {
		return errors.New("webhook hostname could not be resolved")
	}
	var target net.IP
	for _, candidate := range ips {
		if publicIP(candidate.IP) {
			target = candidate.IP
			break
		}
	}
	if target == nil {
		return ErrUnsafeTarget
	}
	payload := struct {
		EventOccurrenceID string                 `json:"event_occurrence_id,omitempty"`
		Event             domain.EventDefinition `json:"event"`
		Sender            domain.Sender          `json:"sender"`
		Log               domain.LogEntry        `json:"log"`
		DeliveredAt       time.Time              `json:"delivered_at"`
	}{
		EventOccurrenceID: value.Entry.EventOccurrenceID,
		Event:             value.Event, Sender: value.Sender, Log: value.Entry,
		DeliveredAt: time.Now().UTC(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.New("webhook payload could not be encoded")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, value.Event.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return ErrInvalidURL
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "LogHill-Webhook/1.0")
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(target.String(), port))
		},
		TLSHandshakeTimeout: 10 * time.Second,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("webhook request failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4*1024))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", response.StatusCode)
	}
	return nil
}

func publicIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	if ipv4 := ip.To4(); ipv4 != nil && ipv4[0] == 100 && ipv4[1] >= 64 && ipv4[1] <= 127 {
		return false
	}
	return ip.IsGlobalUnicast()
}
