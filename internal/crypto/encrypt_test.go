package crypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestKeyFromHex_Valid(t *testing.T) {
	tests := []struct {
		name   string
		hexKey string
	}{
		{
			name:   "lowercase 64 hex characters",
			hexKey: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		{
			name:   "uppercase 64 hex characters",
			hexKey: "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF",
		},
		{
			name:   "all zeros",
			hexKey: strings.Repeat("0", 64),
		},
		{
			name:   "all f",
			hexKey: strings.Repeat("f", 64),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := KeyFromHex(tt.hexKey)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(key) != KeySize {
				t.Fatalf("expected key length %d, got %d", KeySize, len(key))
			}
			expected, _ := hex.DecodeString(tt.hexKey)
			if !bytes.Equal(key, expected) {
				t.Fatalf("key bytes mismatch")
			}
		})
	}
}

func TestKeyFromHex_Invalid(t *testing.T) {
	tests := []struct {
		name   string
		hexKey string
	}{
		{name: "empty string", hexKey: ""},
		{name: "too short - 32 chars", hexKey: strings.Repeat("a", 32)},
		{name: "too short - 63 chars", hexKey: strings.Repeat("a", 63)},
		{name: "too long - 65 chars", hexKey: strings.Repeat("a", 65)},
		{name: "too long - 128 chars", hexKey: strings.Repeat("a", 128)},
		{name: "invalid hex chars", hexKey: strings.Repeat("z", 64)},
		{name: "spaces in hex key", hexKey: strings.Repeat(" ", 64)},
		{name: "contains special characters", hexKey: strings.Repeat("a", 62) + "!@"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := KeyFromHex(tt.hexKey)
			if err == nil {
				t.Fatalf("expected error for input %q, got key: %x", tt.hexKey, key)
			}
			if !errors.Is(err, ErrInvalidHexKey) && !errors.Is(err, ErrInvalidKeySize) {
				t.Fatalf("expected ErrInvalidHexKey or ErrInvalidKeySize, got: %v", err)
			}
		})
	}
}

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	testCases := []struct {
		name      string
		plaintext []byte
	}{
		{
			name:      "empty plaintext",
			plaintext: []byte(""),
		},
		{
			name:      "short string",
			plaintext: []byte("hello, ContextForge!"),
		},
		{
			name:      "simulated OAuth token",
			plaintext: []byte("gho_16C7e42F292c6912E7710c838347Ae178B4a"),
		},
		{
			name:      "unicode and emoji string",
			plaintext: []byte("🔒 ContextForge 密钥 🔑 - Cryptographic Security"),
		},
		{
			name:      "binary payload (1KB)",
			plaintext: make([]byte, 1024),
		},
		{
			name:      "large payload (64KB)",
			plaintext: make([]byte, 64*1024),
		},
	}

	// Fill binary payloads with random data
	for i := 4; i < len(testCases); i++ {
		if _, err := rand.Read(testCases[i].plaintext); err != nil {
			t.Fatalf("failed to generate random plaintext: %v", err)
		}
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ciphertext, err := Encrypt(tc.plaintext, key)
			if err != nil {
				t.Fatalf("encryption failed: %v", err)
			}

			if ciphertext == "" {
				t.Fatalf("expected non-empty ciphertext")
			}

			decrypted, err := Decrypt(ciphertext, key)
			if err != nil {
				t.Fatalf("decryption failed: %v", err)
			}

			if !bytes.Equal(decrypted, tc.plaintext) {
				t.Fatalf("decrypted text does not match plaintext")
			}
		})
	}
}

func TestEncrypt_NonceRandomness(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	plaintext := []byte("constant plaintext message")

	// Encrypt identical plaintext 10 times; all ciphertexts must be distinct
	ciphertexts := make(map[string]struct{})
	for i := 0; i < 10; i++ {
		ct, err := Encrypt(plaintext, key)
		if err != nil {
			t.Fatalf("encryption failed at iteration %d: %v", i, err)
		}
		if _, exists := ciphertexts[ct]; exists {
			t.Fatalf("duplicate ciphertext produced: nonce collision or static nonce detected")
		}
		ciphertexts[ct] = struct{}{}

		// Verify that each decrypts correctly
		decrypted, err := Decrypt(ct, key)
		if err != nil {
			t.Fatalf("decryption failed for ciphertext %d: %v", i, err)
		}
		if !bytes.Equal(decrypted, plaintext) {
			t.Fatalf("decrypted plaintext mismatch")
		}
	}
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	plaintext := []byte("super secret token to be encrypted")
	ciphertextBase64, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	raw, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}

	t.Run("tampered nonce byte", func(t *testing.T) {
		tampered := append([]byte(nil), raw...)
		tampered[0] ^= 0xFF // Flip bits in the nonce
		tamperedB64 := base64.StdEncoding.EncodeToString(tampered)

		_, err := Decrypt(tamperedB64, key)
		if err == nil {
			t.Fatalf("expected decryption error for tampered nonce, got nil")
		}
		if !errors.Is(err, ErrDecryptionFailed) {
			t.Fatalf("expected ErrDecryptionFailed, got: %v", err)
		}
	})

	t.Run("tampered ciphertext body byte", func(t *testing.T) {
		tampered := append([]byte(nil), raw...)
		// Tamper somewhere in the ciphertext body (after 12-byte nonce, before 16-byte tag)
		targetIdx := NonceSize + 2
		tampered[targetIdx] ^= 0xAA
		tamperedB64 := base64.StdEncoding.EncodeToString(tampered)

		_, err := Decrypt(tamperedB64, key)
		if err == nil {
			t.Fatalf("expected decryption error for tampered ciphertext, got nil")
		}
		if !errors.Is(err, ErrDecryptionFailed) {
			t.Fatalf("expected ErrDecryptionFailed, got: %v", err)
		}
	})

	t.Run("tampered auth tag byte", func(t *testing.T) {
		tampered := append([]byte(nil), raw...)
		// Tamper the very last byte (part of the 16-byte GCM tag)
		tampered[len(tampered)-1] ^= 0x01
		tamperedB64 := base64.StdEncoding.EncodeToString(tampered)

		_, err := Decrypt(tamperedB64, key)
		if err == nil {
			t.Fatalf("expected decryption error for tampered tag, got nil")
		}
		if !errors.Is(err, ErrDecryptionFailed) {
			t.Fatalf("expected ErrDecryptionFailed, got: %v", err)
		}
	})

	t.Run("tampered base64 character", func(t *testing.T) {
		chars := []byte(ciphertextBase64)
		if chars[len(chars)-2] == 'A' {
			chars[len(chars)-2] = 'B'
		} else {
			chars[len(chars)-2] = 'A'
		}
		tamperedB64 := string(chars)

		_, err := Decrypt(tamperedB64, key)
		if err == nil {
			t.Fatalf("expected decryption error for tampered base64 string, got nil")
		}
	})
}

