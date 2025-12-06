package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/anyhost/gotunnel/internal/client"
	"github.com/anyhost/gotunnel/internal/common"
	"github.com/anyhost/gotunnel/internal/protocol"
	"github.com/spf13/cobra"
)

var (
	configFile string
	logLevel   string
	serverAddr string
	token      string
	subdomain  string
	localPort  int
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "gotunnel",
	Short: "GoTunnel Client - Tunnel local services to the internet",
	Long: `GoTunnel Client connects to a GoTunnel server and exposes local services
to the internet through secure tunnels.

Examples:
  # Expose local port 3000 with subdomain "myapp"
  gotunnel --server tunnel.example.com:9000 --token mytoken --subdomain myapp --port 3000

  # Use a configuration file
  gotunnel --config ./tunnel.yaml`,
	RunE: runClient,
}

func init() {
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")
	rootCmd.Flags().StringVarP(&logLevel, "log-level", "l", "info", "Log level (debug, info, warn, error)")
	rootCmd.Flags().StringVarP(&serverAddr, "server", "s", "localhost:9000", "Server address")
	rootCmd.Flags().StringVarP(&token, "token", "t", "", "Authentication token")
	rootCmd.Flags().StringVar(&subdomain, "subdomain", "", "Subdomain to request")
	rootCmd.Flags().IntVarP(&localPort, "port", "p", 0, "Local port to expose")
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show tunnel status",
	RunE:  showStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runClient(cmd *cobra.Command, args []string) error {
	// Setup logger
	logger := setupLogger(logLevel)

	// Load or create configuration
	var cfg *common.ClientConfig
	var err error

	if configFile != "" {
		cfg, err = common.LoadClientConfig(configFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		logger.Info("loaded configuration", slog.String("file", configFile))
	} else {
		// Build config from command line flags
		if token == "" {
			return fmt.Errorf("token is required (use --token or --config)")
		}
		if subdomain == "" && localPort == 0 {
			return fmt.Errorf("subdomain and port are required (use --subdomain and --port, or --config)")
		}

		cfg = common.DefaultClientConfig()
		cfg.ServerAddr = serverAddr
		cfg.Token = token
		cfg.LogLevel = logLevel

		if subdomain != "" && localPort > 0 {
			cfg.Tunnels = []protocol.TunnelConfig{
				{
					Subdomain: subdomain,
					LocalPort: localPort,
					LocalHost: "127.0.0.1",
					Protocol:  "http",
				},
			}
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Create and run tunnel
	tunnel, err := client.NewTunnel(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create tunnel: %w", err)
	}

	// Register state change handler
	tunnel.OnStateChange(func(state client.TunnelState) {
		logger.Info("tunnel state changed", slog.String("state", state.String()))
	})

	logger.Info("starting tunnel client",
		slog.String("server", cfg.ServerAddr),
		slog.Int("tunnels", len(cfg.Tunnels)))

	return tunnel.Run()
}

func showStatus(cmd *cobra.Command, args []string) error {
	fmt.Println("Tunnel status not yet implemented")
	fmt.Println("This feature will show active tunnels and connection status.")
	return nil
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
