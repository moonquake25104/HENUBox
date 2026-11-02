package sync

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPassword = "webdav-secret"
const testIterations = 1_000

func samplePayload(updatedAt string) SyncAiPayload {
	return SyncAiPayload{
		Enabled:   true,
		BaseURL:   "https://api.deepseek.com",
		Model:     "deepseek-v4-flash",
		APIKey:    "sk-test-123",
		UpdatedAt: updatedAt,
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	payload := samplePayload("2026-08-01T00:00:00Z")
	dek := randomBytes(dekLen)
	var dekArr [32]byte
	copy(dekArr[:], dek)

	blob, err := encryptToBlobWithIterations(testPassword, testIterations, &payload, dekArr)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	decrypted, err := DecryptBlob(testPassword, blob, nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted.NeedsRewrap {
		t.Fatal("needs_rewrap should be false with the right password")
	}
	if decrypted.DEK != dekArr {
		t.Fatal("decrypted DEK mismatch")
	}
	if decrypted.Payload != payload {
		t.Fatalf("payload mismatch: %+v vs %+v", decrypted.Payload, payload)
	}
}

func TestBlobIsNotPlaintext(t *testing.T) {
	payload := samplePayload("2026-08-01T00:00:00Z")
	dek := randomBytes(dekLen)
	var dekArr [32]byte
	copy(dekArr[:], dek)

	blob, err := encryptToBlobWithIterations(testPassword, testIterations, &payload, dekArr)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	for _, secret := range []string{"sk-test-123", "api.deepseek.com", "deepseek-v4-flash"} {
		if strings.Contains(blob, secret) {
			t.Fatalf("blob leaks %q", secret)
		}
	}
}

func TestDecryptWrongPasswordWithoutLocalDEKFails(t *testing.T) {
	payload := samplePayload("2026-08-01T00:00:00Z")
	dek := randomBytes(dekLen)
	var dekArr [32]byte
	copy(dekArr[:], dek)

	blob, err := encryptToBlobWithIterations(testPassword, testIterations, &payload, dekArr)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	_, err = DecryptBlob("wrong-password", blob, nil)
	if err == nil {
		t.Fatal("decrypt should fail without local DEK")
	}
	if !strings.Contains(err.Error(), "恢复密钥") {
		t.Fatalf("error should suggest recovery key, got %q", err.Error())
	}
}

func TestDecryptWrongPasswordWithLocalDEKFallsBack(t *testing.T) {
	payload := samplePayload("2026-08-01T00:00:00Z")
	dek := randomBytes(dekLen)
	var dekArr [32]byte
	copy(dekArr[:], dek)

	blob, err := encryptToBlobWithIterations(testPassword, testIterations, &payload, dekArr)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	decrypted, err := DecryptBlob("new-password", blob, &dekArr)
	if err != nil {
		t.Fatalf("fallback decrypt: %v", err)
	}
	if !decrypted.NeedsRewrap {
		t.Fatal("needs_rewrap should be true after password change")
	}
	if decrypted.Payload.APIKey != "sk-test-123" {
		t.Fatalf("api_key = %q", decrypted.Payload.APIKey)
	}
}

func TestDecryptWrongPasswordAndWrongLocalDEKFails(t *testing.T) {
	payload := samplePayload("2026-08-01T00:00:00Z")
	dek := randomBytes(dekLen)
	var dekArr [32]byte
	copy(dekArr[:], dek)
	wrongDEK := randomBytes(dekLen)
	var wrongArr [32]byte
	copy(wrongArr[:], wrongDEK)

	blob, err := encryptToBlobWithIterations(testPassword, testIterations, &payload, dekArr)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	_, err = DecryptBlob("new-password", blob, &wrongArr)
	if err == nil {
		t.Fatal("decrypt should fail with wrong password and wrong DEK")
	}
	if !strings.Contains(err.Error(), "校验失败") {
		t.Fatalf("error should mention auth failure, got %q", err.Error())
	}
}

func TestTamperedPayloadFailsAuth(t *testing.T) {
	payload := samplePayload("2026-08-01T00:00:00Z")
	dek := randomBytes(dekLen)
	var dekArr [32]byte
	copy(dekArr[:], dek)

	blob, err := encryptToBlobWithIterations(testPassword, testIterations, &payload, dekArr)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	var parsed AiSettingsBlob
	if err := json.Unmarshal([]byte(blob), &parsed); err != nil {
		t.Fatalf("parse blob: %v", err)
	}
	payloadCipher, err := hex.DecodeString(parsed.PayloadCipherHex)
	if err != nil {
		t.Fatalf("decode payload cipher: %v", err)
	}
	payloadCipher[0] ^= 0x01
	parsed.PayloadCipherHex = hex.EncodeToString(payloadCipher)
	tampered, err := json.Marshal(&parsed)
	if err != nil {
		t.Fatalf("marshal tampered: %v", err)
	}

	if _, err := DecryptBlob(testPassword, string(tampered), &dekArr); err == nil {
		t.Fatal("tampered payload should fail auth")
	}
}

func TestMergeRemoteNewerWins(t *testing.T) {
	local := samplePayload("2026-08-01T00:00:00Z")
	remote := samplePayload("2026-08-02T00:00:00Z")
	remote.BaseURL = "https://new.example.com"

	if MergePayload(&local, &remote) != RemoteWins {
		t.Fatal("newer remote should win")
	}
}

func TestMergeLocalNewerOrTieKeepsLocal(t *testing.T) {
	local := samplePayload("2026-08-02T00:00:00Z")
	olderRemote := samplePayload("2026-08-01T00:00:00Z")
	if MergePayload(&local, &olderRemote) != LocalWins {
		t.Fatal("older remote should lose")
	}
	tieRemote := samplePayload("2026-08-02T00:00:00Z")
	if MergePayload(&local, &tieRemote) != LocalWins {
		t.Fatal("tie should keep local for convergence")
	}
}

func TestMergeNoLocalTakesRemote(t *testing.T) {
	remote := samplePayload("2026-08-01T00:00:00Z")
	if MergePayload(nil, &remote) != RemoteWins {
		t.Fatal("no local should take remote")
	}
}

func TestRecoveryKeyRoundtrip(t *testing.T) {
	dek := randomBytes(dekLen)
	var dekArr [32]byte
	copy(dekArr[:], dek)

	hexKey := RecoveryKeyHex(dekArr)
	if len(hexKey) != 64 {
		t.Fatalf("recovery key length = %d, want 64", len(hexKey))
	}
	parsed, err := RecoveryKeyFromHex(hexKey)
	if err != nil {
		t.Fatalf("parse recovery key: %v", err)
	}
	if parsed != dekArr {
		t.Fatal("recovery key roundtrip mismatch")
	}
	if _, err := RecoveryKeyFromHex("zzzz"); err == nil {
		t.Fatal("invalid hex should fail")
	}
	if _, err := RecoveryKeyFromHex("abcd"); err == nil {
		t.Fatal("short key should fail")
	}
}

func TestDEKFileRoundtrip(t *testing.T) {
	dir := t.TempDir()
	dek := randomBytes(dekLen)
	var dekArr [32]byte
	copy(dekArr[:], dek)

	if got, err := loadDEK(dir); err != nil || got != nil {
		t.Fatalf("loadDEK on missing file = %v, %v", got, err)
	}
	if err := saveDEK(dir, &dekArr); err != nil {
		t.Fatalf("saveDEK: %v", err)
	}
	got, err := loadDEK(dir)
	if err != nil || got == nil || *got != dekArr {
		t.Fatalf("loadDEK after save = %v, %v", got, err)
	}
	created, err := loadOrCreateDEK(dir)
	if err != nil || created != dekArr {
		t.Fatalf("loadOrCreateDEK should reuse existing: %v, %v", created, err)
	}
}

func TestLocalAPIKeyEncryptionRoundtrip(t *testing.T) {
	dir := t.TempDir()
	key, err := ensureKeyFile(dir)
	if err != nil {
		t.Fatalf("ensureKeyFile: %v", err)
	}
	encrypted, err := encryptAPIKey("sk-1234567890abcdef", key)
	if err != nil {
		t.Fatalf("encryptAPIKey: %v", err)
	}
	decrypted, err := decryptAPIKey(encrypted, key)
	if err != nil {
		t.Fatalf("decryptAPIKey: %v", err)
	}
	if decrypted != "sk-1234567890abcdef" {
		t.Fatalf("roundtrip mismatch: %q", decrypted)
	}
	if _, err := decryptAPIKey("not-a-valid-format", key); err == nil {
		t.Fatal("malformed ciphertext should fail")
	}
}

func TestDEKFilePermissions(t *testing.T) {
	dir := t.TempDir()
	dek := randomBytes(dekLen)
	var dekArr [32]byte
	copy(dekArr[:], dek)
	if err := saveDEK(dir, &dekArr); err != nil {
		t.Fatalf("saveDEK: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, dekFileName))
	if err != nil {
		t.Fatalf("stat DEK file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("DEK file mode = %o, want 600", info.Mode().Perm())
	}
}
