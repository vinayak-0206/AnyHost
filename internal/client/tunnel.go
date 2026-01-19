package client

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/anyhost/gotunnel/internal/common"
	"github.com/anyhost/gotunnel/internal/protocol"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

// TunnelState represents the current state of the tunnel.
type TunnelState int32

const (
	// TunnelStateDisconnected indicates no active connection.
	TunnelStateDisconnected TunnelState = iota

	// TunnelStateConnecting indicates connection is being established.
	TunnelStateConnecting

	// TunnelStateConnected indicates tunnel is active.
	TunnelStateConnected

	// TunnelStateReconnecting indicates reconnection in progress.
	TunnelStateReconnecting

	// TunnelStateClosed indicates tunnel has been permanently closed.
	TunnelStateClosed
)

func (s TunnelState) String() string {
	switch s {
	case TunnelStateDisconnected:
		return "disconnected"
	case TunnelStateConnecting:
		return "connecting"
	case TunnelStateConnected:
		return "connected"
	case TunnelStateReconnecting:
		return "reconnecting"
	case TunnelStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// RequestInfo contains information about an incoming request.
type RequestInfo struct {
	ID        string
	Subdomain string
	LocalPort int
	Method    string
	Path      string
	Timestamp time.Time
	Duration  time.Duration
	Status    int
	BytesIn   int64
	BytesOut  int64
}

// RequestHandler is called for each request.
type RequestHandler func(info RequestInfo)

// Tunnel is the main client that connects to the tunnel server.
type Tunnel struct {
	config *common.ClientConfig
	logger *slog.Logger

	conn       net.Conn
	muxSession *yamux.Session
	sessionID  string

	router    *Router
	reconnect *Reconnector

	state atomic.Int32

	mu              sync.RWMutex
	tunnelStatus    []protocol.TunnelStatus
	stateHandlers   []func(TunnelState)
	requestHandlers []RequestHandler

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewTunnel creates a new tunnel client.
func NewTunnel(cfg *common.ClientConfig, logger *slog.Logger) (*Tunnel, error) {
	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())

	t := &Tunnel{
		config:          cfg,
		logger:          logger.With(slog.String("component", "tunnel")),
		ctx:             ctx,
		cancel:          cancel,
		stateHandlers:   make([]func(TunnelState), 0),
		requestHandlers: make([]RequestHandler, 0),
	}

	// Create router with connection pooling
	t.router = NewRouter(cfg, logger)

	// Create reconnector if enabled
	if cfg.Reconnect.Enabled {
		t.reconnect = NewReconnector(&cfg.Reconnect, logger)
	}

	t.state.Store(int32(TunnelStateDisconnected))

	return t, nil
}

// Connect establishes a connection to the tunnel server.
// It auto-detects whether to use WebSocket or raw TCP based on the server address.
func (t *Tunnel) Connect() error {
	t.setState(TunnelStateConnecting)

	t.logger.Info("connecting to server", slog.String("addr", t.config.ServerAddr))

	var conn net.Conn
	var err error

	// Check if server address is a WebSocket URL
	if strings.HasPrefix(t.config.ServerAddr, "ws://") || strings.HasPrefix(t.config.ServerAddr, "wss://") {
		conn, err = t.dialWebSocket()
	} else if strings.Contains(t.config.ServerAddr, "://") {
		// HTTP/HTTPS URL - convert to WebSocket
		wsURL := strings.Replace(t.config.ServerAddr, "https://", "wss://", 1)
		wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
		if !strings.HasSuffix(wsURL, "/tunnel") {
			wsURL = strings.TrimSuffix(wsURL, "/") + "/tunnel"
		}
		t.config.ServerAddr = wsURL
		conn, err = t.dialWebSocket()
	} else {
		// Raw TCP connection
		conn, err = t.dialTCP()
	}

	if err != nil {
		t.setState(TunnelStateDisconnected)
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	t.conn = conn

	// Create yamux session (client mode)
	yamuxConfig := t.defaultYamuxConfig()
	muxSession, err := yamux.Client(conn, yamuxConfig)
	if err != nil {
		conn.Close()
		t.setState(TunnelStateDisconnected)
		return fmt.Errorf("failed to create yamux session: %w", err)
	}
	t.muxSession = muxSession

	// Perform handshake
	if err := t.performHandshake(); err != nil {
		muxSession.Close()
		conn.Close()
		t.setState(TunnelStateDisconnected)
		return fmt.Errorf("handshake failed: %w", err)
	}

	t.setState(TunnelStateConnected)
	t.logger.Info("connected to server", slog.String("session_id", t.sessionID))

	return nil
}

// dialTCP establishes a raw TCP connection to the server.
func (t *Tunnel) dialTCP() (net.Conn, error) {
	return net.DialTimeout("tcp", t.config.ServerAddr, 10*time.Second)
}

// dialWebSocket establishes a WebSocket connection to the server.
func (t *Tunnel) dialWebSocket() (net.Conn, error) {
	t.logger.Debug("connecting via WebSocket", slog.String("url", t.config.ServerAddr))

	// Parse and validate URL
	u, err := url.Parse(t.config.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid WebSocket URL: %w", err)
	}

	// Ensure path ends with /tunnel
	if !strings.HasSuffix(u.Path, "/tunnel") {
		u.Path = strings.TrimSuffix(u.Path, "/") + "/tunnel"
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	ws, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("WebSocket dial failed: %w", err)
	}

	return common.NewWSConn(ws), nil
}

// performHandshake performs the initial handshake with the server.
func (t *Tunnel) performHandshake() error {
	// Open a stream for handshake
	stream, err := t.muxSession.Open()
	if err != nil {
		return fmt.Errorf("failed to open handshake stream: %w", err)
	}
	defer stream.Close()

	codec := protocol.NewCodec(stream, stream)

	// Build handshake request
	request := &protocol.HandshakeRequest{
		Version:  protocol.ProtocolVersion,
		Token:    t.config.Token,
		ClientID: t.config.ClientID,
		Tunnels:  t.config.Tunnels,
	}

	// Send handshake
	if err := codec.SendHandshake(request); err != nil {
		return fmt.Errorf("failed to send handshake: %w", err)
	}

	// Read response
	envelope, err := codec.ReadMessage()
	if err != nil {
		return fmt.Errorf("failed to read handshake response: %w", err)
	}

	if envelope.Type != protocol.MessageTypeHandshakeResponse {
		return fmt.Errorf("unexpected message type: %s", envelope.Type)
	}

	var response protocol.HandshakeResponse
	if err := envelope.DecodePayload(&response); err != nil {
		return fmt.Errorf("failed to decode handshake response: %w", err)
	}

	if !response.Success {
		return fmt.Errorf("handshake rejected: %s (code: %s)", response.Error, response.ErrorCode)
	}

	t.sessionID = response.SessionID
	t.mu.Lock()
	t.tunnelStatus = response.Tunnels
	t.mu.Unlock()

	// Log tunnel status
	for _, status := range response.Tunnels {
		if status.Status == "active" {
			t.logger.Info("tunnel active",
				slog.String("subdomain", status.Subdomain),
				slog.Int("local_port", status.LocalPort),
				slog.String("url", status.URL))
		} else {
			t.logger.Warn("tunnel failed",
				slog.String("subdomain", status.Subdomain),
				slog.String("error", status.Error))
		}
	}

	return nil
}

// Run starts the tunnel and blocks until closed.
func (t *Tunnel) Run() error {
	// Initial connection
	if err := t.Connect(); err != nil {
		if t.reconnect != nil {
			t.logger.Warn("initial connection failed, will retry", slog.Any("error", err))
		} else {
			return err
		}
	}

	// Start accepting streams
	t.wg.Add(1)
	go t.acceptLoop()

	// Wait for interrupt or context cancellation
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		t.logger.Info("received signal", slog.String("signal", sig.String()))
	case <-t.ctx.Done():
		t.logger.Info("context cancelled")
	}

	return t.Close()
}

// acceptLoop accepts incoming streams from the server.
func (t *Tunnel) acceptLoop() {
	defer t.wg.Done()

	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		// Check if we need to reconnect
		if t.State() == TunnelStateDisconnected && t.reconnect != nil {
			t.handleReconnect()
			continue
		}

		if t.muxSession == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Accept stream with timeout
		stream, err := t.muxSession.AcceptStream()
		if err != nil {
			select {
			case <-t.ctx.Done():
				return
			default:
			}

			if t.muxSession.IsClosed() {
				t.logger.Warn("connection lost")
				t.setState(TunnelStateDisconnected)
				continue
			}

			t.logger.Error("failed to accept stream", slog.Any("error", err))
			continue
		}

		// Handle stream in goroutine
		t.wg.Add(1)
		go func() {
			defer t.wg.Done()
			t.handleStream(stream)
		}()
	}
}

// handleStream handles an incoming stream from the server.
func (t *Tunnel) handleStream(stream net.Conn) {
	defer stream.Close()

	startTime := time.Now()

	// Read stream header
	header, err := protocol.ReadStreamHeader(stream)
	if err != nil {
		t.logger.Error("failed to read stream header", slog.Any("error", err))
		return
	}

	logger := t.logger.With(
		slog.String("request_id", header.RequestID),
		slog.String("subdomain", header.Subdomain),
		slog.Int("local_port", header.LocalPort),
	)

	logger.Debug("handling request")

	// Create request info
	info := RequestInfo{
		ID:        header.RequestID,
		Subdomain: header.Subdomain,
		LocalPort: header.LocalPort,
		Method:    header.Method,
		Path:      header.Path,
		Timestamp: startTime,
	}

	// Notify handlers that request started
	t.notifyRequest(info)

	// Forward to local service
	if err := t.router.Forward(stream, header); err != nil {
		logger.Error("failed to forward request", slog.Any("error", err))
		info.Duration = time.Since(startTime)
		info.Status = 502 // Bad Gateway
		t.notifyRequest(info)
		return
	}

	info.Duration = time.Since(startTime)
	info.Status = 200
	logger.Debug("request completed")
}

// handleReconnect handles reconnection with exponential backoff.
func (t *Tunnel) handleReconnect() {
	t.setState(TunnelStateReconnecting)

	delay := t.reconnect.NextDelay()
	t.logger.Info("reconnecting", slog.Duration("delay", delay))

	select {
	case <-time.After(delay):
	case <-t.ctx.Done():
		return
	}

	if err := t.Connect(); err != nil {
		t.logger.Warn("reconnection failed", slog.Any("error", err))
		return
	}

	t.reconnect.Reset()
}

// Close gracefully closes the tunnel.
func (t *Tunnel) Close() error {
	t.logger.Info("closing tunnel")

	t.setState(TunnelStateClosed)
	t.cancel()

	var errs []error

	// Close router (drains connection pools)
	if t.router != nil {
		t.router.Close()
	}

	// Close yamux session
	if t.muxSession != nil {
		if err := t.muxSession.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close yamux session: %w", err))
		}
	}

	// Close connection
	if t.conn != nil {
		if err := t.conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close connection: %w", err))
		}
	}

	// Wait for goroutines
	t.wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("errors during close: %v", errs)
	}

	t.logger.Info("tunnel closed")
	return nil
}

