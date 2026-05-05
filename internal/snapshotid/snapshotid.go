package snapshotid

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// New returns a sortable snapshot ID: YYYYMMDDTHHMMSSZ-xxxxxxxx (8 hex random).
func New() (string, error) {
	ts := time.Now().UTC().Format("20060102T150405Z")
	var rb [4]byte
	if _, err := rand.Read(rb[:]); err != nil {
		return "", fmt.Errorf("snapshot id: %w", err)
	}
	sfx := binary.BigEndian.Uint32(rb[:])
	return fmt.Sprintf("%s-%08x", ts, sfx), nil
}
