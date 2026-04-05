package auth

import (
	"testing"

	"avito/internal/model"
)

func TestIssueAndParseToken(t *testing.T) {
	secret := "test-secret"
	token, err := IssueToken(secret, model.RoleUser)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	claims, err := ParseToken(secret, token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.Role != model.RoleUser {
		t.Fatalf("expected role user, got %s", claims.Role)
	}
	if claims.UserID != DummyUserUserID {
		t.Fatalf("expected fixed user id, got %s", claims.UserID)
	}
}

func TestIssueTokenInvalidRole(t *testing.T) {
	_, err := IssueToken("s", model.Role("bad"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseTokenWrongSecret(t *testing.T) {
	token, err := IssueToken("secret-1", model.RoleAdmin)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	_, err = ParseToken("secret-2", token)
	if err == nil {
		t.Fatal("expected parse error")
	}
}
