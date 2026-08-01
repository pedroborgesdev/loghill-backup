package handler

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIIncludesValidSettingsOperations(t *testing.T) {
	data, err := os.ReadFile("../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		OpenAPI string                    `yaml:"openapi"`
		Paths   map[string]map[string]any `yaml:"paths"`
	}
	if err = yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("invalid OpenAPI YAML: %v", err)
	}
	if document.OpenAPI != "3.0.0" {
		t.Fatalf("unexpected OpenAPI version %q", document.OpenAPI)
	}
	settingsPath := document.Paths["/api/v1/settings"]
	if settingsPath["get"] == nil || settingsPath["put"] == nil {
		t.Fatalf("settings operations are missing: %+v", settingsPath)
	}
	requiredOperations := map[string][]string{
		"/api/v1/senders":                        {"get", "post"},
		"/api/v1/senders/check-id":               {"get"},
		"/api/v1/senders/{sender}":               {"get", "put", "delete"},
		"/api/v1/senders/{sender}/rotate-key":    {"post"},
		"/api/v1/senders/{sender}/revoke":        {"post"},
		"/api/v1/senders/{sender}/reactivate":    {"post"},
		"/api/v1/senders/{sender}/dependencies":  {"get"},
		"/api/v1/alerts":                         {"get", "post"},
		"/api/v1/alerts/{alertID}":               {"get", "put", "delete"},
		"/api/v1/alerts/{alertID}/status":        {"patch"},
		"/api/v1/alerts/{alertID}/test":          {"post"},
		"/api/v1/events":                         {"get", "post"},
		"/api/v1/events/check-key":               {"get"},
		"/api/v1/events/{eventID}":               {"get", "put", "delete"},
		"/api/v1/events/{eventID}/status":        {"patch"},
		"/api/v1/events/{eventID}/test":          {"post"},
		"/api/v1/settings/email":                 {"get", "put"},
		"/api/v1/settings/email/test-connection": {"post"},
		"/api/v1/settings/email/send-test":       {"post"},
	}
	if document.Paths["/api/v1/senders/init"] != nil {
		t.Fatal("legacy public sender initialization is still documented")
	}
	for path, methods := range requiredOperations {
		operations := document.Paths[path]
		for _, method := range methods {
			if operations[method] == nil {
				t.Fatalf("%s %s is missing from OpenAPI", method, path)
			}
		}
	}
}
