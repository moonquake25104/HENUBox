// Package schoolconn implements the HENU direct-connection pieces: session
// cryptography, the fixed-endpoint school client and the interactive login
// helper manager. Plaintext school credentials only ever live in process
// memory here; the database stores AES-256-GCM envelopes keyed by a file that
// never leaves the private config directory.
package schoolconn

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// envelopeVersion is the first byte of every stored ciphertext; a future
	// format change bumps it and rejects old envelopes explicitly.
	envelopeVersion = 1
	nonceSize       = 12
	keySize         = 32
)

// ErrSessionUnreadable reports that a ciphertext exists but cannot be
// decrypted with the configured key (missing, replaced or corrupted). Callers
// must treat it as "re-authorization required", never as "no session" and
// never regenerate the key silently.
var ErrSessionUnreadable = errors.New("school session ciphertext cannot be decrypted with the configured key")

// SessionKey loads or lazily creates the 32-byte AES key at keyPath. create
// controls whether a missing file may be created (first authorization); with
// create=false a missing file yields os.ErrNotExist so callers can decide.
func LoadSessionKey(keyPath string, create bool) ([]byte, error) {
	key, err := os.ReadFile(keyPath)
	if err == nil {
		if len(key) != keySize {
			return nil, fmt.Errorf("key file %s has %d bytes, want %d", keyPath, len(key), keySize)
		}
		return key, nil
	}
	if !os.IsNotExist(err) || !create {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, err
	}
	key = make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	// 0600 is enforced on POSIX; on Windows the ACL comes from the private
	// containing directory. Deployment notes should document this boundary.
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// Seal encrypts plaintext with AES-256-GCM, binding the ciphertext to the
// owner, school and connection generation so a blob copied across rows or
// generations fails to open.
func Seal(key []byte, plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+len(nonce)+len(plaintext)+gcm.Overhead())
	out = append(out, envelopeVersion)
	// Little-endian nonce length keeps the envelope self-describing without
	// trusting the surrounding row shape.
	var lenBuf [2]byte
	binary.LittleEndian.PutUint16(lenBuf[:], uint16(len(nonce)))
	out = append(out, lenBuf[:]...)
	out = append(out, nonce...)
	out = gcm.Seal(out, nonce, plaintext, aad)
	return out, nil
}

// Open decrypts a Seal envelope. Wrong key, tampering or AAD mismatch all
// collapse into ErrSessionUnreadable so callers cannot distinguish them and
// are never tempted to treat a failure as "logged out cleanly".
func Open(key, ciphertext, aad []byte) ([]byte, error) {
	if len(ciphertext) < 1+2+nonceSize {
		return nil, ErrSessionUnreadable
	}
	if ciphertext[0] != envelopeVersion {
		return nil, ErrSessionUnreadable
	}
	nonceLen := int(binary.LittleEndian.Uint16(ciphertext[1:3]))
	if nonceLen != nonceSize || len(ciphertext) < 3+nonceLen {
		return nil, ErrSessionUnreadable
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrSessionUnreadable
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrSessionUnreadable
	}
	nonce := ciphertext[3 : 3+nonceLen]
	body := ciphertext[3+nonceLen:]
	plaintext, err := gcm.Open(nil, nonce, body, aad)
	if err != nil {
		return nil, ErrSessionUnreadable
	}
	return plaintext, nil
}

// SessionAAD derives the additional authenticated data from the row identity.
func SessionAAD(owner, school string, generation int64) []byte {
	return []byte(fmt.Sprintf("henu-session-v1|%s|%s|%d", owner, school, generation))
}
