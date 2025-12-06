// Package protocol defines the wire protocol for communication between
// the tunnel server and clients. It handles handshakes, stream multiplexing
// headers, and control messages.
package protocol

// Version constants for protocol compatibility checking.
// The server and client must agree on a compatible protocol version
// to establish a tunnel connection.
const (
	// ProtocolVersion is the current protocol version.
	// Increment this when making breaking changes to the protocol.
	ProtocolVersion = 1

	// MinSupportedVersion is the minimum protocol version the server will accept.
	// This allows for backwards compatibility during version transitions.
	MinSupportedVersion = 1
)

// IsVersionSupported checks if the given protocol version is supported.
func IsVersionSupported(version int) bool {
	return version >= MinSupportedVersion && version <= ProtocolVersion
}
