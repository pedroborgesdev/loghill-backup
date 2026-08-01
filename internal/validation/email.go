package validation

import (
	"net/mail"
	"strings"
)

func EmailAddress(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || strings.ContainsAny(normalized, "\r\n") || len(normalized) > 254 {
		return "", false
	}
	parsed, err := mail.ParseAddress(normalized)
	if err != nil || parsed.Address != normalized || !strings.Contains(normalized, "@") {
		return "", false
	}
	return normalized, true
}
