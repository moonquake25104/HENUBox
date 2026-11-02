package ai

import (
	"os"
	"testing"
)

func TestEnsureKeyFileCreatesAndReloads(t *testing.T) {
	dir := t.TempDir()
	key1, err := EnsureKeyFile(dir)
	if err != nil {
		t.Fatalf("ensure key: %v", err)
	}
	key2, err := EnsureKeyFile(dir)
	if err != nil {
		t.Fatalf("ensure key again: %v", err)
	}
	if key1 != key2 {
		t.Fatal("再次调用应复用同一 key 文件")
	}
	if _, err := os.Stat(keyPath(dir)); err != nil {
		t.Fatalf("key file should exist: %v", err)
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	dir := t.TempDir()
	key, err := EnsureKeyFile(dir)
	if err != nil {
		t.Fatalf("ensure key: %v", err)
	}
	plaintext := "sk-1234567890abcdef"
	encrypted, err := EncryptAPIKey(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !containsColon(encrypted) {
		t.Fatal("密文应包含 nonce:cipher 分隔")
	}
	decrypted, err := DecryptAPIKey(encrypted, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestRandomNonceProducesDistinctCiphertext(t *testing.T) {
	dir := t.TempDir()
	key, _ := EnsureKeyFile(dir)
	e1, err := EncryptAPIKey("same-plain", key)
	if err != nil {
		t.Fatalf("encrypt 1: %v", err)
	}
	e2, err := EncryptAPIKey("same-plain", key)
	if err != nil {
		t.Fatalf("encrypt 2: %v", err)
	}
	if e1 == e2 {
		t.Fatal("随机 nonce 下同一明文两次密文应不同")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	dir := t.TempDir()
	key, _ := EnsureKeyFile(dir)
	other := [32]byte{9, 9, 9, 9}
	encrypted, err := EncryptAPIKey("secret", key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := DecryptAPIKey(encrypted, other); err == nil {
		t.Fatal("错误 key 解密应失败")
	}
}

func TestDecryptMalformedFails(t *testing.T) {
	dir := t.TempDir()
	key, _ := EnsureKeyFile(dir)
	if _, err := DecryptAPIKey("not-a-valid-format", key); err == nil {
		t.Fatal("缺少分隔符应失败")
	}
	if _, err := DecryptAPIKey("zzzz:zzzz", key); err == nil {
		t.Fatal("非法 hex 应失败")
	}
}

func TestLoadKeyMissingFails(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadKey(dir); err == nil {
		t.Fatal("无 key 文件时应报错")
	}
}

func TestRecoveryKeyRoundtrip(t *testing.T) {
	var dek [32]byte
	for i := range dek {
		dek[i] = byte(i)
	}
	hexKey := RecoveryKeyHex(dek)
	if len(hexKey) != 64 {
		t.Fatalf("recovery key length = %d, want 64", len(hexKey))
	}
	parsed, err := RecoveryKeyFromHex(hexKey)
	if err != nil {
		t.Fatalf("parse recovery key: %v", err)
	}
	if parsed != dek {
		t.Fatal("recovery key roundtrip mismatch")
	}
	if _, err := RecoveryKeyFromHex("bad"); err == nil {
		t.Fatal("非法恢复密钥应报错")
	}
}

func TestDEKPersistAndLoad(t *testing.T) {
	dir := t.TempDir()
	if dek, err := LoadDEK(dir); err != nil || dek != nil {
		t.Fatalf("empty dir should yield nil DEK, got %v err=%v", dek, err)
	}
	var dek [32]byte
	for i := range dek {
		dek[i] = byte(255 - i)
	}
	if err := SaveDEK(dir, &dek); err != nil {
		t.Fatalf("save DEK: %v", err)
	}
	loaded, err := LoadDEK(dir)
	if err != nil || loaded == nil {
		t.Fatalf("load DEK: %v", err)
	}
	if *loaded != dek {
		t.Fatal("DEK roundtrip mismatch")
	}
}

func containsColon(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return true
		}
	}
	return false
}