// State returns the current tunnel state.
func (t *Tunnel) State() TunnelState {
	return TunnelState(t.state.Load())
}

// setState sets the tunnel state and notifies handlers.
func (t *Tunnel) setState(state TunnelState) {
	t.state.Store(int32(state))

	t.mu.RLock()
	handlers := t.stateHandlers
	t.mu.RUnlock()

	for _, handler := range handlers {
		go handler(state)
	}
}

// OnStateChange registers a handler for state changes.
func (t *Tunnel) OnStateChange(handler func(TunnelState)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stateHandlers = append(t.stateHandlers, handler)
}

// OnRequest registers a handler for incoming requests.
func (t *Tunnel) OnRequest(handler RequestHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requestHandlers = append(t.requestHandlers, handler)
}

// notifyRequest notifies all request handlers.
func (t *Tunnel) notifyRequest(info RequestInfo) {
	t.mu.RLock()
	handlers := make([]RequestHandler, len(t.requestHandlers))
	copy(handlers, t.requestHandlers)
	t.mu.RUnlock()

	for _, handler := range handlers {
		go handler(info)
	}
}

// GetTunnelStatus returns the status of all tunnels.
func (t *Tunnel) GetTunnelStatus() []protocol.TunnelStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.tunnelStatus
}

// SessionID returns the current session ID.
func (t *Tunnel) SessionID() string {
	return t.sessionID
}

// defaultYamuxConfig returns sensible yamux configuration for clients.
func (t *Tunnel) defaultYamuxConfig() *yamux.Config {
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
