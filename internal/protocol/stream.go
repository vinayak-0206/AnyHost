package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// StreamType identifies the type of data stream.
type StreamType string

const (
	// StreamTypeHTTP indicates an HTTP request/response stream.
	StreamTypeHTTP StreamType = "http"

	// StreamTypeTCP indicates a raw TCP stream.
	StreamTypeTCP StreamType = "tcp"

	// StreamTypeWebSocket indicates a WebSocket connection.
	StreamTypeWebSocket StreamType = "websocket"
)

// StreamHeader is sent at the beginning of each multiplexed stream
// to inform the client which local port to forward to.
type StreamHeader struct {
	// Type identifies the stream type for proper handling.
	Type StreamType `json:"type"`

	// LocalPort is the target local port on the client.
	LocalPort int `json:"local_port"`

	// LocalHost is the target local host on the client (default: 127.0.0.1).
	LocalHost string `json:"local_host,omitempty"`

	// RequestID is a unique identifier for request correlation and logging.
	RequestID string `json:"request_id"`

	// Subdomain identifies which tunnel this stream belongs to.
	Subdomain string `json:"subdomain"`

	// RemoteAddr is the original client's IP address.
	RemoteAddr string `json:"remote_addr,omitempty"`

	// Host is the original Host header (for HTTP streams).
	Host string `json:"host,omitempty"`

	// Method is the HTTP method (GET, POST, etc.) for request inspection.
	Method string `json:"method,omitempty"`

	// Path is the HTTP request path for request inspection.
	Path string `json:"path,omitempty"`
}

// MaxStreamHeaderSize is the maximum allowed size for a stream header.
// This prevents malicious clients from sending oversized headers.
const MaxStreamHeaderSize = 4096

// WriteStreamHeader writes a stream header to the given writer.
// The format is: [4-byte length (big-endian)][JSON payload]
func WriteStreamHeader(w io.Writer, header *StreamHeader) error {
	data, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal stream header: %w", err)
	}

	if len(data) > MaxStreamHeaderSize {
		return fmt.Errorf("stream header exceeds maximum size of %d bytes", MaxStreamHeaderSize)
	}

	// Write length prefix (4 bytes, big-endian)
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(data)))
	if _, err := w.Write(lengthBuf); err != nil {
		return fmt.Errorf("failed to write header length: %w", err)
	}

	// Write JSON payload
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write header payload: %w", err)
	}

	return nil
}

// ReadStreamHeader reads a stream header from the given reader.
// Returns an error if the header is malformed or exceeds MaxStreamHeaderSize.
func ReadStreamHeader(r io.Reader) (*StreamHeader, error) {
	// Read length prefix (4 bytes, big-endian)
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lengthBuf); err != nil {
		return nil, fmt.Errorf("failed to read header length: %w", err)
	}

	length := binary.BigEndian.Uint32(lengthBuf)
	if length > MaxStreamHeaderSize {
		return nil, fmt.Errorf("stream header size %d exceeds maximum of %d bytes", length, MaxStreamHeaderSize)
	}

	if length == 0 {
		return nil, fmt.Errorf("stream header length cannot be zero")
	}

	// Read JSON payload
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read header payload: %w", err)
	}

	var header StreamHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stream header: %w", err)
	}

	return &header, nil
}

// Validate checks if the stream header contains valid values.
func (sh *StreamHeader) Validate() error {
	if sh.Type == "" {
		return fmt.Errorf("stream type is required")
	}
	if sh.Type != StreamTypeHTTP && sh.Type != StreamTypeTCP && sh.Type != StreamTypeWebSocket {
		return fmt.Errorf("invalid stream type: %s", sh.Type)
	}
	if sh.LocalPort <= 0 || sh.LocalPort > 65535 {
		return fmt.Errorf("local_port must be between 1 and 65535")
	}
	if sh.RequestID == "" {
		return fmt.Errorf("request_id is required")
	}
	return nil
}

// GetLocalAddr returns the full local address (host:port) to connect to.
func (sh *StreamHeader) GetLocalAddr() string {
	host := sh.LocalHost
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%d", host, sh.LocalPort)
}
