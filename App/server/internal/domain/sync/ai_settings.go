package sync

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/pbkdf2"

	"crypto/sha256"

	"kairos/server/internal/store"
)

const (
	kdfIterations = 210_000
	saltLen       = 16
	nonceLen      = 12
	dekLen        = 32
	formatVersion = 1
)

// aad binds the ciphertext to "this is an AI settings envelope", preventing
// cross-purpose key reuse.
var aad = []byte("kairos-ai-settings")

// AiSettingsBlob is the JSON structure of the remote kairos-ai-settings.enc
// file. All binary fields are hex encoded.
type AiSettingsBlob struct {
	FormatVersion    uint32 `json:"format_version"`
	KdfIterations    uint32 `json:"kdf_iterations"`
	KdfSaltHex       string `json:"kdf_salt_hex"`
	DekWrapNonceHex  string `json:"dek_wrap_nonce_hex"`
	WrappedDekHex    string `json:"wrapped_dek_hex"`
	PayloadNonceHex  string `json:"payload_nonce_hex"`
	PayloadCipherHex string `json:"payload_cipher_hex"`
}

// SyncAiPayload is the plaintext inside the envelope.
type SyncAiPayload struct {
	Enabled   bool   `json:"enabled"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	APIKey    string `json:"api_key"`
	UpdatedAt string `json:"updated_at"`
}

// DecryptedBlob is the result of decrypting an envelope.
type DecryptedBlob struct {
	Payload     SyncAiPayload
	DEK         [32]byte
	NeedsRewrap bool
}

// MergeOutcome is the LWW result for the singleton AI settings payload.
type MergeOutcome int

const (
	RemoteWins MergeOutcome = iota
	LocalWins
)

// EncryptToBlob encrypts the AI settings payload into the remote envelope JSON
// using the production KDF iteration count.
func EncryptToBlob(password string, payload *SyncAiPayload, dek [32]byte) (string, error) {
	return encryptToBlobWithIterations(password, kdfIterations, payload, dek)
}

func encryptToBlobWithIterations(password string, iterations uint32, payload *SyncAiPayload, dek [32]byte) (string, error) {
	salt := randomBytes(saltLen)
	kek := deriveKEK(password, salt, iterations)

	wrapNonce := randomBytes(nonceLen)
	wrappedDEK, err := seal(kek, wrapNonce, dek[:], aad)
	if err != nil {
		return "", err
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("AI 设置 payload 序列化失败: %w", err)
	}
	payloadNonce := randomBytes(nonceLen)
	payloadCipher, err := seal(dek, payloadNonce, payloadJSON, aad)
	if err != nil {
		return "", err
	}

	blob := AiSettingsBlob{
		FormatVersion:    formatVersion,
		KdfIterations:    iterations,
		KdfSaltHex:       hex.EncodeToString(salt),
		DekWrapNonceHex:  hex.EncodeToString(wrapNonce),
		WrappedDekHex:    hex.EncodeToString(wrappedDEK),
		PayloadNonceHex:  hex.EncodeToString(payloadNonce),
		PayloadCipherHex: hex.EncodeToString(payloadCipher),
	}
	out, err := json.Marshal(&blob)
	if err != nil {
		return "", fmt.Errorf("AI 设置加密包序列化失败: %w", err)
	}
	return string(out), nil
}

// DecryptBlob decrypts a remote envelope. The current password unwraps the DEK
// when it matches; otherwise the local DEK copy is used (NeedsRewrap=true).
// When both fail a recoverable error suggests pasting the recovery key.
func DecryptBlob(password, blobJSON string, localDEK *[32]byte) (*DecryptedBlob, error) {
	var blob AiSettingsBlob
	if err := json.Unmarshal([]byte(blobJSON), &blob); err != nil {
		return nil, fmt.Errorf("AI 设置加密包解析失败: %w", err)
	}
	if blob.FormatVersion != formatVersion {
		return nil, fmt.Errorf("AI 设置加密包版本不受支持: %d", blob.FormatVersion)
	}

	salt, err := hex.DecodeString(blob.KdfSaltHex)
	if err != nil {
		return nil, errors.New("加密包盐解码失败")
	}
	kek := deriveKEK(password, salt, blob.KdfIterations)
	wrapNonce, err := decodeNonce(blob.DekWrapNonceHex)
	if err != nil {
		return nil, err
	}
	wrappedDEK, err := hex.DecodeString(blob.WrappedDekHex)
	if err != nil {
		return nil, errors.New("包裹 DEK 解码失败")
	}

	var dek [32]byte
	needsRewrap := false
	if opened, err := open(kek, wrapNonce, wrappedDEK, aad); err == nil {
		if len(opened) != dekLen {
			return nil, errors.New("加密包 DEK 长度异常")
		}
		copy(dek[:], opened)
	} else if localDEK != nil {
		dek = *localDEK
		needsRewrap = true
	} else {
		return nil, errors.New("无法解密 AI 设置：WebDAV 密码不匹配，且本机没有可用的恢复密钥。请在 AI 设置页粘贴恢复密钥后重试")
	}

	payloadNonce, err := decodeNonce(blob.PayloadNonceHex)
	if err != nil {
		return nil, err
	}
	payloadCipher, err := hex.DecodeString(blob.PayloadCipherHex)
	if err != nil {
		return nil, errors.New("payload 密文解码失败")
	}
	payloadJSON, err := open(dek, payloadNonce, payloadCipher, aad)
	if err != nil {
		return nil, errors.New("AI 设置加密包校验失败：密文可能已损坏或恢复密钥不正确")
	}
	var payload SyncAiPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, fmt.Errorf("AI 设置 payload 解析失败: %w", err)
	}
	return &DecryptedBlob{Payload: payload, DEK: dek, NeedsRewrap: needsRewrap}, nil
}

// RecoveryKeyHex renders the DEK as the 64-char hex recovery key.
func RecoveryKeyHex(dek [32]byte) string {
	return hex.EncodeToString(dek[:])
}

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

// PayloadFromConfig builds the plaintext payload from the local AI config row.
func PayloadFromConfig(c store.AiConfig, apiKeyPlaintext string) SyncAiPayload {
	return SyncAiPayload{
		Enabled:   c.Enabled,
		BaseURL:   c.BaseUrl,
		Model:     c.Model,
		APIKey:    apiKeyPlaintext,
		UpdatedAt: store.TSString(c.UpdatedAt),
	}
}

// MergePayload decides the LWW winner for the singleton AI settings: a newer
// remote wins, otherwise local is kept (ties keep local for convergence).
func MergePayload(local *SyncAiPayload, remote *SyncAiPayload) MergeOutcome {
	if local == nil {
		return RemoteWins
	}
	if remote.UpdatedAt > local.UpdatedAt {
		return RemoteWins
	}
	return LocalWins
}

func deriveKEK(password string, salt []byte, iterations uint32) [32]byte {
	var kek [32]byte
	copy(kek[:], pbkdf2.Key([]byte(password), salt, int(iterations), dekLen, sha256.New))
	return kek
}

func seal(key [32]byte, nonce, msg, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, errors.New("初始化 AES-256-GCM 失败")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("初始化 AES-256-GCM 失败")
	}
	return gcm.Seal(nil, nonce, msg, aad), nil
}

func open(key [32]byte, nonce, ciphertext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, errors.New("初始化 AES-256-GCM 失败")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("初始化 AES-256-GCM 失败")
	}
	return gcm.Open(nil, nonce, ciphertext, aad)
}

func decodeNonce(hexStr string) ([]byte, error) {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, errors.New("加密包 nonce 解码失败")
	}
	if len(b) != nonceLen {
		return nil, errors.New("加密包 nonce 长度无效")
	}
	return b, nil
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}
