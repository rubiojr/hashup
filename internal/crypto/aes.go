package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
)

type aesMachine struct {
	key []byte
}

// NewEncryptor creates a new Encryptor instance with the given key
func NewAES(key []byte) (Machine, error) {
	// Derive a 32-byte key for AES-256 from the encryption key
	hasher := sha256.New()
	hasher.Write(key)
	ek := hasher.Sum(nil)
	if len(ek) != 32 {
		return nil, fmt.Errorf("invalid key length")
	}
	return &aesMachine{key: ek}, nil
}

// encrypt uses AES-GCM to encrypt data with the processor's key
func (e *aesMachine) Encrypt(data []byte) ([]byte, error) {
	// Create a new cipher block from the key
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	// Create a new GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Create a nonce (12 bytes for GCM)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Encrypt and seal the data
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// decrypt uses AES-GCM to decrypt data with the processor's key
func (e *aesMachine) Decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	// Create a new GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Get the nonce size
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract the nonce and ciphertext
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]

	// Decrypt the data
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
