package crypto

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	// Initialize encryption with a test key
	if err := InitializeEncryption("test-encryption-key-for-momentum"); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}
	
	tests := []struct {
		name      string
		plaintext string
	}{
		{"simple string", "hello world"},
		{"empty string", ""},
		{"special characters", "!@#$%^&*()_+-={}[]|:;<>?,./"},
		{"unicode", "Hello 世界 🌍"},
		{"long string", "This is a longer test string with multiple words and sentences. It should still encrypt and decrypt correctly."},
		{"password", "MySecureP@ssw0rd123!"},
		{"url", "https://caldav.example.com/dav/calendars/user@example.com/tasks"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt
			ciphertext, err := Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encryption failed: %v", err)
			}
			
			// Verify ciphertext is different from plaintext (unless empty)
			if tt.plaintext != "" && ciphertext == tt.plaintext {
				t.Error("Ciphertext should be different from plaintext")
			}
			
			// Decrypt
			decrypted, err := Decrypt(ciphertext)
			if err != nil {
				t.Fatalf("Decryption failed: %v", err)
			}
			
			// Verify decrypted matches original
			if decrypted != tt.plaintext {
				t.Errorf("Decrypted text does not match original. Got %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptionWithoutKey(t *testing.T) {
	// Reset encryption key
	encryptionKey = nil
	
	_, err := Encrypt("test")
	if err != ErrEncryptionKeyNotSet {
		t.Errorf("Expected ErrEncryptionKeyNotSet, got %v", err)
	}
	
	_, err = Decrypt("test")
	if err != ErrEncryptionKeyNotSet {
		t.Errorf("Expected ErrEncryptionKeyNotSet, got %v", err)
	}
}

func TestDecryptInvalidCiphertext(t *testing.T) {
	if err := InitializeEncryption("test-key"); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}
	
	tests := []struct {
		name       string
		ciphertext string
	}{
		{"invalid base64", "not-valid-base64!!!"},
		{"too short", "YWJj"}, // Valid base64 but too short for GCM
		{"empty", ""},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decrypt(tt.ciphertext)
			if err == nil {
				t.Error("Expected error for invalid ciphertext")
			}
		})
	}
}

func TestEncryptionDeterminism(t *testing.T) {
	if err := InitializeEncryption("test-key"); err != nil {
		t.Fatalf("Failed to initialize encryption: %v", err)
	}
	
	plaintext := "test message"
	
	// Encrypt the same plaintext twice
	ciphertext1, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("First encryption failed: %v", err)
	}
	
	ciphertext2, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Second encryption failed: %v", err)
	}
	
	// Ciphertexts should be different due to random nonce
	if ciphertext1 == ciphertext2 {
		t.Error("Encrypting the same plaintext twice should produce different ciphertexts")
	}
	
	// But both should decrypt to the same plaintext
	decrypted1, err := Decrypt(ciphertext1)
	if err != nil {
		t.Fatalf("First decryption failed: %v", err)
	}
	
	decrypted2, err := Decrypt(ciphertext2)
	if err != nil {
		t.Fatalf("Second decryption failed: %v", err)
	}
	
	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Error("Both decryptions should produce the original plaintext")
	}
}

func TestInitializeEncryption(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid key", "valid-encryption-key", false},
		{"empty key", "", true},
		{"short key", "abc", false}, // Short keys are hashed to 32 bytes
		{"long key", "this-is-a-very-long-encryption-key-with-many-characters", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InitializeEncryption(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("InitializeEncryption() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
