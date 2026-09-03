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
	"regexp"
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
	renderer TextRenderer
}

type TextRenderer interface {
	RenderEventText(domain.Notification, string) string
}

func NewClient(renderer ...TextRenderer) *Client {
	client := &Client{resolver: net.DefaultResolver}
	if len(renderer) > 0 {
		client.renderer = renderer[0]
	}
	return client
}

var cookieNamePattern = regexp.MustCompile(`^[!#$%&'*+\-.^_` + "`" + `|~0-9A-Za-z]+$`)

var supportedMethods = map[string]bool{
	http.MethodGet: true, http.MethodHead: true, http.MethodPost: true,
	http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
	http.MethodConnect: true, http.MethodOptions: true, http.MethodTrace: true,
}

func ValidateRequestConfig(config domain.HTTPRequestConfig) error {
	method := strings.ToUpper(strings.TrimSpace(config.Method))
	if !supportedMethods[method] {
		return errors.New("Select a valid HTTP method.")
	}
	if err := ValidateURL(config.URL); err != nil {
		return errors.New("Enter a public HTTPS URL without embedded credentials.")
	}
	if len(config.Headers) > 50 {
		return errors.New("Use at most 50 HTTP headers.")
	}
	for name, value := range config.Headers {
		trimmedName := strings.TrimSpace(name)
		canonical := http.CanonicalHeaderKey(trimmedName)
		if !cookieNamePattern.MatchString(trimmedName) || len(name) > 100 || len(value) > 8192 || strings.ContainsAny(value, "\r\n") || reservedHeader(canonical) {
			return errors.New("One or more HTTP headers are invalid or restricted.")
		}
	}
	if len(config.Cookies) > 50 {
		return errors.New("Use at most 50 cookies.")
	}
	for name, value := range config.Cookies {
		if !cookieNamePattern.MatchString(strings.TrimSpace(name)) || len(name) > 100 || len(value) > 4096 || strings.ContainsAny(value, "\r\n;") {
			return errors.New("One or more cookies are invalid.")
		}
	}
	if len([]byte(config.Body)) > 64*1024 {
		return errors.New("The HTTP body must be at most 64 KiB.")
	}
	return nil
}

func reservedHeader(name string) bool {
	return name == "Host" || name == "Content-Length" || name == "Cookie" || name == "Connection" || name == "Transfer-Encoding" || strings.HasPrefix(name, "Proxy-")
}

func ValidateURL(raw string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return ErrInvalidURL
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") {
		return ErrUnsafeTarget
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
	if value.Event.ActionType == domain.EventActionHTTP {
		return c.sendRequest(ctx, value)
	}
	if err := ValidateURL(value.Event.WebhookURL); err != nil {
		return err
	}
	parsed, target, err := c.resolveTarget(ctx, value.Event.WebhookURL)
	if err != nil {
		return err
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
	request.Header.Set("User-Agent", "LogMate-Webhook/1.0")
	transport := pinnedTransport(parsed, target)
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

func (c *Client) sendRequest(ctx context.Context, value domain.Notification) error {
	if value.Event.HTTPRequest == nil {
		return errors.New("HTTP request is not configured")
	}
	config := *value.Event.HTTPRequest
	if err := ValidateRequestConfig(config); err != nil {
		return err
	}
	parsed, target, err := c.resolveTarget(ctx, config.URL)
	if err != nil {
		return err
	}
	render := func(text string) string {
		if c.renderer == nil {
			return text
		}
		return c.renderer.RenderEventText(value, text)
	}
	request, err := http.NewRequestWithContext(ctx, strings.ToUpper(strings.TrimSpace(config.Method)), config.URL, strings.NewReader(render(config.Body)))
	if err != nil {
		return ErrInvalidURL
	}
	for name, headerValue := range config.Headers {
		request.Header.Set(strings.TrimSpace(name), render(headerValue))
	}
	for name, cookieValue := range config.Cookies {
		request.AddCookie(&http.Cookie{Name: strings.TrimSpace(name), Value: render(cookieValue)})
	}
	request.Close = true
	if request.Header.Get("User-Agent") == "" {
		request.Header.Set("User-Agent", "LogMate-HTTP/1.0")
	}
	transport := pinnedTransport(parsed, target)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("HTTP request failed")
	}
	_ = response.Body.Close()
	return nil
}

func (c *Client) resolveTarget(ctx context.Context, rawURL string) (*url.URL, net.IP, error) {
	parsed, _ := url.Parse(rawURL)
	ips, err := c.resolver.LookupIPAddr(ctx, parsed.Hostname())
	if err != nil || len(ips) == 0 {
		return nil, nil, errors.New("HTTP hostname could not be resolved")
	}
	for _, candidate := range ips {
		if publicIP(candidate.IP) {
			return parsed, candidate.IP, nil
		}
	}
	return nil, nil, ErrUnsafeTarget
}

func pinnedTransport(parsed *url.URL, target net.IP) *http.Transport {
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	return &http.Transport{
		Proxy:                  nil,
		MaxResponseHeaderBytes: 64 * 1024,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(target.String(), port))
		},
		TLSHandshakeTimeout: 10 * time.Second,
	}
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
