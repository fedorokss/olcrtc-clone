package crypto

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"

	"golang.org/x/crypto/chacha20poly1305"
)

const WireOverhead = chacha20poly1305.NonceSizeX + chacha20poly1305.Overhead

var (
	ErrInvalidKeySize     = errors.New("invalid key size")
	ErrCiphertextTooShort = errors.New("ciphertext too short")
)

const (
	nonceSize     = chacha20poly1305.NonceSizeX
	nonceSaltSize = nonceSize - 8
	aeadOverhead  = chacha20poly1305.Overhead
)

type Cipher struct {
	aead    cipher.AEAD
	counter atomic.Uint64
	salt    [nonceSaltSize]byte
}

func NewCipher(keyStr string) (*Cipher, error) {
	if len(keyStr) != chacha20poly1305.KeySize {
		return nil, ErrInvalidKeySize
	}
	aead, err := chacha20poly1305.NewX([]byte(keyStr))
	if err != nil {
		return nil, fmt.Errorf("failed to create aead: %w", err)
	}
	c := &Cipher{aead: aead}
	if _, err := rand.Read(c.salt[:]); err != nil {
		return nil, fmt.Errorf("failed to seed nonce salt: %w", err)
	}
	return c, nil
}

func (c *Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	out := make([]byte, nonceSize, nonceSize+len(plaintext)+aeadOverhead)
	copy(out, c.salt[:])
	binary.BigEndian.PutUint64(out[nonceSaltSize:nonceSize], c.counter.Add(1))
	return c.aead.Seal(out, out[:nonceSize], plaintext, nil), nil
}

func (c *Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	return c.DecryptInto(nil, ciphertext)
}

func (c *Cipher) DecryptInto(dst, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < nonceSize {
		return nil, ErrCiphertextTooShort
	}
	res, err := c.aead.Open(dst, ciphertext[:nonceSize], ciphertext[nonceSize:], nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}
	return res, nil
}
