package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	encryptionKey     []byte
	encryptionKeyOnce sync.Once
)

// getEncryptionKey returns the encryption key, deriving it from ENCRYPTION_KEY env var
// or generating a default one based on a machine-specific seed
func getEncryptionKey() ([]byte, error) {
	var keyErr error
	encryptionKeyOnce.Do(func() {
		keyStr := os.Getenv("ENCRYPTION_KEY")
		if keyStr == "" {
			keyErr = fmt.Errorf("ENCRYPTION_KEY environment variable must be set for production use")
			return
		}
		hash := sha256.Sum256([]byte(keyStr))
		encryptionKey = hash[:]
	})
	if encryptionKey == nil || keyErr != nil {
		return nil, keyErr
	}
	return encryptionKey, nil
}

// Encrypt encrypts plaintext using AES-256-GCM and returns base64-encoded ciphertext
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	key, err := getEncryptionKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts base64-encoded ciphertext using AES-256-GCM
func Decrypt(ciphertextB64 string) (string, error) {
	if ciphertextB64 == "" {
		return "", nil
	}

	key, err := getEncryptionKey()
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		// If it's not base64, it might be a legacy unencrypted key
		return ciphertextB64, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		// Too short to be encrypted, return as-is (legacy unencrypted key)
		return ciphertextB64, nil
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// Decryption failed - might be a legacy unencrypted key
		return ciphertextB64, nil
	}

	return string(plaintext), nil
}

// IsEncrypted checks if a string appears to be encrypted (base64 encoded with proper length)
func IsEncrypted(s string) bool {
	if s == "" {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return false
	}
	// GCM nonce is 12 bytes, tag is 16 bytes, so minimum length is 28 + some data
	return len(decoded) >= 28
}

// MigrateAPIKey encrypts an API key if it's not already encrypted
func MigrateAPIKey(apiKey string) (string, error) {
	if apiKey == "" {
		return "", nil
	}

	// Check if already encrypted by trying to decrypt
	if IsEncrypted(apiKey) {
		// Try decrypting to verify
		decrypted, err := Decrypt(apiKey)
		if err == nil && decrypted != apiKey {
			// Successfully decrypted, it was encrypted
			return apiKey, nil
		}
	}

	// Not encrypted, encrypt it
	return Encrypt(apiKey)
}
