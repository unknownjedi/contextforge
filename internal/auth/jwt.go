package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	// ErrEmptySecret is returned when the JWT signing or verification secret is empty.
	ErrEmptySecret = errors.New("auth: jwt secret cannot be empty")

	// ErrEmptyToken is returned when the token string to validate is empty.
	ErrEmptyToken = errors.New("auth: token string cannot be empty")

	// ErrInvalidToken is returned when token parsing or validation fails.
	ErrInvalidToken = errors.New("auth: invalid token")

	// ErrExpiredToken is returned when the token has expired.
	ErrExpiredToken = fmt.Errorf("%w: token has expired", ErrInvalidToken)

	// ErrInvalidSignature is returned when the token signature does not match.
	ErrInvalidSignature = fmt.Errorf("%w: invalid token signature", ErrInvalidToken)
)

// Claims represents the JWT claims for ContextForge user authentication.
// It embeds jwt.RegisteredClaims to supply standard RFC 7519 claims:
// - sub: user ID string
// - exp: expiration timestamp
// - jti: unique token identifier (UUID)
// - iat: issued-at timestamp
// - nbf: not-before timestamp
// Along with custom user claims: UserID and GitHubLogin.
type Claims struct {
	UserID      uuid.UUID `json:"user_id"`
	GitHubLogin string    `json:"login"`
	jwt.RegisteredClaims
}

// GenerateToken creates an HMAC-SHA256 signed JWT token containing standard registered
// claims (sub, exp, jti, iat, nbf) and custom claims (login, user_id).
func GenerateToken(userID uuid.UUID, githubLogin string, secret []byte, duration time.Duration) (string, error) {
	if len(secret) == 0 {
		return "", ErrEmptySecret
	}

	now := time.Now()
	claims := Claims{
		UserID:      userID,
		GitHubLogin: githubLogin,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(now.Add(duration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        uuid.New().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("auth: failed to sign token: %w", err)
	}

	return signed, nil
}

// ValidateToken parses and validates an HMAC-SHA256 JWT token string against the secret.
// It enforces the HS256 signing method and verifies token expiration and signature integrity.
// Upon success, it returns the parsed Claims.
func ValidateToken(tokenString string, secret []byte) (*Claims, error) {
	if len(secret) == 0 {
		return nil, ErrEmptySecret
	}
	if tokenString == "" {
		return nil, ErrEmptyToken
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		// Strictly enforce HMAC-SHA256
		if sm, ok := t.Method.(*jwt.SigningMethodHMAC); !ok || sm.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, t.Header["alg"])
		}
		return secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		if errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return nil, ErrInvalidSignature
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	// In case UserID was only present as the Subject claim, populate it
	if claims.UserID == uuid.Nil && claims.Subject != "" {
		if parsed, err := uuid.Parse(claims.Subject); err == nil {
			claims.UserID = parsed
		}
	}

	return claims, nil
}
