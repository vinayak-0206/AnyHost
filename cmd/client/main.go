package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyhost/gotunnel/internal/client"
	"github.com/anyhost/gotunnel/internal/common"
	"github.com/anyhost/gotunnel/internal/protocol"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	// Default server - change this to your deployed server
	DefaultServer = "wss://anyhost-tunnel.fly.dev"
	ConfigDir     = ".gotunnel"
	ConfigFile    = "config.yaml"
)

// CLIConfig stores the persistent configuration
type CLIConfig struct {
	AuthToken string `yaml:"authtoken"`
	Server    string `yaml:"server,omitempty"`
}

var (
	logLevel  string
	subdomain string
	server    string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "gotunnel",
	Short: "Expose local servers to the internet",
	Long: `GoTunnel - Expose local servers to the internet instantly.

Quick start:
  1. gotunnel config add-authtoken <your-token>
  2. gotunnel http 3000

Your local server on port 3000 will be available on the internet!`,
}

// ============ HTTP Command ============

var httpCmd = &cobra.Command{
	Use:   "http <port>",
	Short: "Expose a local HTTP server to the internet",
	Long: `Expose a local HTTP server to the internet.

Examples:
  gotunnel http 3000                    # Expose port 3000 with random subdomain
  gotunnel http 8080 --subdomain myapp  # Expose port 8080 as myapp.server.com
  gotunnel http 3000 --server wss://custom.server.com`,
	Args: cobra.ExactArgs(1),
	RunE: runHTTP,
}

func init() {
	httpCmd.Flags().StringVar(&subdomain, "subdomain", "", "Request a specific subdomain (default: auto-generated)")
	httpCmd.Flags().StringVar(&server, "server", "", "Server URL (default: from config or "+DefaultServer+")")
	httpCmd.Flags().StringVarP(&logLevel, "log-level", "l", "info", "Log level (debug, info, warn, error)")
	rootCmd.AddCommand(httpCmd)
}

func runHTTP(cmd *cobra.Command, args []string) error {
	port := 0
	if _, err := fmt.Sscanf(args[0], "%d", &port); err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port: %s", args[0])
	}

	// Load config
	cfg, err := loadCLIConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\n\nRun: gotunnel config add-authtoken <your-token>", err)
	}

	if cfg.AuthToken == "" {
		return fmt.Errorf("no auth token configured\n\nRun: gotunnel config add-authtoken <your-token>")
	}

	// Determine server
	serverURL := DefaultServer
	if cfg.Server != "" {
		serverURL = cfg.Server
	}
	if server != "" {
		serverURL = server
	}

	// Generate subdomain if not specified
	subdomainName := subdomain
	if subdomainName == "" {
		subdomainName = generateSubdomain()
	}

	// Setup logger
	logger := setupLogger(logLevel)

	// Build client config
	clientCfg := common.DefaultClientConfig()
	clientCfg.ServerAddr = serverURL
	clientCfg.Token = cfg.AuthToken
	clientCfg.LogLevel = logLevel
	clientCfg.Tunnels = []protocol.TunnelConfig{
		{
			Subdomain: subdomainName,
			LocalPort: port,
			LocalHost: "127.0.0.1",
			Protocol:  "http",
		},
	}

	// Validate
	if err := clientCfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Create and run tunnel
	tunnel, err := client.NewTunnel(clientCfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create tunnel: %w", err)
	}

	// Show connection info
	fmt.Println()
	fmt.Println("GoTunnel")
	fmt.Println()
	fmt.Printf("Forwarding:  https://%s → http://localhost:%d\n", formatURL(serverURL, subdomainName), port)
	fmt.Println()
	fmt.Println("Connections: Press Ctrl+C to stop")
	fmt.Println()

	// Register state change handler
	tunnel.OnStateChange(func(state client.TunnelState) {
		if state == client.TunnelStateConnected {
			logger.Debug("tunnel connected")
		} else if state == client.TunnelStateDisconnected {
			logger.Debug("tunnel disconnected, reconnecting...")
		}
	})

	return tunnel.Run()
}

// ============ Config Commands ============

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage gotunnel configuration",
}

var configAddTokenCmd = &cobra.Command{
	Use:   "add-authtoken <token>",
	Short: "Save authentication token",
	Long: `Save your authentication token to the config file.

This only needs to be done once. The token will be saved to ~/.gotunnel/config.yaml

Example:
  gotunnel config add-authtoken abc123xyz`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigAddToken,
}

var configSetServerCmd = &cobra.Command{
	Use:   "set-server <url>",
	Short: "Set the default server URL",
	Long: `Set the default server URL.

Example:
  gotunnel config set-server wss://my-tunnel-server.com`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigSetServer,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE:  runConfigShow,
}

func init() {
	configCmd.AddCommand(configAddTokenCmd)
	configCmd.AddCommand(configSetServerCmd)
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigAddToken(cmd *cobra.Command, args []string) error {
	token := args[0]

	cfg, _ := loadCLIConfig() // Ignore error, will create new config
	if cfg == nil {
		cfg = &CLIConfig{}
	}
	cfg.AuthToken = token

	if err := saveCLIConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println("Auth token saved to", getConfigPath())
	return nil
}

func runConfigSetServer(cmd *cobra.Command, args []string) error {
	serverURL := args[0]

	cfg, _ := loadCLIConfig()
	if cfg == nil {
		cfg = &CLIConfig{}
	}
	cfg.Server = serverURL

	if err := saveCLIConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println("Server URL saved to", getConfigPath())
	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg, err := loadCLIConfig()
	if err != nil {
		fmt.Println("No configuration found.")
		fmt.Println("Run: gotunnel config add-authtoken <your-token>")
		return nil
	}

	fmt.Println("Config file:", getConfigPath())
	fmt.Println()

	serverURL := DefaultServer
	if cfg.Server != "" {
		serverURL = cfg.Server
	}
	fmt.Println("Server:", serverURL)

	if cfg.AuthToken != "" {
		// Mask the token
		masked := cfg.AuthToken[:4] + "..." + cfg.AuthToken[len(cfg.AuthToken)-4:]
		fmt.Println("Token:", masked)
	} else {
		fmt.Println("Token: (not set)")
	}

	return nil
}

// ============ Version Command ============

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("gotunnel version 1.0.0")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

// ============ Helper Functions ============

func getConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ConfigDir
	}
	return filepath.Join(home, ConfigDir)
}

func getConfigPath() string {
	return filepath.Join(getConfigDir(), ConfigFile)
}

func loadCLIConfig() (*CLIConfig, error) {
	data, err := os.ReadFile(getConfigPath())
	if err != nil {
		return nil, err
	}

	var cfg CLIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func saveCLIConfig(cfg *CLIConfig) error {
	// Create config directory if it doesn't exist
	configDir := getConfigDir()
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(getConfigPath(), data, 0600)
}

func generateSubdomain() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func formatURL(serverURL, subdomain string) string {
	// Extract host from server URL for display
	host := serverURL
	host = strings.TrimPrefix(host, "wss://")
	host = strings.TrimPrefix(host, "ws://")
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")

	// For path-based routing
	return host + "/" + subdomain
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

	handler := slog.NewTextHandler(os.Stderr, opts)
	return slog.New(handler)
}
