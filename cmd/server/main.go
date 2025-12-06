package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/anyhost/gotunnel/internal/common"
	"github.com/anyhost/gotunnel/internal/server"
	"github.com/spf13/cobra"
)

var (
	configFile string
	logLevel   string
	domain     string
	controlAddr string
	httpAddr   string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "gotunnel-server",
	Short: "GoTunnel Server - Expose local services to the internet",
	Long: `GoTunnel Server is the server component of the GoTunnel tunneling service.
It accepts connections from tunnel clients and routes incoming HTTP traffic
to the appropriate client based on subdomain.`,
	RunE: runServer,
}

func init() {
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")
	rootCmd.Flags().StringVarP(&logLevel, "log-level", "l", "info", "Log level (debug, info, warn, error)")
	rootCmd.Flags().StringVarP(&domain, "domain", "d", "localhost", "Base domain for subdomains")
	rootCmd.Flags().StringVar(&controlAddr, "control-addr", ":9000", "Address for client connections")
	rootCmd.Flags().StringVar(&httpAddr, "http-addr", ":8080", "Address for HTTP traffic")
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
		// Override with command line flags
		cfg.Domain = domain
		cfg.ControlAddr = controlAddr
		cfg.HTTPAddr = httpAddr
		cfg.LogLevel = logLevel
	}

	// Create and run server
	srv, err := server.NewServer(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// For development/testing, add a default token if no token file is configured
	if cfg.Auth.Mode == "token" && cfg.Auth.TokenFile == "" {
		logger.Warn("no token file configured, adding default development token")
		if err := srv.AddToken("dev-token", "dev-user"); err != nil {
			logger.Warn("failed to add development token", slog.Any("error", err))
		}
	}

	return srv.Run()
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
