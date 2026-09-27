package secretbox

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

const keySize = 32

type Box struct {
	aead cipher.AEAD
}

func NewFromHex(value string) (*Box, error) {
	if value == "" {
		return nil, nil
	}
	key, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode master key: %w", err)
	}
	if len(key) != keySize {
		return nil, fmt.Errorf("master key must be %d bytes (%d hex characters)", keySize, keySize*2)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	return &Box{aead: aead}, nil
}

func (b *Box) Seal(plaintext []byte) (ciphertext, nonce []byte, err error) {
	if b == nil {
		return nil, nil, errors.New("secret encryption is disabled")
	}
	nonce = make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext = b.aead.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func (b *Box) Open(ciphertext, nonce []byte) ([]byte, error) {
	if b == nil {
		return nil, errors.New("secret encryption is disabled")
	}
	if len(nonce) != b.aead.NonceSize() {
		return nil, errors.New("invalid secret nonce")
	}
	plaintext, err := b.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	return plaintext, nil
}

func (b *Box) SealURL(plaintext []byte) (string, error) {
	ciphertext, nonce, err := b.Seal(plaintext)
	if err != nil {
		return "", err
	}
	payload := make([]byte, 0, len(nonce)+len(ciphertext))
	payload = append(payload, nonce...)
	payload = append(payload, ciphertext...)
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func (b *Box) OpenURL(value string) ([]byte, error) {
	if b == nil {
		return nil, errors.New("secret encryption is disabled")
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted token: %w", err)
	}
	nonceSize := b.aead.NonceSize()
	if len(payload) <= nonceSize {
		return nil, errors.New("invalid encrypted token")
	}
	return b.Open(payload[nonceSize:], payload[:nonceSize])
}
