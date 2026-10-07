package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/adrianozp/platform/pkg/config"
)

var (
	ErrKeySize    = errors.New("secretbox: key must be 32 bytes")
	ErrUnknownKey = errors.New("secretbox: ciphertext was sealed with an unknown key")
)

type Box struct {
	keyID string
	aead  cipher.AEAD
}

func New(cfg config.Config) (*Box, error) {
	key, err := base64.StdEncoding.DecodeString(cfg.Secrets.Key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: decode key: %w", err)
	}
	return NewWithKey(cfg.Secrets.KeyID, key)
}

func NewWithKey(keyID string, key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, ErrKeySize
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{keyID: keyID, aead: aead}, nil
}

func (b *Box) Seal(plaintext, additionalData []byte) (ciphertext, nonce []byte, keyID string, err error) {
	nonce = make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, "", err
	}
	return b.aead.Seal(nil, nonce, plaintext, additionalData), nonce, b.keyID, nil
}

func (b *Box) Open(ciphertext, nonce, additionalData []byte, keyID string) ([]byte, error) {
	if keyID != b.keyID {
		return nil, ErrUnknownKey
	}
	return b.aead.Open(nil, nonce, ciphertext, additionalData)
}
