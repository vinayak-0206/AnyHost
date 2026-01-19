package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/anyhost/gotunnel/internal/common"
	"github.com/anyhost/gotunnel/internal/database"
)

// Server is the main tunnel server that coordinates all components.
type Server struct {
	config       *common.ServerConfig
	registry     *Registry
	auth         Authenticator
	controlPlane *ControlPlane
	httpProxy    *HTTPProxy
	logger       *slog.Logger
	db  *database.DB
    api *API

	ctx    context.Context
	cancel context.CancelFunc
}

// NewServer creates a new tunnel server.
func NewServer(cfg *common.ServerConfig, logger *slog.Logger) (*Server, error) {
	// Use configured database path, with fallback to default
	dbPath := cfg.DatabasePath
	if dbPath == "" {
		dbPath = "./gotunnel.db"
	}
	db, err := database.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create registry
	registry := NewRegistry(cfg.Domain, cfg.ReservedSubdomains)

	// Set database as owner checker for subdomain ownership validation
	registry.SetOwnerChecker(db)

	// Create base authenticator from config
	baseAuth, err := NewAuthenticatorFromConfig(&cfg.Auth)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create authenticator: %w", err)
	}

	// Wrap with database authenticator to also accept user IDs from dashboard
	auth := NewDatabaseAuthenticator(db, baseAuth)

	// Create control plane
	controlPlane := NewControlPlane(cfg, registry, auth, logger)

	// Create HTTP proxy
	httpProxy := NewHTTPProxy(cfg, registry, controlPlane, logger)

	api := NewAPI(db, registry, controlPlane)

	return &Server{
		config:       cfg,
		registry:     registry,
		auth:         auth,
		controlPlane: controlPlane,
		httpProxy:    httpProxy,
		logger:       logger.With(slog.String("component", "server")),
		ctx:          ctx,
		cancel:       cancel,
		db:           db,
		api:          api,
	}, nil
}

func (s *Server) setupRoutes() http.Handler {
    mux := http.NewServeMux()

    // API Routes
    mux.HandleFunc("POST /api/auth/register", s.api.HandleRegister)
    mux.HandleFunc("POST /api/auth/login", s.api.HandleLogin)
    mux.HandleFunc("POST /api/tunnels", AuthMiddleware(s.api.HandleReserve))
    mux.HandleFunc("GET /api/tunnels", AuthMiddleware(s.api.HandleListTunnels))

    // Static Dashboard Files (React Build)
    fileServer := http.FileServer(http.Dir("./web/dist"))
    mux.Handle("/dashboard/", http.StripPrefix("/dashboard/", fileServer))

    // Fallback to existing HTTP Proxy
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/dashboard/") {
            mux.ServeHTTP(w, r)
            return
        }
        // If Host matches "dashboard.domain.com", serve dashboard
        if strings.HasPrefix(r.Host, "dashboard.") {
             mux.ServeHTTP(w, r)
             return
        }
        
        // Else, existing Proxy logic
        s.httpProxy.ServeHTTP(w, r)
    })
}

// Start starts all server components.
func (s *Server) Start() error {
	s.logger.Info("starting server",
		slog.String("domain", s.config.Domain),
		slog.String("control_addr", s.config.ControlAddr),
		slog.String("http_addr", s.config.HTTPAddr))

	// Start control plane
	if err := s.controlPlane.Start(); err != nil {
		return fmt.Errorf("failed to start control plane: %w", err)
	}

	// Start HTTP proxy
	if err := s.httpProxy.Start(); err != nil {
		s.controlPlane.Stop(5 * time.Second)
		return fmt.Errorf("failed to start HTTP proxy: %w", err)
	}

	s.logger.Info("server started")
	return nil
}

// Stop gracefully stops all server components.
func (s *Server) Stop(gracePeriod time.Duration) error {
	s.logger.Info("stopping server", slog.Duration("grace_period", gracePeriod))

	s.cancel()

	// Notify clients of shutdown
	s.controlPlane.BroadcastShutdown("server shutting down", int(gracePeriod.Milliseconds()))

	var errs []error

	// Stop HTTP proxy first (stop accepting new requests)
	if err := s.httpProxy.Stop(gracePeriod); err != nil {
		errs = append(errs, fmt.Errorf("HTTP proxy: %w", err))
	}

	// Stop control plane (close client connections)
	if err := s.controlPlane.Stop(gracePeriod); err != nil {
		errs = append(errs, fmt.Errorf("control plane: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errs)
	}

	s.logger.Info("server stopped")
	return nil
}

// Run starts the server and blocks until interrupted.
func (s *Server) Run() error {
	if err := s.Start(); err != nil {
		return err
	}

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		s.logger.Info("received signal", slog.String("signal", sig.String()))
	case <-s.ctx.Done():
		s.logger.Info("context cancelled")
	}

	return s.Stop(30 * time.Second)
}

// Registry returns the server's registry.
func (s *Server) Registry() *Registry {
	return s.registry
}

// GetStats returns server statistics.
func (s *Server) GetStats() ServerStats {
	return ServerStats{
		ActiveSessions: s.controlPlane.GetSessionCount(),
		ActiveTunnels:  s.registry.GetTunnelCount(),
	}
}

// ServerStats holds server statistics.
type ServerStats struct {
	ActiveSessions int
	ActiveTunnels  int
}

// AddToken adds a token to the authenticator (if using token auth).
func (s *Server) AddToken(token, userID string) error {
	// Check if it's a direct TokenAuthenticator
	if ta, ok := s.auth.(*TokenAuthenticator); ok {
		ta.AddToken(token, userID)
		return nil
	}
	// Check if it's a DatabaseAuthenticator with a TokenAuthenticator fallback
	if da, ok := s.auth.(*DatabaseAuthenticator); ok {
		if ta, ok := da.fallback.(*TokenAuthenticator); ok {
			ta.AddToken(token, userID)
			return nil
		}
	}
	return fmt.Errorf("server is not using token authentication")
}
