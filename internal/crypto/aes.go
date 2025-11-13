package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()-_=+[]{}|;:,.<>?")

// GenerateString returns a random alpha-numeric string of length n.
func GenerateString(n int) (string, error) {
	if n <= 0 {
		return "", errors.New("length must be positive")
	}
	b := make([]rune, n)
	buf := make([]byte, len(b))
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = letters[int(buf[i])%len(letters)]
	}
	return string(b), nil
}

// EncryptToBase64 encrypts data with AES-CBC and returns base64 ciphertext.
func EncryptToBase64(plaintext []byte, key []byte, iv []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return "", errors.New("key must be 16, 24 or 32 bytes")
	}
	if len(iv) != aes.BlockSize {
		return "", errors.New("iv must be 16 bytes")
	}
	plaintext = pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(plaintext))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, plaintext)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padtext := bytesRepeat(byte(padding), padding)
	return append(data, padtext...)
}

func bytesRepeat(b byte, count int) []byte {
	out := make([]byte, count)
	for i := range out {
		out[i] = b
	}
	return out
}
