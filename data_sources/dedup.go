package data_sources

import (
	"sync"
	"time"
)

// A sensor re-advertises each measurement several times (three BLE channels,
// advertising interval shorter than the measurement interval), and the gateway
// forwards every copy. dedup drops a payload identical to the previous one
// from the same MAC within dedupWindow, so every sink sees each measurement
// once. Formats without a sequence number can repeat legitimately when the
// readings are stable; the window lets such a payload through again after a
// few seconds instead of suppressing it indefinitely.
const dedupWindow = 5 * time.Second

type dedupEntry struct {
	data  string
	first time.Time
}

type dedup struct {
	mu   sync.Mutex
	last map[string]dedupEntry
}

func newDedup() *dedup {
	return &dedup{last: make(map[string]dedupEntry)}
}

// duplicate reports whether data is a repeat of the last accepted payload of
// mac; otherwise it records data as the new reference and returns false.
func (d *dedup) duplicate(mac, data string, now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if e, ok := d.last[mac]; ok && e.data == data && now.Sub(e.first) < dedupWindow {
		return true
	}
	d.last[mac] = dedupEntry{data: data, first: now}
	return false
}
