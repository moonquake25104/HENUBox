package sync

import (
	"crypto/rand"
	"fmt"
	"time"
)

// newSyncID generates a UUID v4 string used for sync_id, device_id and
// dataset_id, mirroring src-tauri/src/sync/ids.rs.
func newSyncID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("sync-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
