package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var testSecret = []byte("super-secret-contextforge-jwt-key-2026")

func TestGenerateToken_Success(t *testing.T) {
	userID := uuid.New()
	login := "octocat"
	duration := 1 * time.Hour

	tokenString, err := GenerateToken(userID, login, testSecret, duration)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	if tokenString == "" {
		t.Fatalf("expected non-empty token string")
	}

	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		t.Fatalf("expected standard 3-part JWT, got %d parts", len(parts))
	}

	claims, err := ValidateToken(tokenString, testSecret)
	if err != nil {
		t.Fatalf("unexpected error validating generated token: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("expected UserID %v, got %v", userID, claims.UserID)
	}
	if claims.GitHubLogin != login {
		t.Errorf("expected GitHubLogin %q, got %q", login, claims.GitHubLogin)
	}
	if claims.Subject != userID.String() {
		t.Errorf("expected Subject %q, got %q", userID.String(), claims.Subject)
	}
	if claims.ID == "" {
		t.Errorf("expected non-empty jti (ID) claim")
	}
	if _, err := uuid.Parse(claims.ID); err != nil {
		t.Errorf("expected jti to be a valid UUID, got: %v", err)
	}
	if claims.ExpiresAt == nil {
		t.Errorf("expected ExpiresAt claim to be set")
	} else if claims.ExpiresAt.Time.Before(time.Now()) {
		t.Errorf("expected ExpiresAt to be in the future")
	}
	if claims.IssuedAt == nil {
		t.Errorf("expected IssuedAt claim to be set")
	}
}

func TestValidateToken_Expired(t *testing.T) {
	userID := uuid.New()
	login := "octocat"
	duration := -10 * time.Minute // Expired in the past

	tokenString, err := GenerateToken(userID, login, testSecret, duration)
	if err != nil {
		t.Fatalf("unexpected error generating expired token: %v", err)
	}

	_, err = ValidateToken(tokenString, testSecret)
	if err == nil {
		t.Fatalf("expected validation error for expired token, got nil")
	}
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken, got: %v", err)
	}
}

func TestValidateToken_TamperedSignature(t *testing.T) {
	userID := uuid.New()
	login := "octocat"
	duration := 1 * time.Hour

	tokenString, err := GenerateToken(userID, login, testSecret, duration)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected token parts: %d", len(parts))
	}

	// Tamper the signature portion (3rd part)
	sig := []byte(parts[2])
	if sig[len(sig)-1] == 'A' {
		sig[len(sig)-1] = 'B'
	} else {
		sig[len(sig)-1] = 'A'
	}
	tamperedToken := parts[0] + "." + parts[1] + "." + string(sig)

	_, err = ValidateToken(tamperedToken, testSecret)
	if err == nil {
		t.Fatalf("expected error for tampered signature, got nil")
	}
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got: %v", err)
	}
}

func TestValidateToken_TamperedPayload(t *testing.T) {
	userID := uuid.New()
	login := "octocat"
	duration := 1 * time.Hour

	tokenString, err := GenerateToken(userID, login, testSecret, duration)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	parts := strings.Split(tokenString, ".")
	// Modify payload part
	payload := []byte(parts[1])
	payload[0] ^= 0x01
	tamperedToken := parts[0] + "." + string(payload) + "." + parts[2]

	_, err = ValidateToken(tamperedToken, testSecret)
	if err == nil {
		t.Fatalf("expected error for tampered payload, got nil")
	}
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got: %v", err)
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	userID := uuid.New()
	login := "octocat"
	tokenString, err := GenerateToken(userID, login, testSecret, 1*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wrongSecret := []byte("completely-different-secret-key-12345")
	_, err = ValidateToken(tokenString, wrongSecret)
	if err == nil {
		t.Fatalf("expected validation failure with wrong secret, got nil")
	}
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got: %v", err)
	}
}

func TestGenerateToken_EmptySecret(t *testing.T) {
	userID := uuid.New()
	_, err := GenerateToken(userID, "octocat", nil, 1*time.Hour)
	if !errors.Is(err, ErrEmptySecret) {
		t.Fatalf("expected ErrEmptySecret for nil secret, got: %v", err)
	}

	_, err = GenerateToken(userID, "octocat", []byte(""), 1*time.Hour)
	if !errors.Is(err, ErrEmptySecret) {
		t.Fatalf("expected ErrEmptySecret for empty secret, got: %v", err)
	}
}

func TestValidateToken_EmptyInputs(t *testing.T) {
	_, err := ValidateToken("", testSecret)
	if !errors.Is(err, ErrEmptyToken) {
		t.Fatalf("expected ErrEmptyToken for empty token string, got: %v", err)
	}

	_, err = ValidateToken("some.token.here", nil)
	if !errors.Is(err, ErrEmptySecret) {
		t.Fatalf("expected ErrEmptySecret for nil secret, got: %v", err)
	}

	_, err = ValidateToken("some.token.here", []byte(""))
	if !errors.Is(err, ErrEmptySecret) {
		t.Fatalf("expected ErrEmptySecret for empty secret, got: %v", err)
	}
}

func TestValidateToken_NoneAlgorithmAttack(t *testing.T) {
	// Construct a token with alg: "none"
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub":   uuid.New().String(),
		"login": "hacker",
		"exp":   time.Now().Add(1 * time.Hour).Unix(),
	})
	unsignedToken, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("unexpected error creating unsigned token: %v", err)
	}

	_, err = ValidateToken(unsignedToken, testSecret)
	if err == nil {
		t.Fatalf("expected validation error for 'none' algorithm token, got nil")
	}
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got: %v", err)
	}
}

func TestValidateToken_SubjectFallback(t *testing.T) {
	// Token with only 'sub' claim (no 'user_id' custom claim)
	expectedID := uuid.New()
	claims := jwt.RegisteredClaims{
		Subject:   expectedID.String(),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		ID:        uuid.New().String(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(testSecret)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	parsedClaims, err := ValidateToken(tokenString, testSecret)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if parsedClaims.UserID != expectedID {
		t.Fatalf("expected UserID %v populated from Subject, got %v", expectedID, parsedClaims.UserID)
	}
}
