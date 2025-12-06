package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/anyhost/gotunnel/internal/common"
	"github.com/anyhost/gotunnel/internal/protocol"
	"github.com/hashicorp/yamux"
)

// ControlPlane handles client connections and manages the tunnel registry.
type ControlPlane struct {
	config   *common.ServerConfig
	listener net.Listener
	registry *Registry
	auth     Authenticator
	logger   *slog.Logger

	mu       sync.RWMutex
	sessions map[string]*Session

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewControlPlane creates a new control plane.
func NewControlPlane(cfg *common.ServerConfig, registry *Registry, auth Authenticator, logger *slog.Logger) *ControlPlane {
	ctx, cancel := context.WithCancel(context.Background())

	return &ControlPlane{
		config:   cfg,
		registry: registry,
		auth:     auth,
		logger:   logger.With(slog.String("component", "control_plane")),
		sessions: make(map[string]*Session),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start starts listening for client connections.
func (cp *ControlPlane) Start() error {
	listener, err := net.Listen("tcp", cp.config.ControlAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", cp.config.ControlAddr, err)
	}
	cp.listener = listener

	cp.logger.Info("control plane listening", slog.String("addr", cp.config.ControlAddr))

	cp.wg.Add(1)
	go cp.acceptLoop()

	return nil
}

// Stop gracefully stops the control plane.
func (cp *ControlPlane) Stop(gracePeriod time.Duration) error {
	cp.logger.Info("stopping control plane", slog.Duration("grace_period", gracePeriod))

	// Signal shutdown to all sessions
	cp.mu.RLock()
	for _, session := range cp.sessions {
		go func(s *Session) {
			// Best-effort shutdown notification - ignore errors
			_ = s.Close()
		}(session)
	}
	cp.mu.RUnlock()

	// Stop accepting new connections
	cp.cancel()
	if cp.listener != nil {
		cp.listener.Close()
	}

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		cp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		cp.logger.Info("control plane stopped gracefully")
	case <-time.After(gracePeriod):
		cp.logger.Warn("control plane shutdown timed out")
	}

	return nil
}

// acceptLoop accepts incoming client connections.
func (cp *ControlPlane) acceptLoop() {
	defer cp.wg.Done()

	for {
		conn, err := cp.listener.Accept()
		if err != nil {
			select {
			case <-cp.ctx.Done():
				return
			default:
				if !errors.Is(err, net.ErrClosed) {
					cp.logger.Error("failed to accept connection", slog.Any("error", err))
				}
				continue
			}
		}

		cp.wg.Add(1)
		go cp.handleConnection(conn)
	}
}

// handleConnection handles a new client connection.
func (cp *ControlPlane) handleConnection(conn net.Conn) {
	defer cp.wg.Done()

	remoteAddr := conn.RemoteAddr().String()
	logger := cp.logger.With(slog.String("remote_addr", remoteAddr))
	logger.Debug("new connection")

	// Set handshake deadline
	if err := conn.SetDeadline(time.Now().Add(cp.config.Timeouts.HandshakeTimeout)); err != nil {
		logger.Error("failed to set deadline", slog.Any("error", err))
		conn.Close()
		return
	}

	// Create yamux session first (server mode) - client wraps connection in yamux
	yamuxConfig := DefaultYamuxConfig()
	muxSession, err := yamux.Server(conn, yamuxConfig)
	if err != nil {
		logger.Error("failed to create yamux session", slog.Any("error", err))
		conn.Close()
		return
	}

	// Accept the handshake stream from client
	stream, err := muxSession.AcceptStream()
	if err != nil {
		logger.Error("failed to accept handshake stream", slog.Any("error", err))
		muxSession.Close()
		conn.Close()
		return
	}

	// Create codec for handshake on the stream (not raw connection)
	codec := protocol.NewCodec(stream, stream)

	// Read handshake request
	envelope, err := codec.ReadMessage()
	if err != nil {
		logger.Error("failed to read handshake", slog.Any("error", err))
		stream.Close()
		muxSession.Close()
		conn.Close()
		return
	}

	if envelope.Type != protocol.MessageTypeHandshake {
		logger.Warn("unexpected message type", slog.String("type", string(envelope.Type)))
		cp.sendHandshakeError(codec, "expected handshake message", protocol.ErrorCodeProtocolError)
		stream.Close()
		muxSession.Close()
		conn.Close()
		return
	}

	var handshake protocol.HandshakeRequest
	if err := envelope.DecodePayload(&handshake); err != nil {
		logger.Error("failed to decode handshake", slog.Any("error", err))
		cp.sendHandshakeError(codec, "invalid handshake payload", protocol.ErrorCodeProtocolError)
		stream.Close()
		muxSession.Close()
		conn.Close()
		return
	}

	// Validate handshake
	if err := handshake.Validate(); err != nil {
		logger.Warn("invalid handshake", slog.Any("error", err))
		cp.sendHandshakeError(codec, err.Error(), protocol.ErrorCodeProtocolError)
		stream.Close()
		muxSession.Close()
		conn.Close()
		return
	}

	// Check protocol version
	if !protocol.IsVersionSupported(handshake.Version) {
		logger.Warn("unsupported protocol version", slog.Int("version", handshake.Version))
		cp.sendHandshakeError(codec, fmt.Sprintf("unsupported protocol version %d", handshake.Version), protocol.ErrorCodeProtocolError)
		stream.Close()
		muxSession.Close()
		conn.Close()
		return
	}

	// Authenticate
	valid, err := cp.auth.Validate(handshake.Token)
	if err != nil {
		logger.Error("authentication error", slog.Any("error", err))
		cp.sendHandshakeError(codec, "authentication failed", protocol.ErrorCodeUnauthorized)
		stream.Close()
		muxSession.Close()
		conn.Close()
		return
	}
	if !valid {
		logger.Warn("authentication failed")
		cp.sendHandshakeError(codec, "invalid token", protocol.ErrorCodeUnauthorized)
		stream.Close()
		muxSession.Close()
		conn.Close()
		return
	}

	// Check tunnel limits
	if len(handshake.Tunnels) > cp.config.Limits.MaxTunnelsPerConnection {
		logger.Warn("too many tunnels requested",
			slog.Int("requested", len(handshake.Tunnels)),
			slog.Int("max", cp.config.Limits.MaxTunnelsPerConnection))
		cp.sendHandshakeError(codec, fmt.Sprintf("maximum %d tunnels allowed", cp.config.Limits.MaxTunnelsPerConnection), protocol.ErrorCodeTunnelLimitReached)
		stream.Close()
		muxSession.Close()
		conn.Close()
		return
	}

	// Clear deadline for normal operation
	if err := conn.SetDeadline(time.Time{}); err != nil {
		logger.Error("failed to clear deadline", slog.Any("error", err))
		stream.Close()
		muxSession.Close()
		conn.Close()
		return
	}

	// Create session with the existing yamux session
	session, err := NewSessionWithMux(&SessionConfig{
		Conn:     conn,
		Token:    handshake.Token,
		ClientID: handshake.ClientID,
		Logger:   cp.logger,
	}, muxSession)
	if err != nil {
		logger.Error("failed to create session", slog.Any("error", err))
		cp.sendHandshakeError(codec, "internal error", protocol.ErrorCodeInternalError)
		stream.Close()
		muxSession.Close()
		conn.Close()
		return
	}

	// Register tunnels
	tunnelStatuses := cp.registry.Register(session, handshake.Tunnels)

	// Track registered tunnels in session
	for i, tc := range handshake.Tunnels {
		if tunnelStatuses[i].Status == "active" {
			session.RegisterTunnel(&tc)
		}
	}

	// Check if at least one tunnel was registered
	hasActiveTunnel := false
	for _, status := range tunnelStatuses {
		if status.Status == "active" {
			hasActiveTunnel = true
			break
		}
	}

	if !hasActiveTunnel {
		logger.Warn("no tunnels could be registered")
		cp.sendHandshakeResponse(codec, &protocol.HandshakeResponse{
			Success:       false,
			Tunnels:       tunnelStatuses,
			ServerVersion: protocol.ProtocolVersion,
			Error:         "no tunnels could be registered",
		})
		stream.Close()
		session.Close()
		return
	}

	// Send success response
	response := &protocol.HandshakeResponse{
		Success:       true,
		SessionID:     session.ID,
		Tunnels:       tunnelStatuses,
		ServerVersion: protocol.ProtocolVersion,
	}

	if err := cp.sendHandshakeResponse(codec, response); err != nil {
		logger.Error("failed to send handshake response", slog.Any("error", err))
		cp.registry.Unregister(session.ID)
		stream.Close()
		session.Close()
		return
	}

	// Close the handshake stream - it's no longer needed
	stream.Close()

	// Store session
	cp.mu.Lock()
	cp.sessions[session.ID] = session
	cp.mu.Unlock()

	session.SetState(SessionStateActive)

	logger.Info("session established",
		slog.String("session_id", session.ID),
		slog.Int("tunnels", len(session.GetTunnels())))

	// Handle session lifecycle
	cp.handleSession(session)

	// Cleanup on disconnect
	cp.mu.Lock()
	delete(cp.sessions, session.ID)
	cp.mu.Unlock()

	cp.registry.Unregister(session.ID)
	logger.Info("session ended", slog.String("session_id", session.ID))
}

// handleSession monitors a session for keepalive and handles control messages.
func (cp *ControlPlane) handleSession(session *Session) {
	defer session.Close()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cp.ctx.Done():
			return
		case <-session.Context().Done():
			return
		case <-ticker.C:
			// Check if session is still alive
			if session.IsClosed() {
				session.Logger().Info("session connection closed")
				return
			}

			// Check idle timeout
			if cp.config.Timeouts.IdleTimeout > 0 && session.IdleDuration() > cp.config.Timeouts.IdleTimeout {
				session.Logger().Info("session idle timeout")
				return
			}
		}
	}
}

