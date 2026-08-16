package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
)

const (
	gcmNonceSize = 12
	aesKeySize   = 32
)

var (
	ErrKeyUnavailable = errors.New("secret encryption key is unavailable")
	ErrInvalidCipher  = errors.New("ciphertext is invalid")
)

type Cipher struct {
	key []byte
}

func NewCipher(base64Key string) (*Cipher, error) {
	trimmed := base64Key
	if trimmed == "" {
		return &Cipher{}, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("MILETOS_SECRETS_AES_KEY must be valid base64: %w", err)
	}
	if len(decoded) != aesKeySize {
		return nil, fmt.Errorf("MILETOS_SECRETS_AES_KEY must decode to exactly %d bytes", aesKeySize)
	}
	return &Cipher{key: decoded}, nil
}

func (secretCipher *Cipher) Configured() bool {
	return secretCipher != nil && len(secretCipher.key) == aesKeySize
}

func (secretCipher *Cipher) Decrypt(encoded string) (string, error) {
	if !secretCipher.Configured() {
		return "", ErrKeyUnavailable
	}
	if encoded == "" {
		return "", ErrInvalidCipher
	}
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrInvalidCipher
	}
	if len(payload) <= gcmNonceSize {
		return "", ErrInvalidCipher
	}
	block, err := aes.NewCipher(secretCipher.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := payload[:gcmNonceSize]
	ciphertext := payload[gcmNonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrInvalidCipher
	}
	return string(plaintext), nil
}
