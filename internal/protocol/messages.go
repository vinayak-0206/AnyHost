package protocol

import (
	"encoding/json"
	"fmt"
	"time"
)

// MessageType identifies the type of control message being sent.
type MessageType string

const (
	// MessageTypeHandshake is sent by the client to initiate a tunnel connection.
	MessageTypeHandshake MessageType = "handshake"

	// MessageTypeHandshakeResponse is sent by the server in response to a handshake.
	MessageTypeHandshakeResponse MessageType = "handshake_response"

	// MessageTypeAddTunnel requests adding a new tunnel to an existing session.
	MessageTypeAddTunnel MessageType = "add_tunnel"

	// MessageTypeRemoveTunnel requests removing a tunnel from an existing session.
	MessageTypeRemoveTunnel MessageType = "remove_tunnel"

	// MessageTypeTunnelUpdate is sent by the server to confirm tunnel changes.
	MessageTypeTunnelUpdate MessageType = "tunnel_update"

	// MessageTypePing is a keepalive message.
	MessageTypePing MessageType = "ping"

	// MessageTypePong is a response to a ping.
	MessageTypePong MessageType = "pong"

	// MessageTypeShutdown signals graceful shutdown intent.
	MessageTypeShutdown MessageType = "shutdown"

	// MessageTypeError indicates a protocol-level error.
	MessageTypeError MessageType = "error"
)

