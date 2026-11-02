package sync

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	dekFileName     = "ai_sync_dek"
	aiKeyFileName   = ".ai_encryption_key"
	localKeyFileLen = 32
)

func dekPath(dir string) string {
	return filepath.Join(dir, dekFileName)
}

func aiKeyPath(dir string) string {
	return filepath.Join(dir, aiKeyFileName)
}

// loadDEK reads the local DEK copy; nil when it does not exist yet.
func loadDEK(dir string) (*[32]byte, error) {
	b, err := os.ReadFile(dekPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) != dekLen {
		return nil, errors.New("AI 同步 DEK 文件长度异常，请删除该文件后重新同步")
	}
	var dek [32]byte
	copy(dek[:], b)
	return &dek, nil
}

// loadOrCreateDEK returns the local DEK, generating and persisting a fresh one
// on first use.
func loadOrCreateDEK(dir string) ([32]byte, error) {
	if dek, err := loadDEK(dir); err != nil {
		return [32]byte{}, err
	} else if dek != nil {
		return *dek, nil
	}
	var dek [32]byte
	if _, err := rand.Read(dek[:]); err != nil {
		return [32]byte{}, err
	}
	if err := saveDEK(dir, &dek); err != nil {
		return [32]byte{}, err
	}
	return dek, nil
}

func saveDEK(dir string, dek *[32]byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(dekPath(dir), dek[:], 0o600)
}

// ensureKeyFile returns the local API-key encryption key, creating it on first
// use.
func ensureKeyFile(dir string) ([32]byte, error) {
	b, err := os.ReadFile(aiKeyPath(dir))
	if err == nil {
		if len(b) != localKeyFileLen {
			return [32]byte{}, errors.New("AI key 文件长度异常，请删除后重新配置")
		}
		var key [32]byte
		copy(key[:], b)
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return [32]byte{}, err
	}
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return [32]byte{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return [32]byte{}, err
	}
	if err := os.WriteFile(aiKeyPath(dir), key[:], 0o600); err != nil {
		return [32]byte{}, err
	}
	return key, nil
}

// encryptAPIKey seals the plaintext API key for local storage as
// hex(nonce):hex(ciphertext).
func encryptAPIKey(plaintext string, key [32]byte) (string, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(nonce) + ":" + hex.EncodeToString(sealed), nil
}

// decryptAPIKey opens a hex(nonce):hex(ciphertext) API key ciphertext.
func decryptAPIKey(encrypted string, key [32]byte) (string, error) {
	ivHex, cipherHex, ok := strings.Cut(encrypted, ":")
	if !ok {
		return "", errors.New("AI key 密文格式无效（缺少 ':' 分隔）")
	}
	nonce, err := hex.DecodeString(ivHex)
	if err != nil {
		return "", errors.New("AI key IV 解码失败")
	}
	ciphertext, err := hex.DecodeString(cipherHex)
	if err != nil {
		return "", errors.New("AI key 密文解码失败")
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("AI key 解密失败（key 文件或密文可能已损坏）")
	}
	return string(plaintext), nil
}
