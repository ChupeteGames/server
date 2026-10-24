package security

import (
	"errors"
	"testing"
	"time"

	"server/internal/auth/domain"
)

func TestBcryptHasherHashesAndVerifiesPassword(t *testing.T) {
	hasher := NewBcryptHasher()
	hash, err := hasher.Hash("super_secret_password")
	if err != nil {
		t.Fatalf("Hash() failed: %v", err)
	}
	if hash == "super_secret_password" {
		t.Fatal("expected password to be hashed, not plain text")
	}

	if err := hasher.Compare(hash, "super_secret_password"); err != nil {
		t.Fatalf("expected Compare() to succeed for matching password, got: %v", err)
	}
	if err := hasher.Compare(hash, "wrong_password"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for wrong password, got: %v", err)
	}
}

func TestJWTManagerGeneratesAndValidatesToken(t *testing.T) {
	manager := NewJWTManager("test_secret_key_for_hmac_sha256_32b")

	token, err := manager.GenerateToken("player-uuid-123", "santiago")
	if err != nil {
		t.Fatalf("GenerateToken() failed: %v", err)
	}

	playerID, err := manager.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken() failed: %v", err)
	}
	if playerID != "player-uuid-123" {
		t.Fatalf("expected playerID 'player-uuid-123', got %q", playerID)
	}

	// Token alterado o firmado con otra clave debe fallar.
	otherManager := NewJWTManager("different_secret_key_for_hmac_32b")
	if _, err := otherManager.ValidateToken(token); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for signature mismatch, got: %v", err)
	}

	// Token expirado debe fallar.
	expiredManager := NewJWTManager("test_secret_key_for_hmac_sha256_32b")
	expiredManager.now = func() time.Time {
		return time.Now().Add(48 * time.Hour)
	}
	if _, err := expiredManager.ValidateToken(token); !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for expired token, got: %v", err)
	}
}