// Envelope wraps all control messages with type information for routing.
type Envelope struct {
	Type      MessageType     `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	RequestID string          `json:"request_id,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

// NewEnvelope creates a new envelope with the given type and payload.
func NewEnvelope(msgType MessageType, requestID string, payload interface{}) (*Envelope, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	return &Envelope{
		Type:      msgType,
		Timestamp: time.Now().UTC(),
		RequestID: requestID,
		Payload:   data,
	}, nil
}

// DecodePayload unmarshals the envelope payload into the given target.
func (e *Envelope) DecodePayload(target interface{}) error {
	if err := json.Unmarshal(e.Payload, target); err != nil {
		return fmt.Errorf("failed to decode payload: %w", err)
	}
	return nil
}

// TunnelConfig defines a single tunnel mapping from subdomain to local port.
type TunnelConfig struct {
	// Subdomain is the requested subdomain (e.g., "api" for api.example.com).
	Subdomain string `json:"subdomain" yaml:"subdomain"`

	// LocalPort is the local port to forward traffic to.
	LocalPort int `json:"local_port" yaml:"local_port"`

	// LocalHost is the local host to forward traffic to (default: localhost).
	LocalHost string `json:"local_host,omitempty" yaml:"local_host,omitempty"`

	// Protocol specifies the tunnel protocol: "http" or "tcp".
	// HTTP tunnels route based on Host header.
	// TCP tunnels require dedicated ports on the server.
	Protocol string `json:"protocol,omitempty" yaml:"protocol,omitempty"`
}

// Validate checks if the tunnel config is valid.
func (tc *TunnelConfig) Validate() error {
	if tc.Subdomain == "" {
		return fmt.Errorf("subdomain is required")
	}
	if tc.LocalPort <= 0 || tc.LocalPort > 65535 {
		return fmt.Errorf("local_port must be between 1 and 65535")
	}
	if tc.Protocol == "" {
		tc.Protocol = "http"
	}
	if tc.Protocol != "http" && tc.Protocol != "tcp" {
		return fmt.Errorf("protocol must be 'http' or 'tcp'")
	}
	if tc.LocalHost == "" {
		tc.LocalHost = "127.0.0.1"
	}
	return nil
}

// HandshakeRequest is sent by the client to initiate a tunnel session.
type HandshakeRequest struct {
	// Version is the protocol version the client supports.
	Version int `json:"version"`

	// Token is the authentication token for this client.
	Token string `json:"token"`

	// ClientID is a unique identifier for this client instance.
	// Used for logging and debugging.
	ClientID string `json:"client_id"`

	// Tunnels is the list of tunnels the client wants to establish.
	Tunnels []TunnelConfig `json:"tunnels"`

	// Capabilities lists optional features the client supports.
	Capabilities []string `json:"capabilities,omitempty"`
}

// Validate checks if the handshake request is valid.
func (hr *HandshakeRequest) Validate() error {
	if hr.Version < MinSupportedVersion {
		return fmt.Errorf("unsupported protocol version %d, minimum is %d", hr.Version, MinSupportedVersion)
	}
	if hr.Token == "" {
		return fmt.Errorf("token is required")
	}
	if len(hr.Tunnels) == 0 {
		return fmt.Errorf("at least one tunnel configuration is required")
	}
	for i, tunnel := range hr.Tunnels {
		if err := tunnel.Validate(); err != nil {
			return fmt.Errorf("tunnel[%d]: %w", i, err)
		}
	}
	return nil
}

// TunnelStatus represents the status of a registered tunnel.
type TunnelStatus struct {
	Subdomain string `json:"subdomain"`
	LocalPort int    `json:"local_port"`
	URL       string `json:"url"`
	Status    string `json:"status"` // "active", "pending", "error"
	Error     string `json:"error,omitempty"`
}

// HandshakeResponse is sent by the server in response to a handshake request.
type HandshakeResponse struct {
	// Success indicates whether the handshake was accepted.
	Success bool `json:"success"`

	// SessionID is the unique identifier for this tunnel session.
	SessionID string `json:"session_id,omitempty"`

	// Tunnels contains the status of each requested tunnel.
	Tunnels []TunnelStatus `json:"tunnels,omitempty"`

	// ServerVersion is the protocol version the server is using.
	ServerVersion int `json:"server_version"`

	// Error contains error details if Success is false.
	Error string `json:"error,omitempty"`

	// ErrorCode is a machine-readable error code.
	ErrorCode string `json:"error_code,omitempty"`
}

// AddTunnelRequest requests adding a new tunnel to an existing session.
type AddTunnelRequest struct {
	Tunnel TunnelConfig `json:"tunnel"`
}

// RemoveTunnelRequest requests removing a tunnel from an existing session.
type RemoveTunnelRequest struct {
	Subdomain string `json:"subdomain"`
}

// TunnelUpdateResponse is sent by the server to confirm tunnel changes.
type TunnelUpdateResponse struct {
	Success   bool         `json:"success"`
	Tunnel    TunnelStatus `json:"tunnel,omitempty"`
	Error     string       `json:"error,omitempty"`
	ErrorCode string       `json:"error_code,omitempty"`
}

// PingMessage is a keepalive ping.
type PingMessage struct {
	Timestamp time.Time `json:"timestamp"`
}

// PongMessage is a response to a ping.
type PongMessage struct {
	Timestamp     time.Time `json:"timestamp"`
	PingTimestamp time.Time `json:"ping_timestamp"`
}

// ShutdownMessage signals graceful shutdown intent.
type ShutdownMessage struct {
	Reason string `json:"reason,omitempty"`
	// GracePeriod is how long the sender will wait before closing.
	GracePeriodMs int `json:"grace_period_ms"`
}

// ErrorMessage indicates a protocol-level error.
type ErrorMessage struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// Common error codes for ErrorMessage.
const (
	ErrorCodeUnauthorized       = "UNAUTHORIZED"
	ErrorCodeSubdomainTaken     = "SUBDOMAIN_TAKEN"
	ErrorCodeSubdomainReserved  = "SUBDOMAIN_RESERVED"
	ErrorCodeSubdomainInvalid   = "SUBDOMAIN_INVALID"
	ErrorCodeRateLimited        = "RATE_LIMITED"
	ErrorCodeInternalError      = "INTERNAL_ERROR"
	ErrorCodeProtocolError      = "PROTOCOL_ERROR"
	ErrorCodeConnectionLimit    = "CONNECTION_LIMIT"
	ErrorCodeTunnelLimitReached = "TUNNEL_LIMIT_REACHED"
)
