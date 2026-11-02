package ai

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	keyFileName  = ".ai_encryption_key"
	dekFileName  = "ai_sync_dek"
	keyLen       = 32
	dekLen       = 32
	keyFilePerms = 0o600
	dirPerms     = 0o700
)

func keyPath(dir string) string { return filepath.Join(dir, keyFileName) }

func dekPath(dir string) string { return filepath.Join(dir, dekFileName) }

// EnsureKeyFile returns the local API-key encryption key, generating and
// persisting a fresh random key on first use. The file name and AES-256-GCM
// layout match the sync package so the encrypted ai_config.api_key_encrypted
// column is interchangeable between the two.
func EnsureKeyFile(dir string) ([32]byte, error) {
	var key [32]byte
	b, err := os.ReadFile(keyPath(dir))
	if err == nil {
		if len(b) != keyLen {
			return key, errors.New("AI key 文件长度异常，请删除后重新配置")
		}
		copy(key[:], b)
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return key, err
	}
	if _, err := rand.Read(key[:]); err != nil {
		return key, err
	}
	if err := os.MkdirAll(dir, dirPerms); err != nil {
		return key, err
	}
	if err := os.WriteFile(keyPath(dir), key[:], keyFilePerms); err != nil {
		return key, err
	}
	return key, nil
}

// LoadKey reads an existing key file. A missing file is an error, mirroring the
// Rust load_key contract (the caller must configure the key first).
func LoadKey(dir string) ([32]byte, error) {
	var key [32]byte
	b, err := os.ReadFile(keyPath(dir))
	if err != nil {
		return key, fmt.Errorf("读取 AI key 文件失败: %w", err)
	}
	if len(b) != keyLen {
		return key, errors.New("AI key 文件长度异常，请删除后重新配置")
	}
	copy(key[:], b)
	return key, nil
}

// EncryptAPIKey seals the plaintext API key as hex(nonce):hex(ciphertext).
func EncryptAPIKey(plaintext string, key [32]byte) (string, error) {
	gcm, err := newGCM(key)
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

// DecryptAPIKey opens a hex(nonce):hex(ciphertext) API-key ciphertext.
func DecryptAPIKey(encrypted string, key [32]byte) (string, error) {
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
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("AI key 解密失败（key 文件或密文可能已损坏）")
	}
	return string(plaintext), nil
}

// LoadDEK reads the local recovery-key (DEK) copy, returning nil when the file
// does not exist yet. The path matches the sync package's ai_sync_dek file.
func LoadDEK(dir string) (*[32]byte, error) {
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

// SaveDEK persists a recovery-key (DEK) copy.
func SaveDEK(dir string, dek *[32]byte) error {
	if err := os.MkdirAll(dir, dirPerms); err != nil {
		return err
	}
	return os.WriteFile(dekPath(dir), dek[:], keyFilePerms)
}

// RecoveryKeyHex renders the DEK as the 64-char hex recovery key.
func RecoveryKeyHex(dek [32]byte) string { return hex.EncodeToString(dek[:]) }

// RecoveryKeyFromHex parses a recovery key back into the DEK.
func RecoveryKeyFromHex(input string) ([32]byte, error) {
	var dek [32]byte
	b, err := hex.DecodeString(strings.TrimSpace(input))
	if err != nil {
		return dek, errors.New("恢复密钥格式无效：应为 64 位十六进制字符")
	}
	if len(b) != dekLen {
		return dek, errors.New("恢复密钥长度无效：应为 32 字节")
	}
	copy(dek[:], b)
	return dek, nil
}

func newGCM(key [32]byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, errors.New("初始化 AES-256-GCM 失败")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("初始化 AES-256-GCM 失败")
	}
	return gcm, nil
}
