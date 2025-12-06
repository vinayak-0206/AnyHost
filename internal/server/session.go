package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyhost/gotunnel/internal/common"
	"github.com/anyhost/gotunnel/internal/protocol"
	"github.com/hashicorp/yamux"
)

// SessionState represents the current state of a session.
type SessionState int32

const (
	// SessionStateConnecting indicates the session is being established.
	SessionStateConnecting SessionState = iota

	// SessionStateActive indicates the session is active and can handle traffic.
	SessionStateActive

	// SessionStateClosing indicates the session is gracefully closing.
	SessionStateClosing

	// SessionStateClosed indicates the session has been closed.
	SessionStateClosed
)

func (s SessionState) String() string {
	switch s {
	case SessionStateConnecting:
		return "connecting"
	case SessionStateActive:
		return "active"
	case SessionStateClosing:
		return "closing"
	case SessionStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// Session represents a connected client with multiplexed streams.
type Session struct {
	// ID is the unique session identifier.
	ID string

	// ClientID is the client-provided identifier.
	ClientID string

	// Token is the authentication token used for this session.
	Token string

	// RemoteAddr is the remote address of the client.
	RemoteAddr string

	// CreatedAt is when the session was created.
	CreatedAt time.Time

	// conn is the underlying network connection.
	conn net.Conn

	// muxSession is the yamux multiplexer session.
	muxSession *yamux.Session

	// codec is used for control message communication.
	codec *protocol.Codec

	// state is the current session state.
	state atomic.Int32

	// mu protects concurrent access to session fields.
	mu sync.RWMutex

	// tunnels tracks the tunnels registered for this session.
	tunnels map[string]*protocol.TunnelConfig

	// logger is the session-specific logger.
	logger *slog.Logger

	// ctx is the session context for cancellation.
	ctx context.Context

	// cancel cancels the session context.
	cancel context.CancelFunc

	// lastActivity tracks the last activity time for idle detection.
	lastActivity atomic.Int64

	// metrics tracks session-level metrics.
	metrics *SessionMetrics
}

// SessionMetrics tracks metrics for a session.
type SessionMetrics struct {
	StreamsOpened   atomic.Int64
	StreamsClosed   atomic.Int64
	BytesSent       atomic.Int64
	BytesReceived   atomic.Int64
	RequestsHandled atomic.Int64
	Errors          atomic.Int64
}

// SessionConfig holds configuration for creating a new session.
type SessionConfig struct {
	Conn       net.Conn
	Token      string
	ClientID   string
	Logger     *slog.Logger
	YamuxConf  *yamux.Config
}

// NewSession creates a new session from an established connection.
func NewSession(cfg *SessionConfig) (*Session, error) {
	ctx, cancel := context.WithCancel(context.Background())

	sessionID := common.GenerateSessionID()

	// Create yamux session (server mode)
	yamuxConfig := cfg.YamuxConf
	if yamuxConfig == nil {
		yamuxConfig = DefaultYamuxConfig()
	}

	muxSession, err := yamux.Server(cfg.Conn, yamuxConfig)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create yamux session: %w", err)
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With(
		slog.String("session_id", sessionID),
		slog.String("client_id", cfg.ClientID),
		slog.String("remote_addr", cfg.Conn.RemoteAddr().String()),
	)

	s := &Session{
		ID:         sessionID,
		ClientID:   cfg.ClientID,
		Token:      cfg.Token,
		RemoteAddr: cfg.Conn.RemoteAddr().String(),
		CreatedAt:  time.Now(),
		conn:       cfg.Conn,
		muxSession: muxSession,
		tunnels:    make(map[string]*protocol.TunnelConfig),
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		metrics:    &SessionMetrics{},
	}

	s.state.Store(int32(SessionStateConnecting))
	s.updateActivity()

	return s, nil
}

// NewSessionWithMux creates a new session with an existing yamux session.
// Use this when the yamux session has already been established (e.g., during handshake).
func NewSessionWithMux(cfg *SessionConfig, muxSession *yamux.Session) (*Session, error) {
	ctx, cancel := context.WithCancel(context.Background())

	sessionID := common.GenerateSessionID()

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With(
		slog.String("session_id", sessionID),
		slog.String("client_id", cfg.ClientID),
		slog.String("remote_addr", cfg.Conn.RemoteAddr().String()),
	)

	s := &Session{
		ID:         sessionID,
		ClientID:   cfg.ClientID,
		Token:      cfg.Token,
		RemoteAddr: cfg.Conn.RemoteAddr().String(),
		CreatedAt:  time.Now(),
		conn:       cfg.Conn,
		muxSession: muxSession,
		tunnels:    make(map[string]*protocol.TunnelConfig),
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		metrics:    &SessionMetrics{},
	}

	s.state.Store(int32(SessionStateConnecting))
	s.updateActivity()

	return s, nil
}

// DefaultYamuxConfig returns sensible yamux configuration.
func DefaultYamuxConfig() *yamux.Config {
	config := yamux.DefaultConfig()
	config.AcceptBacklog = 256
	config.EnableKeepAlive = true
	config.KeepAliveInterval = 30 * time.Second
	config.ConnectionWriteTimeout = 10 * time.Second
	config.StreamOpenTimeout = 30 * time.Second
	config.StreamCloseTimeout = 5 * time.Minute
	config.MaxStreamWindowSize = 256 * 1024 // 256KB
	return config
}

// State returns the current session state.
func (s *Session) State() SessionState {
	return SessionState(s.state.Load())
}

// SetState sets the session state.
func (s *Session) SetState(state SessionState) {
	s.state.Store(int32(state))
}

// IsActive returns true if the session is in active state.
func (s *Session) IsActive() bool {
	return s.State() == SessionStateActive
}

// Context returns the session context.
func (s *Session) Context() context.Context {
	return s.ctx
}

// Logger returns the session logger.
func (s *Session) Logger() *slog.Logger {
	return s.logger
}

// Metrics returns the session metrics.
func (s *Session) Metrics() *SessionMetrics {
	return s.metrics
}

// OpenStream opens a new multiplexed stream to the client.
// This is used by the server to send incoming requests to the client.
func (s *Session) OpenStream() (net.Conn, error) {
	if !s.IsActive() {
		return nil, fmt.Errorf("session is not active")
	}

	stream, err := s.muxSession.OpenStream()
	if err != nil {
		s.metrics.Errors.Add(1)
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	s.metrics.StreamsOpened.Add(1)
	s.updateActivity()

	return stream, nil
}

// OpenStreamWithHeader opens a stream and writes the stream header.
func (s *Session) OpenStreamWithHeader(header *protocol.StreamHeader) (net.Conn, error) {
	stream, err := s.OpenStream()
	if err != nil {
		return nil, err
	}

	if err := protocol.WriteStreamHeader(stream, header); err != nil {
		stream.Close()
		return nil, fmt.Errorf("failed to write stream header: %w", err)
	}

	return stream, nil
}

// AcceptStream accepts an incoming stream from the client.
// This is used for streams initiated by the client (e.g., control messages).
func (s *Session) AcceptStream() (net.Conn, error) {
	stream, err := s.muxSession.AcceptStream()
	if err != nil {
		if err == io.EOF {
			return nil, protocol.ErrConnectionClosed
		}
		return nil, fmt.Errorf("failed to accept stream: %w", err)
	}

	s.updateActivity()
	return stream, nil
}

// RegisterTunnel adds a tunnel to the session's tunnel map.
func (s *Session) RegisterTunnel(tc *protocol.TunnelConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tunnels[tc.Subdomain] = tc
}

// UnregisterTunnel removes a tunnel from the session's tunnel map.
func (s *Session) UnregisterTunnel(subdomain string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tunnels, subdomain)
}

// GetTunnel returns a tunnel configuration by subdomain.
func (s *Session) GetTunnel(subdomain string) (*protocol.TunnelConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tc, exists := s.tunnels[subdomain]
	return tc, exists
}

// GetTunnels returns all tunnel configurations for this session.
func (s *Session) GetTunnels() []*protocol.TunnelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tunnels := make([]*protocol.TunnelConfig, 0, len(s.tunnels))
	for _, tc := range s.tunnels {
		tunnels = append(tunnels, tc)
	}
	return tunnels
}

// Close gracefully closes the session.
func (s *Session) Close() error {
	if !s.state.CompareAndSwap(int32(SessionStateActive), int32(SessionStateClosing)) {
		// Already closing or closed
		return nil
	}

	s.logger.Info("closing session")

	s.cancel()

	var errs []error

	if s.muxSession != nil {
		if err := s.muxSession.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close yamux session: %w", err))
		}
	}

	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close connection: %w", err))
		}
	}

	s.state.Store(int32(SessionStateClosed))

	if len(errs) > 0 {
		return fmt.Errorf("errors closing session: %v", errs)
	}

	return nil
}

// updateActivity updates the last activity timestamp.
func (s *Session) updateActivity() {
	s.lastActivity.Store(time.Now().UnixNano())
}

// LastActivity returns the last activity time.
func (s *Session) LastActivity() time.Time {
	return time.Unix(0, s.lastActivity.Load())
}

// IdleDuration returns how long the session has been idle.
func (s *Session) IdleDuration() time.Duration {
	return time.Since(s.LastActivity())
}

// IsClosed returns true if the underlying connection is closed.
func (s *Session) IsClosed() bool {
	return s.muxSession.IsClosed()
}
