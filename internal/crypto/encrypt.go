package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const (
	// KeySize is the required AES-256 key size in bytes.
	KeySize = 32

	// NonceSize is the standard GCM nonce size in bytes (12 bytes / 96 bits).
	NonceSize = 12

	// HexKeyLength is the required length of hex-encoded 32-byte key (64 hex characters).
	HexKeyLength = 64
)

var (
	// ErrInvalidKeySize is returned when an AES key is not exactly 32 bytes (256 bits).
	ErrInvalidKeySize = errors.New("crypto: invalid key size: must be exactly 32 bytes (256 bits)")

	// ErrInvalidHexKey is returned when a hex-encoded key does not consist of exactly 64 hexadecimal characters.
	ErrInvalidHexKey = errors.New("crypto: invalid hex key: must be exactly 64 hexadecimal characters")

	// ErrCiphertextTooShort is returned when the decoded ciphertext is shorter than nonce + tag overhead.
	ErrCiphertextTooShort = errors.New("crypto: ciphertext too short")

	// ErrInvalidBase64 is returned when ciphertext is not valid standard base64.
	ErrInvalidBase64 = errors.New("crypto: invalid base64 encoding")

	// ErrDecryptionFailed is returned when ciphertext decryption or MAC verification fails (tampered or corrupted ciphertext).
	ErrDecryptionFailed = errors.New("crypto: decryption failed: ciphertext is corrupted or tampered")
)

// KeyFromHex validates that hexKey is exactly 64 hex characters (32 bytes)
// and decodes it into a byte slice.
func KeyFromHex(hexKey string) ([]byte, error) {
	if len(hexKey) != HexKeyLength {
		return nil, fmt.Errorf("%w: got length %d, expected %d", ErrInvalidHexKey, len(hexKey), HexKeyLength)
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidHexKey, err)
	}
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}
	return key, nil
}

// GenerateKey generates a cryptographically secure random 32-byte (256-bit) AES key.
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("crypto: failed to generate random key: %w", err)
	}
	return key, nil
}

// GenerateHexKey generates a cryptographically secure random 32-byte AES key and returns it as a 64-character hex string.
func GenerateHexKey() (string, error) {
	key, err := GenerateKey()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

// Encrypt encrypts plaintext using AES-256-GCM authenticated encryption.
// It enforces that the key is exactly 32 bytes.
// A 12-byte random nonce is generated using crypto/rand per operation.
// The nonce is prepended to the ciphertext, and the result is returned as a standard base64 string.
func Encrypt(plaintext []byte, key []byte) (string, error) {
	if len(key) != KeySize {
		return "", ErrInvalidKeySize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: failed to create cipher block: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: failed to create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize(), gcm.NonceSize()+len(plaintext)+gcm.Overhead())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: failed to generate nonce: %w", err)
	}

	// Prepend nonce: gcm.Seal appends ciphertext and authentication tag to dst (nonce).
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)

	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decodes standard base64 ciphertext, extracts the 12-byte nonce,
// and decrypts the ciphertext using AES-256-GCM while verifying the MAC tag.
// It returns a descriptive error wrapping ErrDecryptionFailed upon failure.
func Decrypt(ciphertextBase64 string, key []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}

	data, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidBase64, err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to create cipher block: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: failed to create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize+gcm.Overhead() {
		return nil, ErrCiphertextTooShort
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return plaintext, nil
}
