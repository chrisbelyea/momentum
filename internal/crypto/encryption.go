package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

var (
	// ErrInvalidCiphertext is returned when the ciphertext is invalid
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	// ErrEncryptionKeyNotSet is returned when the encryption key is not configured
	ErrEncryptionKeyNotSet = errors.New("encryption key not set")
)

// EncryptionKey holds the encryption key for the application
// In production, this should be loaded from a secure location (environment variable, key management service)
var encryptionKey []byte

// InitializeEncryption initializes the encryption system with a key
// The key should be 32 bytes for AES-256
func InitializeEncryption(key string) error {
	if key == "" {
		return ErrEncryptionKeyNotSet
	}
	
	// Derive a 32-byte key from the provided string using SHA-256
	hash := sha256.Sum256([]byte(key))
	encryptionKey = hash[:]
	
	return nil
}

// LoadEncryptionKeyFromEnv loads the encryption key from environment variable
func LoadEncryptionKeyFromEnv() error {
	key := os.Getenv("MOMENTUM_ENCRYPTION_KEY")
	if key == "" {
		return ErrEncryptionKeyNotSet
	}
	return InitializeEncryption(key)
}

// Encrypt encrypts plaintext using AES-256-GCM
func Encrypt(plaintext string) (string, error) {
	if len(encryptionKey) == 0 {
		return "", ErrEncryptionKeyNotSet
	}
	
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts ciphertext using AES-256-GCM
func Decrypt(ciphertext string) (string, error) {
	if len(encryptionKey) == 0 {
		return "", ErrEncryptionKeyNotSet
	}
	
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}
	
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", ErrInvalidCiphertext
	}
	
	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}
	
	return string(plaintext), nil
}
