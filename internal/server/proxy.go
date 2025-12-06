package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anyhost/gotunnel/internal/common"
)

// HTTPProxy handles incoming HTTP requests and proxies them to tunnel clients.
type HTTPProxy struct {
	config       *common.ServerConfig
	registry     *Registry
	controlPlane *ControlPlane
	httpServer   *http.Server
	httpsServer  *http.Server
	logger       *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewHTTPProxy creates a new HTTP proxy.
func NewHTTPProxy(cfg *common.ServerConfig, registry *Registry, cp *ControlPlane, logger *slog.Logger) *HTTPProxy {
	ctx, cancel := context.WithCancel(context.Background())

	return &HTTPProxy{
		config:       cfg,
		registry:     registry,
		controlPlane: cp,
		logger:       logger.With(slog.String("component", "http_proxy")),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start starts the HTTP proxy servers.
func (p *HTTPProxy) Start() error {
	handler := http.HandlerFunc(p.handleRequest)

	// Start HTTP server
	if p.config.HTTPAddr != "" {
		p.httpServer = &http.Server{
			Addr:              p.config.HTTPAddr,
			Handler:           handler,
			ReadTimeout:       p.config.Timeouts.ReadTimeout,
			WriteTimeout:      p.config.Timeouts.WriteTimeout,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       p.config.Timeouts.IdleTimeout,
		}

		listener, err := net.Listen("tcp", p.config.HTTPAddr)
		if err != nil {
			return fmt.Errorf("failed to listen on %s: %w", p.config.HTTPAddr, err)
		}

		p.logger.Info("HTTP proxy listening", slog.String("addr", p.config.HTTPAddr))

		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			if err := p.httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
				p.logger.Error("HTTP server error", slog.Any("error", err))
			}
		}()
	}

	// Start HTTPS server (if configured)
	if p.config.HTTPSAddr != "" && p.config.TLS.Enabled {
		p.httpsServer = &http.Server{
			Addr:              p.config.HTTPSAddr,
			Handler:           handler,
			ReadTimeout:       p.config.Timeouts.ReadTimeout,
			WriteTimeout:      p.config.Timeouts.WriteTimeout,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       p.config.Timeouts.IdleTimeout,
		}

		p.logger.Info("HTTPS proxy listening", slog.String("addr", p.config.HTTPSAddr))

		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			if err := p.httpsServer.ListenAndServeTLS(p.config.TLS.CertFile, p.config.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
				p.logger.Error("HTTPS server error", slog.Any("error", err))
			}
		}()
	}

	return nil
}

// Stop gracefully stops the HTTP proxy.
func (p *HTTPProxy) Stop(gracePeriod time.Duration) error {
	p.logger.Info("stopping HTTP proxy", slog.Duration("grace_period", gracePeriod))

	ctx, cancel := context.WithTimeout(context.Background(), gracePeriod)
	defer cancel()

	p.cancel()

	var errs []error

	if p.httpServer != nil {
		if err := p.httpServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("HTTP server shutdown error: %w", err))
		}
	}

	if p.httpsServer != nil {
		if err := p.httpsServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("HTTPS server shutdown error: %w", err))
		}
	}

	p.wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errs)
	}

	p.logger.Info("HTTP proxy stopped")
	return nil
}

// handleRequest handles an incoming HTTP request.
func (p *HTTPProxy) handleRequest(w http.ResponseWriter, r *http.Request) {
	requestID := common.GenerateRequestID()
	logger := p.logger.With(
		slog.String("request_id", requestID),
		slog.String("host", r.Host),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("remote_addr", r.RemoteAddr),
	)

	// Check for WebSocket upgrade
	isWebSocket := isWebSocketUpgrade(r)
	if isWebSocket {
		logger.Debug("WebSocket upgrade request")
	}

	// Try to lookup tunnel by host (subdomain-based routing)
	entry, found := p.registry.LookupByHost(r.Host)

	// If not found by host, try path-based routing: /subdomain/...
	if !found {
		entry, found = p.lookupByPath(r)
	}

	// Also check X-Tunnel-Subdomain header as fallback
	if !found {
		if subdomain := r.Header.Get("X-Tunnel-Subdomain"); subdomain != "" {
			entry, found = p.registry.Lookup(subdomain)
		}
	}

	if !found {
		logger.Debug("no tunnel found for host or path")
		http.Error(w, "Tunnel not found", http.StatusNotFound)
		return
	}

	logger = logger.With(
		slog.String("subdomain", entry.Subdomain),
		slog.String("session_id", entry.Session.ID),
	)

	// Check if session is active
	if !entry.Session.IsActive() {
		logger.Warn("session is not active")
		http.Error(w, "Tunnel unavailable", http.StatusServiceUnavailable)
		return
	}

	// Open stream to client
	stream, err := p.controlPlane.ProxyRequest(entry, requestID)
	if err != nil {
		logger.Error("failed to open stream", slog.Any("error", err))
		http.Error(w, "Failed to connect to tunnel", http.StatusBadGateway)
		return
	}
	defer stream.Close()

	// Handle WebSocket upgrade differently
	if isWebSocket {
		p.handleWebSocket(w, r, stream, logger)
		return
	}

	// Forward request to client
	if err := p.forwardRequest(stream, r); err != nil {
		logger.Error("failed to forward request", slog.Any("error", err))
		http.Error(w, "Failed to forward request", http.StatusBadGateway)
		return
	}

	// Read response from client
	if err := p.forwardResponse(w, stream, logger); err != nil {
		logger.Error("failed to forward response", slog.Any("error", err))
		// Response may have already started, so we can't send an error
		return
	}

	logger.Debug("request completed")
}

