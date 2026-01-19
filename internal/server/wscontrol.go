package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/anyhost/gotunnel/internal/common"
	"github.com/anyhost/gotunnel/internal/protocol"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

// createUpgrader creates a WebSocket upgrader with origin checking based on config.
// For tunnel connections, we're more permissive since the tunnel protocol has its own auth.
func createUpgrader(config *common.ServerConfig) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  16 * 1024,
		WriteBufferSize: 16 * 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			// If no origin header (e.g., CLI clients), allow
			if origin == "" {
				return true
			}
			// Check against allowed origins
			return config.IsOriginAllowed(origin)
		},
	}
}

// HandleWebSocket handles WebSocket connections for the control plane.
// This allows clients to connect via WebSocket instead of raw TCP.
func (cp *ControlPlane) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	logger := cp.logger.With(slog.String("remote_addr", r.RemoteAddr))
	logger.Debug("WebSocket connection request")

	// Create upgrader with proper origin checking
	upgrader := createUpgrader(cp.config)

	// Upgrade to WebSocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("failed to upgrade to WebSocket", slog.Any("error", err))
		return
	}

	// Wrap WebSocket as net.Conn
	conn := common.NewWSConn(ws)

	// Handle the connection using existing logic
	cp.wg.Add(1)
	go cp.handleWebSocketConnection(conn, logger)
}

// handleWebSocketConnection handles a WebSocket-based client connection.
func (cp *ControlPlane) handleWebSocketConnection(conn *common.WSConn, logger *slog.Logger) {
	defer cp.wg.Done()
	defer conn.Close()

	logger.Debug("handling WebSocket connection")

	// Create yamux session
	yamuxConfig := DefaultYamuxConfig()
	muxSession, err := yamux.Server(conn, yamuxConfig)
	if err != nil {
		logger.Error("failed to create yamux session", slog.Any("error", err))
		return
	}

	// Accept the handshake stream from client
	stream, err := muxSession.AcceptStream()
	if err != nil {
		logger.Error("failed to accept handshake stream", slog.Any("error", err))
		muxSession.Close()
		return
	}

	// Create codec for handshake on the stream
	codec := protocol.NewCodec(stream, stream)

	// Read handshake request
	envelope, err := codec.ReadMessage()
	if err != nil {
		logger.Error("failed to read handshake", slog.Any("error", err))
		stream.Close()
		muxSession.Close()
		return
	}

	if envelope.Type != protocol.MessageTypeHandshake {
		logger.Warn("unexpected message type", slog.String("type", string(envelope.Type)))
		cp.sendHandshakeError(codec, "expected handshake message", protocol.ErrorCodeProtocolError)
		stream.Close()
		muxSession.Close()
		return
	}

	var handshake protocol.HandshakeRequest
	if err := envelope.DecodePayload(&handshake); err != nil {
		logger.Error("failed to decode handshake", slog.Any("error", err))
		cp.sendHandshakeError(codec, "invalid handshake payload", protocol.ErrorCodeProtocolError)
		stream.Close()
		muxSession.Close()
		return
	}

	// Validate handshake
	if err := handshake.Validate(); err != nil {
		logger.Warn("invalid handshake", slog.Any("error", err))
		cp.sendHandshakeError(codec, err.Error(), protocol.ErrorCodeProtocolError)
		stream.Close()
		muxSession.Close()
		return
	}

	// Check protocol version
	if !protocol.IsVersionSupported(handshake.Version) {
		logger.Warn("unsupported protocol version", slog.Int("version", handshake.Version))
		cp.sendHandshakeError(codec, fmt.Sprintf("unsupported protocol version %d", handshake.Version), protocol.ErrorCodeProtocolError)
		stream.Close()
		muxSession.Close()
		return
	}

	// Authenticate
	valid, err := cp.auth.Validate(handshake.Token)
	if err != nil {
		logger.Error("authentication error", slog.Any("error", err))
		cp.sendHandshakeError(codec, "authentication failed", protocol.ErrorCodeUnauthorized)
		stream.Close()
		muxSession.Close()
		return
	}
	if !valid {
		logger.Warn("authentication failed")
		cp.sendHandshakeError(codec, "invalid token", protocol.ErrorCodeUnauthorized)
		stream.Close()
		muxSession.Close()
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

	// Close the handshake stream
	stream.Close()

	// Store session
	cp.mu.Lock()
	cp.sessions[session.ID] = session
	cp.mu.Unlock()

	session.SetState(SessionStateActive)

	logger.Info("WebSocket session established",
		slog.String("session_id", session.ID),
		slog.Int("tunnels", len(session.GetTunnels())))

	// Handle session lifecycle
	cp.handleSession(session)

	// Cleanup on disconnect
	cp.mu.Lock()
	delete(cp.sessions, session.ID)
	cp.mu.Unlock()

	cp.registry.Unregister(session.ID)
	logger.Info("WebSocket session ended", slog.String("session_id", session.ID))
}

// UnifiedHandler returns an HTTP handler that routes between WebSocket control
// connections and HTTP proxy requests based on the request path.
func (s *Server) UnifiedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this is a WebSocket upgrade request for tunnel control
		if r.URL.Path == "/tunnel" || r.URL.Path == "/_tunnel" {
			if websocket.IsWebSocketUpgrade(r) {
				s.controlPlane.HandleWebSocket(w, r)
				return
			}
			http.Error(w, "WebSocket upgrade required", http.StatusUpgradeRequired)
			return
		}

		// Health check endpoint
		if r.URL.Path == "/health" || r.URL.Path == "/_health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"status":"ok","sessions":%d,"tunnels":%d}`,
				s.controlPlane.GetSessionCount(),
				s.registry.GetTunnelCount())
			return
		}

		// API endpoints
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.handleAPI(w, r)
			return
		}

		// Dashboard static files
		if strings.HasPrefix(r.URL.Path, "/dashboard") {
			s.serveDashboard(w, r)
			return
		}

		// Serve assets for dashboard (Vite builds with absolute paths)
		if strings.HasPrefix(r.URL.Path, "/assets/") || r.URL.Path == "/vite.svg" {
			http.FileServer(http.Dir("./web/dist")).ServeHTTP(w, r)
			return
		}

		// Otherwise, handle as HTTP proxy request
		s.httpProxy.ServeHTTP(w, r)
	})
}

