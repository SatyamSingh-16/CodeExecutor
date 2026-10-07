package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenManager defines operations for issuing and validating JWT tokens.
type TokenManager interface {
	GenerateToken(userID string) (string, error)
	ValidateToken(tokenStr string) (string, error)
}

// JWTConfig holds configuration parameters for the JWT token manager.
type JWTConfig struct {
	Secret     string
	Expiration time.Duration
}

// JWTTokenManager implements TokenManager using HMAC-SHA256 tokens.
type JWTTokenManager struct {
	secret     []byte
	expiration time.Duration
}

// NewJWTTokenManager creates a new JWTTokenManager with the given secret and expiration.
func NewJWTTokenManager(cfg JWTConfig) (*JWTTokenManager, error) {
	if len(cfg.Secret) == 0 {
		return nil, errors.New("JWT secret cannot be empty")
	}
	if cfg.Expiration == 0 {
		cfg.Expiration = 24 * time.Hour
	}
	return &JWTTokenManager{
		secret:     []byte(cfg.Secret),
		expiration: cfg.Expiration,
	}, nil
}

// Claims represents the standard JWT claims payload.
type Claims struct {
	jwt.RegisteredClaims
}

// GenerateToken issues a signed JWT token containing the user ID as subject.
func (m *JWTTokenManager) GenerateToken(userID string) (string, error) {
	if userID == "" {
		return "", errors.New("user ID cannot be empty")
	}

	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.expiration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return signedToken, nil
}

// ValidateToken verifies token signature, algorithm, expiration, and extracts user ID.
func (m *JWTTokenManager) ValidateToken(tokenStr string) (string, error) {
	if tokenStr == "" {
		return "", ErrInvalidToken
	}

	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		// Enforce explicit HMAC-SHA256 signing algorithm
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing algorithm: %v", t.Header["alg"])
		}
		return m.secret, nil
	})

	if err != nil || token == nil || !token.Valid {
		return "", ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || claims.Subject == "" {
		return "", ErrInvalidToken
	}

	return claims.Subject, nil
}
