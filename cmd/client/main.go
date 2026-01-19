package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/anyhost/gotunnel/internal/client"
	"github.com/anyhost/gotunnel/internal/common"
	"github.com/anyhost/gotunnel/internal/protocol"
	"github.com/spf13/cobra"
)

const (
	DefaultServer = "wss://anyhost-tunnel.fly.dev"
)

var (
	subdomain string
	server    string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "gotunnel [port]",
	Short: "Expose local servers to the internet",
	Long: `Expose local servers to the internet instantly.

Examples:
  gotunnel 3000                      # Expose port 3000
  gotunnel 8080 --subdomain myapp    # Expose with custom subdomain`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTunnel,
}

func init() {
	rootCmd.Flags().StringVar(&subdomain, "subdomain", "", "Request a specific subdomain")
	rootCmd.Flags().StringVar(&server, "server", DefaultServer, "Tunnel server URL")
}

func runTunnel(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}

	port := 0
	if _, err := fmt.Sscanf(args[0], "%d", &port); err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port: %s", args[0])
	}

	// Generate subdomain if not specified
	subdomainName := subdomain
	if subdomainName == "" {
		subdomainName = generateSubdomain()
	}

	// Silent logger (only errors)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Build client config
	cfg := common.DefaultClientConfig()
	cfg.ServerAddr = server
	cfg.Token = "public"  // Public access token
	cfg.Tunnels = []protocol.TunnelConfig{
		{
			Subdomain: subdomainName,
			LocalPort: port,
			LocalHost: "127.0.0.1",
			Protocol:  "http",
		},
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	tunnel, err := client.NewTunnel(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Show the URL
	host := strings.TrimPrefix(server, "wss://")
	host = strings.TrimPrefix(host, "ws://")

	fmt.Printf("\n  Your tunnel is ready!\n\n")
	fmt.Printf("  https://%s/%s  →  http://localhost:%d\n\n", host, subdomainName, port)

	return tunnel.Run()
}

func generateSubdomain() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
