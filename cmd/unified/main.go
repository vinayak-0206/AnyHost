package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/anyhost/gotunnel/internal/common"
	"github.com/anyhost/gotunnel/internal/server"
	"github.com/spf13/cobra"
)

var (
	configFile string
	logLevel   string
	domain     string
	addr       string
	port       string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "gotunnel-unified",
	Short: "GoTunnel Unified Server - Single port for control + proxy",
	Long: `GoTunnel Unified Server runs both the control plane (WebSocket) and
HTTP proxy on a single port. This is ideal for deployment on platforms
that only allow a single port (Replit, Render, etc).

Clients connect via WebSocket to /tunnel endpoint.
HTTP proxy requests are handled on all other paths.`,
	RunE: runServer,
}

func init() {
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")
	rootCmd.Flags().StringVarP(&logLevel, "log-level", "l", "info", "Log level (debug, info, warn, error)")
	rootCmd.Flags().StringVarP(&domain, "domain", "d", "", "Base domain for subdomains")
	rootCmd.Flags().StringVar(&addr, "addr", "", "Address to listen on (e.g., :8080)")
	rootCmd.Flags().StringVar(&port, "port", "", "Port to listen on (alternative to --addr)")
}

func runServer(cmd *cobra.Command, args []string) error {
	// Setup logger
	logger := setupLogger(logLevel)

	// Load or create configuration
	var cfg *common.ServerConfig
	var err error

	if configFile != "" {
		cfg, err = common.LoadServerConfig(configFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		logger.Info("loaded configuration", slog.String("file", configFile))
	} else {
		cfg = common.DefaultServerConfig()
	}

	// Override with command line flags or environment variables
	if domain != "" {
		cfg.Domain = domain
	}
	if envDomain := os.Getenv("DOMAIN"); envDomain != "" {
		cfg.Domain = envDomain
	}

	if addr != "" {
		cfg.HTTPAddr = addr
	} else if port != "" {
		cfg.HTTPAddr = ":" + port
	}
	if envPort := os.Getenv("PORT"); envPort != "" {
		cfg.HTTPAddr = ":" + envPort
	}

	cfg.LogLevel = logLevel

	// For unified mode, we don't use the control addr separately
	cfg.ControlAddr = ""

	logger.Info("configuration",
		slog.String("domain", cfg.Domain),
		slog.String("addr", cfg.HTTPAddr))

	// Create server
	srv, err := server.NewServer(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Add default dev token if no token file
	if cfg.Auth.Mode == "token" && cfg.Auth.TokenFile == "" {
		logger.Warn("no token file configured, adding default development token")
		if err := srv.AddToken("dev-token", "dev-user"); err != nil {
			logger.Warn("failed to add development token", slog.Any("error", err))
		}
	}

	// Create HTTP server with unified handler
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.UnifiedHandler(),
		ReadTimeout:       cfg.Timeouts.ReadTimeout,
		WriteTimeout:      cfg.Timeouts.WriteTimeout,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       cfg.Timeouts.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		logger.Info("unified server listening",
			slog.String("addr", cfg.HTTPAddr),
			slog.String("domain", cfg.Domain),
			slog.String("tunnel_endpoint", "/tunnel"),
			slog.String("health_endpoint", "/health"))

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down...")
	return httpServer.Close()
}

func setupLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	handler := slog.NewTextHandler(os.Stdout, opts)
	return slog.New(handler)
}
