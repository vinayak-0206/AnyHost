package protocol

import (
	"bytes"
	"testing"
	"time"
)

func TestCodec_WriteAndReadMessage(t *testing.T) {
	tests := []struct {
		name    string
		msgType MessageType
		payload interface{}
	}{
		{
			name:    "handshake message",
			msgType: MessageTypeHandshake,
			payload: &HandshakeRequest{
				Version:  1,
				Token:    "test-token",
				ClientID: "test-client",
				Tunnels: []TunnelConfig{
					{Subdomain: "test", LocalPort: 3000, Protocol: "http"},
				},
			},
		},
		{
			name:    "ping message",
			msgType: MessageTypePing,
			payload: &PingMessage{Timestamp: time.Now()},
		},
		{
			name:    "error message",
			msgType: MessageTypeError,
			payload: &ProtocolError{Code: ErrorCodeUnauthorized, Message: "invalid token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			codec := NewCodec(&buf, &buf)

			// Write message
			env, err := NewEnvelope(tt.msgType, "test-request", tt.payload)
			if err != nil {
				t.Fatalf("NewEnvelope failed: %v", err)
			}

			if err := codec.WriteMessage(env); err != nil {
				t.Fatalf("WriteMessage failed: %v", err)
			}

			// Read message back
			readEnv, err := codec.ReadMessage()
			if err != nil {
				t.Fatalf("ReadMessage failed: %v", err)
			}

			if readEnv.Type != tt.msgType {
				t.Errorf("message type mismatch: got %v, want %v", readEnv.Type, tt.msgType)
			}
		})
	}
}

func TestCodec_MaxMessageSize(t *testing.T) {
	var buf bytes.Buffer
	codec := NewCodec(&buf, &buf)

	// Create a message that exceeds max size
	largePayload := make([]byte, MaxMessageSize+1)
	env := &Envelope{
		Type:    MessageTypeHandshake,
		Payload: largePayload,
	}

	err := codec.WriteMessage(env)
	if err == nil {
		t.Error("expected error for oversized message, got nil")
	}
}

func TestTunnelConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  TunnelConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: TunnelConfig{
				Subdomain: "myapp",
				LocalPort: 3000,
				Protocol:  "http",
			},
			wantErr: false,
		},
		{
			name: "missing subdomain",
			config: TunnelConfig{
				LocalPort: 3000,
				Protocol:  "http",
			},
			wantErr: true,
		},
		{
			name: "invalid port zero",
			config: TunnelConfig{
				Subdomain: "myapp",
				LocalPort: 0,
				Protocol:  "http",
			},
			wantErr: true,
		},
		{
			name: "invalid port negative",
			config: TunnelConfig{
				Subdomain: "myapp",
				LocalPort: -1,
				Protocol:  "http",
			},
			wantErr: true,
		},
		{
			name: "port too high",
			config: TunnelConfig{
				Subdomain: "myapp",
				LocalPort: 70000,
				Protocol:  "http",
			},
			wantErr: true,
		},
		// Note: TunnelConfig.Validate() only checks basic requirements
		// Subdomain format validation happens in Registry.ValidateSubdomain()
		{
			name: "short subdomain allowed at config level",
			config: TunnelConfig{
				Subdomain: "ab",
				LocalPort: 3000,
			},
			wantErr: false, // Detailed validation happens in registry
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHandshakeRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     HandshakeRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: HandshakeRequest{
				Version: 1,
				Token:   "test-token",
				Tunnels: []TunnelConfig{
					{Subdomain: "test", LocalPort: 3000},
				},
			},
			wantErr: false,
		},
		{
			name: "missing token",
			req: HandshakeRequest{
				Version: 1,
				Tunnels: []TunnelConfig{
					{Subdomain: "test", LocalPort: 3000},
				},
			},
			wantErr: true,
		},
		{
			name: "no tunnels",
			req: HandshakeRequest{
				Version: 1,
				Token:   "test-token",
				Tunnels: []TunnelConfig{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