// forwardRequest forwards an HTTP request to the tunnel stream.
func (p *HTTPProxy) forwardRequest(stream io.Writer, r *http.Request) error {
	// Write request line
	requestLine := fmt.Sprintf("%s %s %s\r\n", r.Method, r.URL.RequestURI(), r.Proto)
	if _, err := stream.Write([]byte(requestLine)); err != nil {
		return fmt.Errorf("failed to write request line: %w", err)
	}

	// Write headers
	if err := r.Header.Write(stream); err != nil {
		return fmt.Errorf("failed to write headers: %w", err)
	}

	// Write host header (may not be in headers)
	if r.Header.Get("Host") == "" {
		if _, err := fmt.Fprintf(stream, "Host: %s\r\n", r.Host); err != nil {
			return fmt.Errorf("failed to write Host header: %w", err)
		}
	}

	// Add X-Forwarded headers
	if _, err := fmt.Fprintf(stream, "X-Forwarded-For: %s\r\n", getClientIP(r)); err != nil {
		return fmt.Errorf("failed to write X-Forwarded-For: %w", err)
	}
	if _, err := fmt.Fprintf(stream, "X-Forwarded-Proto: %s\r\n", getScheme(r)); err != nil {
		return fmt.Errorf("failed to write X-Forwarded-Proto: %w", err)
	}

	// End headers
	if _, err := stream.Write([]byte("\r\n")); err != nil {
		return fmt.Errorf("failed to write header terminator: %w", err)
	}

	// Copy body if present
	if r.Body != nil && r.ContentLength != 0 {
		if _, err := io.Copy(stream, r.Body); err != nil {
			return fmt.Errorf("failed to copy request body: %w", err)
		}
	}

	return nil
}

// forwardResponse reads an HTTP response from the tunnel and forwards it to the client.
func (p *HTTPProxy) forwardResponse(w http.ResponseWriter, stream io.Reader, logger *slog.Logger) error {
	reader := bufio.NewReader(stream)

	// Read response
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}
	defer resp.Body.Close()

	// Copy headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Write status code
	w.WriteHeader(resp.StatusCode)

	// Copy body
	if _, err := io.Copy(w, resp.Body); err != nil {
		// Connection may have been closed by client
		if !isConnectionReset(err) {
			return fmt.Errorf("failed to copy response body: %w", err)
		}
	}

	return nil
}

// handleWebSocket handles WebSocket upgrade requests.
func (p *HTTPProxy) handleWebSocket(w http.ResponseWriter, r *http.Request, stream io.ReadWriteCloser, logger *slog.Logger) {
	// Hijack the connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		logger.Error("response writer does not support hijacking")
		http.Error(w, "WebSocket not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		logger.Error("failed to hijack connection", slog.Any("error", err))
		http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Forward the original request to the tunnel
	if err := p.forwardRequest(stream, r); err != nil {
		logger.Error("failed to forward WebSocket upgrade request", slog.Any("error", err))
		return
	}

	// Bidirectional copy
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Tunnel
	go func() {
		defer wg.Done()
		io.Copy(stream, clientConn)
	}()

	// Tunnel -> Client
	go func() {
		defer wg.Done()
		io.Copy(clientConn, stream)
	}()

	wg.Wait()
	logger.Debug("WebSocket connection closed")
}

// isWebSocketUpgrade checks if the request is a WebSocket upgrade request.
func isWebSocketUpgrade(r *http.Request) bool {
	connection := strings.ToLower(r.Header.Get("Connection"))
	upgrade := strings.ToLower(r.Header.Get("Upgrade"))
	return strings.Contains(connection, "upgrade") && upgrade == "websocket"
}

// getClientIP extracts the client IP from the request.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For first (if behind a proxy)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to remote address
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// getScheme determines the request scheme (http/https).
func getScheme(r *http.Request) string {
	// Check X-Forwarded-Proto
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}

	// Check if TLS
	if r.TLS != nil {
		return "https"
	}

	return "http"
}

// isConnectionReset checks if the error is a connection reset.
func isConnectionReset(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "broken pipe")
}

// lookupByPath extracts subdomain from path and looks up the tunnel.
// Supports format: /subdomain/... which gets rewritten to /...
func (p *HTTPProxy) lookupByPath(r *http.Request) (*TunnelEntry, bool) {
	path := r.URL.Path
	if path == "" || path == "/" {
		return nil, false
	}

	// Remove leading slash and split
	path = strings.TrimPrefix(path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 {
		return nil, false
	}

	subdomain := strings.ToLower(parts[0])
	entry, found := p.registry.Lookup(subdomain)
	if !found {
		return nil, false
	}

	// Rewrite the path to remove the subdomain prefix
	if len(parts) > 1 {
		r.URL.Path = "/" + parts[1]
	} else {
		r.URL.Path = "/"
	}

	return entry, true
}
