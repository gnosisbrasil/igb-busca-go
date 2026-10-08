// Package service mirrors SuperGnosis.Api Service classes.
package service

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenService mirrors TokenService.cs: HS256 JWTs keyed by the ASCII
// bytes of SECRET_JWT, claim "id", 1-day expiry.
type TokenService struct {
	secret string
}

// NewTokenService creates a TokenService.
func NewTokenService(secret string) *TokenService {
	return &TokenService{secret: secret}
}

// GenerateToken mirrors GenerateJwtTokenAsync. The claim set (id, exp, iat,
// nbf) matches what JwtSecurityTokenHandler emits for the same descriptor,
// so tokens are interchangeable with the .NET API.
func (s *TokenService) GenerateToken(userID string) (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"id":  userID,
		"exp": now.Add(24 * time.Hour).Unix(),
		"iat": now.Unix(),
		"nbf": now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.secret))
}

// ValidateToken mirrors ValidateToken: verifies the HMAC-SHA256 signature
// and lifetime (with .NET's default 5-minute clock skew), ignoring issuer
// and audience. Any failure is an error (surfaced as HTTP 500 by the
// middleware, exactly like the uncaught exception in JwtMiddleware).
func (s *TokenService) ValidateToken(raw string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(raw,
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return []byte(s.secret), nil
		},
		jwt.WithLeeway(5*time.Minute),
		jwt.WithValidMethods([]string{"HS256"}),
	)
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}
