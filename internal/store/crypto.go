package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

const plainPrefix = "PLAIN1:"

func tokenKey() ([]byte, bool) {
	h := os.Getenv("PE_TOKEN_KEY")
	if len(h) == 64 {
		b, err := hex.DecodeString(h)
		if err == nil && len(b) == 32 {
			return b, true
		}
	}
	if len(h) >= 32 {
		k := make([]byte, 32)
		copy(k, h[:32])
		return k, true
	}
	return nil, false
}

func encryptToken(plain string) ([]byte, error) {
	key, ok := tokenKey()
	if !ok {
		return append([]byte(plainPrefix), []byte(plain)...), nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plain), nil), nil
}

func decryptToken(blob []byte) (string, error) {
	if len(blob) > len(plainPrefix) && string(blob[:len(plainPrefix)]) == plainPrefix {
		return string(blob[len(plainPrefix):]), nil
	}
	key, ok := tokenKey()
	if !ok {
		return string(blob), nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(blob) < gcm.NonceSize() {
		return "", errors.New("token blob too short")
	}
	nonce, ct := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	out, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
