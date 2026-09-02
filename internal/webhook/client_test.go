package webhook

import (
	"errors"
	"testing"
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
	if err := ValidateURL("https://hooks.example.com/loghill"); err != nil {
		t.Fatalf("public HTTPS URL rejected: %v", err)
	}
	if !errors.Is(ValidateURL("https://127.0.0.1/hook"), ErrUnsafeTarget) {
		t.Fatal("private literal should return the unsafe-target error")
	}
}
