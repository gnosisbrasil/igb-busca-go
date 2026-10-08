package service

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestTokenRoundtrip(t *testing.T) {
	svc := NewTokenService("test-secret")
	token, err := svc.GenerateToken("42")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	claims, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims["id"] != "42" {
		t.Fatalf("id claim = %v, want 42", claims["id"])
	}
}

func TestTokenWrongSecret(t *testing.T) {
	a := NewTokenService("one")
	b := NewTokenService("two")
	token, _ := a.GenerateToken("1")
	if _, err := b.ValidateToken(token); err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestTokenTampered(t *testing.T) {
	svc := NewTokenService("s")
	token, _ := svc.GenerateToken("1")
	bad := token[:len(token)-2] + "xx"
	if _, err := svc.ValidateToken(bad); err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestTokenExpired(t *testing.T) {
	svc := NewTokenService("s")
	now := time.Now().UTC()
	claims := jwt.MapClaims{"id": "1", "exp": now.Add(-time.Hour).Unix()}
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("s"))
	if _, err := svc.ValidateToken(token); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestVisiblePerfis(t *testing.T) {
	cases := map[string][]string{
		"publico":       {"publico"},
		"segundacamara": {"publico", "segundacamara"},
		"sacerdotal":    {"publico", "segundacamara", "sacerdotal"},
		"":              {"publico"},
		"outro":         {"publico"},
	}
	for in, want := range cases {
		got := visiblePerfis(in)
		if len(got) != len(want) {
			t.Fatalf("visiblePerfis(%q) = %v, want %v", in, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("visiblePerfis(%q) = %v, want %v", in, got, want)
			}
		}
	}
}

func TestMapDriveURL(t *testing.T) {
	id := "abc123"
	if got := mapDriveURL(&id); got != "https://drive.google.com/file/d/abc123/view" {
		t.Fatalf("drive url = %q", got)
	}
	empty := ""
	if got := mapDriveURL(&empty); got != fallbackURL {
		t.Fatalf("empty drive url = %q", got)
	}
	if got := mapDriveURL(nil); got != fallbackURL {
		t.Fatalf("nil drive url = %q", got)
	}
}