// sendHandshakeError sends an error response during handshake.
func (cp *ControlPlane) sendHandshakeError(codec *protocol.Codec, message, code string) {
	response := &protocol.HandshakeResponse{
		Success:       false,
		ServerVersion: protocol.ProtocolVersion,
		Error:         message,
		ErrorCode:     code,
	}
	// Best-effort - ignore send errors during error handling
	_ = cp.sendHandshakeResponse(codec, response)
}

// sendHandshakeResponse sends a handshake response.
func (cp *ControlPlane) sendHandshakeResponse(codec *protocol.Codec, resp *protocol.HandshakeResponse) error {
	return codec.SendHandshakeResponse(resp)
}

// GetSession returns a session by ID.
func (cp *ControlPlane) GetSession(id string) (*Session, bool) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	s, ok := cp.sessions[id]
	return s, ok
}

// GetSessionCount returns the number of active sessions.
func (cp *ControlPlane) GetSessionCount() int {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	return len(cp.sessions)
}

// BroadcastShutdown sends a shutdown message to all connected clients.
func (cp *ControlPlane) BroadcastShutdown(reason string, gracePeriodMs int) {
	cp.mu.RLock()
	sessions := make([]*Session, 0, len(cp.sessions))
	for _, s := range cp.sessions {
		sessions = append(sessions, s)
	}
	cp.mu.RUnlock()

	for _, session := range sessions {
		go func(s *Session) {
			stream, err := s.OpenStream()
			if err != nil {
				return
			}
			defer stream.Close()

			codec := protocol.NewCodec(stream, stream)
			// Best-effort - ignore errors
			_ = codec.SendShutdown(reason, gracePeriodMs)
		}(session)
	}
}

// ProxyRequest proxies an incoming HTTP request to the appropriate client.
func (cp *ControlPlane) ProxyRequest(entry *TunnelEntry, requestID string) (io.ReadWriteCloser, error) {
	if !entry.Session.IsActive() {
		return nil, fmt.Errorf("session is not active")
	}

	header := &protocol.StreamHeader{
		Type:      protocol.StreamTypeHTTP,
		LocalPort: entry.LocalPort,
		LocalHost: entry.LocalHost,
		RequestID: requestID,
		Subdomain: entry.Subdomain,
	}

	stream, err := entry.Session.OpenStreamWithHeader(header)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	entry.Session.Metrics().RequestsHandled.Add(1)
	return stream, nil
}
