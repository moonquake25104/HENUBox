// Package notify implements the W9 email-notification domain: notification
// ids, Chinese templates, an SMTP mailer and the central scheduler. It is a Go
// port of the Rust subsystem (src-tauri/src/notifications/*,
// src-tauri/src/ai/scheduler.rs) rebuilt around server timers and SMTP mail
// instead of process-internal threads and OS notifications.
package notify

// StableID derives a deterministic id from key using FNV-1a (64-bit) over the
// raw bytes, then masks the hash to the positive int32 range. The constants are
// the fixed FNV-1a offset basis and prime; the mask keeps the value non-negative
// for all keys.
func StableID(key string) int32 {
	const (
		offsetBasis uint64 = 0xcbf29ce484222325
		prime       uint64 = 0x100000001b3
	)

	hash := offsetBasis
	for i := 0; i < len(key); i++ {
		hash ^= uint64(key[i])
		hash *= prime
	}
	return int32(hash & 0x7FFFFFFF)
}
