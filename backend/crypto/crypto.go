package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

var globalKey []byte

// Init loads or generates the master key.
func Init(keyPath, keyEnv string) error {
	if keyEnv != "" {
		if val := os.Getenv(keyEnv); val != "" {
			key, err := base64.StdEncoding.DecodeString(val)
			if err != nil {
				return fmt.Errorf("invalid MASTER_KEY env var: %w", err)
			}
			if len(key) != 32 {
				return fmt.Errorf("MASTER_KEY must be 32 bytes (got %d)", len(key))
			}
			globalKey = key
			return nil
		}
	}

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		key := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			return fmt.Errorf("generate master key: %w", err)
		}
		encoded := base64.StdEncoding.EncodeToString(key)
		if err := os.WriteFile(keyPath, []byte(encoded), 0600); err != nil {
			return fmt.Errorf("write master key: %w", err)
		}
		globalKey = key
		return nil
	}

	data, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read master key: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return fmt.Errorf("decode master key: %w", err)
	}
	if len(key) != 32 {
		return fmt.Errorf("master key must be 32 bytes (got %d)", len(key))
	}
	globalKey = key
	return nil
}

// Encrypt encrypts plaintext with AES-256-GCM, returns base64(nonce+ciphertext).
func Encrypt(plaintext string) (string, error) {
	if globalKey == nil {
		return "", fmt.Errorf("crypto not initialized")
	}
	block, err := aes.NewCipher(globalKey)
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

// Decrypt decrypts a base64(nonce+ciphertext) produced by Encrypt.
func Decrypt(encoded string) (string, error) {
	if globalKey == nil {
		return "", fmt.Errorf("crypto not initialized")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(globalKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
