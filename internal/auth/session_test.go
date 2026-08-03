package auth

import "testing"

func TestManagerCreateAndValidate(t *testing.T) {
	manager := NewManager("secret", 0, false)
	if !manager.Authenticate("secret") || manager.Authenticate("wrong") {
		t.Fatal("password check failed")
	}
	session, err := manager.Create()
	if err != nil {
		t.Fatal(err)
	}
	if !manager.Valid(session.ID) {
		t.Fatal("session should be valid")
	}
	manager.Revoke(session.ID)
	if manager.Valid(session.ID) {
		t.Fatal("revoked session should be invalid")
	}
}
