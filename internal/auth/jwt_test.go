package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTTokenManager_ValidToken(t *testing.T) {
	mgr, err := NewJWTTokenManager(JWTConfig{
		Secret:     "super-secret-key-12345",
		Expiration: 1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("unexpected error initializing manager: %v", err)
	}

	userID := "user-uuid-123"
	tokenStr, err := mgr.GenerateToken(userID)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	if tokenStr == "" {
		t.Fatal("expected non-empty token string")
	}

	extractedID, err := mgr.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}

	if extractedID != userID {
		t.Errorf("expected userID %q, got %q", userID, extractedID)
	}
}

func TestJWTTokenManager_ExpiredToken(t *testing.T) {
	mgr, err := NewJWTTokenManager(JWTConfig{
		Secret:     "super-secret-key-12345",
		Expiration: -1 * time.Second, // already expired
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tokenStr, err := mgr.GenerateToken("user-expired")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = mgr.ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("expected error validating expired token, got nil")
	}
}

func TestJWTTokenManager_InvalidSignature(t *testing.T) {
	mgr1, _ := NewJWTTokenManager(JWTConfig{Secret: "secret-one", Expiration: time.Hour})
	mgr2, _ := NewJWTTokenManager(JWTConfig{Secret: "secret-two", Expiration: time.Hour})

	tokenStr, err := mgr1.GenerateToken("user-sig")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = mgr2.ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("expected signature mismatch error, got nil")
	}
}

func TestJWTTokenManager_MalformedToken(t *testing.T) {
	mgr, _ := NewJWTTokenManager(JWTConfig{Secret: "secret", Expiration: time.Hour})

	badTokens := []string{
		"",
		"not-a-token",
		"a.b.c.d",
		"header.payload",
	}

	for _, bt := range badTokens {
		_, err := mgr.ValidateToken(bt)
		if err == nil {
			t.Errorf("expected error for malformed token %q, got nil", bt)
		}
	}
}

func TestJWTTokenManager_EmptySubjectRejected(t *testing.T) {
	mgr, _ := NewJWTTokenManager(JWTConfig{Secret: "secret", Expiration: time.Hour})

	_, err := mgr.GenerateToken("")
	if err == nil {
		t.Fatal("expected error generating token with empty user ID")
	}

	// Manually construct token with empty subject
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte("secret"))

	_, err = mgr.ValidateToken(signed)
	if err == nil {
		t.Fatal("expected validation failure for token with empty subject")
	}
}

func TestJWTTokenManager_AlgorithmConfusionNoneRejected(t *testing.T) {
	mgr, _ := NewJWTTokenManager(JWTConfig{Secret: "secret", Expiration: time.Hour})

	// Construct token with alg=none
	token := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-none-attack",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to craft none token: %v", err)
	}

	_, err = mgr.ValidateToken(signed)
	if err == nil {
		t.Fatal("expected algorithm confusion rejection for 'none' algorithm")
	}
}

func TestJWTTokenManager_ConfigValidation(t *testing.T) {
	_, err := NewJWTTokenManager(JWTConfig{Secret: ""})
	if err == nil {
		t.Fatal("expected error on empty secret")
	}
}
