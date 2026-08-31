package domain

import "testing"

func TestParseSeverityAcceptsUndefined(t *testing.T) {
	severity, err := ParseSeverity(" undefined ")
	if err != nil {
		t.Fatal(err)
	}
	if severity != Undefined {
		t.Fatalf("severity=%q, want %q", severity, Undefined)
	}
}