func TestEncrypt_InvalidKeyLengths(t *testing.T) {
	plaintext := []byte("sample plaintext")
	invalidLengths := []int{0, 1, 15, 16, 24, 31, 33, 64}

	for _, length := range invalidLengths {
		key := make([]byte, length)
		_, err := Encrypt(plaintext, key)
		if err == nil {
			t.Fatalf("expected error for key length %d, got nil", length)
		}
		if !errors.Is(err, ErrInvalidKeySize) {
			t.Fatalf("expected ErrInvalidKeySize for key length %d, got: %v", length, err)
		}
	}
}

func TestDecrypt_InvalidKeyLengths(t *testing.T) {
	validKey, _ := GenerateKey()
	ciphertext, _ := Encrypt([]byte("data"), validKey)

	invalidLengths := []int{0, 1, 15, 16, 24, 31, 33, 64}

	for _, length := range invalidLengths {
		key := make([]byte, length)
		_, err := Decrypt(ciphertext, key)
		if err == nil {
			t.Fatalf("expected error for key length %d, got nil", length)
		}
		if !errors.Is(err, ErrInvalidKeySize) {
			t.Fatalf("expected ErrInvalidKeySize for key length %d, got: %v", length, err)
		}
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	keyA, _ := GenerateKey()
	keyB, _ := GenerateKey()

	ciphertext, err := Encrypt([]byte("confidential payload"), keyA)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	_, err = Decrypt(ciphertext, keyB)
	if err == nil {
		t.Fatalf("expected decryption failure with wrong key, got nil")
	}
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("expected ErrDecryptionFailed, got: %v", err)
	}
}

func TestDecrypt_CorruptedBase64(t *testing.T) {
	key, _ := GenerateKey()

	corruptedInputs := []string{
		"not-valid-base64!@#$%",
		"=====",
		"a",
		"abc",
		"!!!invalid!!!",
	}

	for _, input := range corruptedInputs {
		_, err := Decrypt(input, key)
		if err == nil {
			t.Fatalf("expected base64 decoding error for %q, got nil", input)
		}
		if !errors.Is(err, ErrInvalidBase64) {
			t.Fatalf("expected ErrInvalidBase64, got: %v", err)
		}
	}
}

func TestDecrypt_CiphertextTooShort(t *testing.T) {
	key, _ := GenerateKey()

	// AES-GCM requires at least NonceSize (12) + Overhead (16) = 28 bytes.
	tooShortLengths := []int{0, 1, 5, 11, 12, 13, 27}

	for _, length := range tooShortLengths {
		shortBytes := make([]byte, length)
		shortB64 := base64.StdEncoding.EncodeToString(shortBytes)

		_, err := Decrypt(shortB64, key)
		if err == nil {
			t.Fatalf("expected ErrCiphertextTooShort for length %d, got nil", length)
		}
		if !errors.Is(err, ErrCiphertextTooShort) {
			t.Fatalf("expected ErrCiphertextTooShort for length %d, got: %v", length, err)
		}
	}
}

func TestGenerateHexKey(t *testing.T) {
	hexKey, err := GenerateHexKey()
	if err != nil {
		t.Fatalf("failed to generate hex key: %v", err)
	}

	if len(hexKey) != HexKeyLength {
		t.Fatalf("expected hex key length %d, got %d", HexKeyLength, len(hexKey))
	}

	key, err := KeyFromHex(hexKey)
	if err != nil {
		t.Fatalf("KeyFromHex failed on generated hex key: %v", err)
	}

	// Verify key can encrypt and decrypt
	ct, err := Encrypt([]byte("test"), key)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if string(pt) != "test" {
		t.Fatalf("expected 'test', got %q", string(pt))
	}
}
