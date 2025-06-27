package database

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

var aesgcm cipher.AEAD

func InitCrypto(keyString string) error {
	if keyString == "" {
		return fmt.Errorf("encryption key is empty")
	}
	key, err := hex.DecodeString(keyString)
	if err != nil {
		return fmt.Errorf("failed to decode encryption key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	aesgcm, err = cipher.NewGCM(block)
	if err != nil {
		return err
	}

	return nil
}

func EncryptToken(plaintext string) (string, error) {
	if aesgcm == nil {
		return "", fmt.Errorf("crypto not initialized")
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aesgcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func DecryptToken(ciphertext string) (string, error) {
	if aesgcm == nil {
		return "", fmt.Errorf("crypto not initialized")
	}

	decodedCipher, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	nonceSize := aesgcm.NonceSize()
	if len(decodedCipher) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, encryptedMessage := decodedCipher[:nonceSize], decodedCipher[nonceSize:]
	plaintext, err := aesgcm.Open(nil, nonce, encryptedMessage, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