// handleAPI routes API requests
func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	// CORS headers - use configured allowed origins
	origin := r.Header.Get("Origin")
	if origin != "" && s.config.IsOriginAllowed(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if s.config.CORS.AllowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch {
	// Auth endpoints (no auth required)
	case r.URL.Path == "/api/auth/register" && r.Method == "POST":
		s.api.HandleRegister(w, r)
	case r.URL.Path == "/api/auth/login" && r.Method == "POST":
		s.api.HandleLogin(w, r)

	// Tunnel endpoints
	case r.URL.Path == "/api/tunnels" && r.Method == "GET":
		AuthMiddleware(s.api.HandleListTunnels)(w, r)
	case r.URL.Path == "/api/tunnels" && r.Method == "POST":
		AuthMiddleware(s.api.HandleReserve)(w, r)

	// Request inspector endpoints
	case strings.HasPrefix(r.URL.Path, "/api/requests/") && r.Method == "GET":
		AuthMiddleware(s.api.HandleGetRequestLogs)(w, r)

	// Organization endpoints
	case r.URL.Path == "/api/orgs" && r.Method == "GET":
		AuthMiddleware(s.api.HandleListOrganizations)(w, r)
	case r.URL.Path == "/api/orgs" && r.Method == "POST":
		AuthMiddleware(s.api.HandleCreateOrganization)(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/orgs/") && strings.HasSuffix(r.URL.Path, "/members") && r.Method == "GET":
		AuthMiddleware(s.api.HandleGetOrganizationMembers)(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/orgs/") && strings.HasSuffix(r.URL.Path, "/members") && r.Method == "POST":
		AuthMiddleware(s.api.HandleAddOrganizationMember)(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/orgs/") && r.Method == "GET":
		AuthMiddleware(s.api.HandleGetOrganization)(w, r)

	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// serveDashboard serves the React dashboard
func (s *Server) serveDashboard(w http.ResponseWriter, r *http.Request) {
	// Serve static files from web/dist
	fs := http.FileServer(http.Dir("./web/dist"))
	http.StripPrefix("/dashboard", fs).ServeHTTP(w, r)
}

// ServeHTTP makes HTTPProxy implement http.Handler
func (p *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.handleRequest(w, r)
}

// StartUnified starts a unified HTTP server that handles both control plane
// and HTTP proxy on a single port.
func (s *Server) StartUnified() error {
	s.logger.Info("starting unified server",
		slog.String("domain", s.config.Domain),
		slog.String("addr", s.config.HTTPAddr))

	// Start the unified HTTP server
	server := &http.Server{
		Addr:              s.config.HTTPAddr,
		Handler:           s.UnifiedHandler(),
		ReadTimeout:       s.config.Timeouts.ReadTimeout,
		WriteTimeout:      s.config.Timeouts.WriteTimeout,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       s.config.Timeouts.IdleTimeout,
	}

	s.logger.Info("unified server listening",
		slog.String("addr", s.config.HTTPAddr),
		slog.String("tunnel_endpoint", "/tunnel"))

	return server.ListenAndServe()
}
