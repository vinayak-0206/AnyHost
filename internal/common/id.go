package common

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

// idCounter is used to ensure uniqueness within the same millisecond.
var idCounter uint64

// GenerateID generates a unique identifier suitable for session IDs,
// request IDs, and other identifiers. The format is:
// [timestamp-hex]-[random-hex]-[counter-hex]
// This ensures uniqueness even under high concurrency.
func GenerateID() string {
	timestamp := time.Now().UnixNano()
	counter := atomic.AddUint64(&idCounter, 1)

	randomBytes := make([]byte, 4)
	_, _ = rand.Read(randomBytes)

	return fmt.Sprintf("%x-%s-%x",
		timestamp,
		hex.EncodeToString(randomBytes),
		counter,
	)
}

// GenerateShortID generates a shorter random identifier (16 characters).
// Suitable for request IDs where brevity is preferred.
func GenerateShortID() string {
	bytes := make([]byte, 8)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GenerateToken generates a secure random token for authentication.
// The token is 32 bytes (64 hex characters).
func GenerateToken() string {
	bytes := make([]byte, 32)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GenerateSessionID generates a unique session identifier.
func GenerateSessionID() string {
	return "sess_" + GenerateID()
}

// GenerateRequestID generates a unique request identifier.
func GenerateRequestID() string {
	return "req_" + GenerateShortID()
}
