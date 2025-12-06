package protocol

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// MaxMessageSize is the maximum allowed size for a control message.
const MaxMessageSize = 64 * 1024 // 64KB

// Codec handles encoding and decoding of protocol messages over a connection.
// It is safe for concurrent use - reads and writes are independently synchronized.
type Codec struct {
	reader *bufio.Reader
	writer io.Writer

	readMu  sync.Mutex
	writeMu sync.Mutex
}

// NewCodec creates a new Codec for the given reader and writer.
func NewCodec(r io.Reader, w io.Writer) *Codec {
	return &Codec{
		reader: bufio.NewReader(r),
		writer: w,
	}
}

// WriteMessage encodes and writes a message envelope to the underlying writer.
// The format is: [4-byte length (big-endian)][JSON payload]
func (c *Codec) WriteMessage(envelope *Envelope) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("failed to marshal envelope: %w", err)
	}

	if len(data) > MaxMessageSize {
		return fmt.Errorf("message size %d exceeds maximum of %d bytes", len(data), MaxMessageSize)
	}

	// Write length prefix (4 bytes, big-endian)
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(data)))
	if _, err := c.writer.Write(lengthBuf); err != nil {
		return fmt.Errorf("failed to write message length: %w", err)
	}

	// Write JSON payload
	if _, err := c.writer.Write(data); err != nil {
		return fmt.Errorf("failed to write message payload: %w", err)
	}

	return nil
}

// ReadMessage reads and decodes a message envelope from the underlying reader.
func (c *Codec) ReadMessage() (*Envelope, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	// Read length prefix (4 bytes, big-endian)
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.reader, lengthBuf); err != nil {
		if err == io.EOF {
			return nil, ErrConnectionClosed
		}
		return nil, fmt.Errorf("failed to read message length: %w", err)
	}

	length := binary.BigEndian.Uint32(lengthBuf)
	if length > MaxMessageSize {
		return nil, fmt.Errorf("message size %d exceeds maximum of %d bytes", length, MaxMessageSize)
	}

	if length == 0 {
		return nil, fmt.Errorf("message length cannot be zero")
	}

	// Read JSON payload
	data := make([]byte, length)
	if _, err := io.ReadFull(c.reader, data); err != nil {
		return nil, fmt.Errorf("failed to read message payload: %w", err)
	}

	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("failed to unmarshal envelope: %w", err)
	}

	return &envelope, nil
}

// SendHandshake is a convenience method to send a handshake request.
func (c *Codec) SendHandshake(req *HandshakeRequest) error {
	envelope, err := NewEnvelope(MessageTypeHandshake, "", req)
	if err != nil {
		return fmt.Errorf("failed to create handshake envelope: %w", err)
	}
	return c.WriteMessage(envelope)
}

// SendHandshakeResponse is a convenience method to send a handshake response.
func (c *Codec) SendHandshakeResponse(resp *HandshakeResponse) error {
	envelope, err := NewEnvelope(MessageTypeHandshakeResponse, "", resp)
	if err != nil {
		return fmt.Errorf("failed to create handshake response envelope: %w", err)
	}
	return c.WriteMessage(envelope)
}

// SendPing sends a ping message.
func (c *Codec) SendPing(msg *PingMessage) error {
	envelope, err := NewEnvelope(MessageTypePing, "", msg)
	if err != nil {
		return fmt.Errorf("failed to create ping envelope: %w", err)
	}
	return c.WriteMessage(envelope)
}

// SendPong sends a pong message.
func (c *Codec) SendPong(msg *PongMessage) error {
	envelope, err := NewEnvelope(MessageTypePong, "", msg)
	if err != nil {
		return fmt.Errorf("failed to create pong envelope: %w", err)
	}
	return c.WriteMessage(envelope)
}

// SendError sends an error message.
func (c *Codec) SendError(requestID, code, message string) error {
	errMsg := &ErrorMessage{
		Code:    code,
		Message: message,
	}
	envelope, err := NewEnvelope(MessageTypeError, requestID, errMsg)
	if err != nil {
		return fmt.Errorf("failed to create error envelope: %w", err)
	}
	return c.WriteMessage(envelope)
}

// SendShutdown sends a shutdown message.
func (c *Codec) SendShutdown(reason string, gracePeriodMs int) error {
	msg := &ShutdownMessage{
		Reason:        reason,
		GracePeriodMs: gracePeriodMs,
	}
	envelope, err := NewEnvelope(MessageTypeShutdown, "", msg)
	if err != nil {
		return fmt.Errorf("failed to create shutdown envelope: %w", err)
	}
	return c.WriteMessage(envelope)
}
