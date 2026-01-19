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
	NumURLs       = 3 // Number of URLs to generate
)

var (
	subdomain string
	server    string
	urls      int
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

Unlike ngrok/cloudflare, AnyHost gives you MULTIPLE URLs for the same tunnel.
Share different URLs with different people or teams!

Examples:
  gotunnel 3000                      # Expose port 3000 with 3 URLs
  gotunnel 8080 --urls 5             # Expose with 5 different URLs
  gotunnel 3000 --subdomain myapp    # Expose with one custom subdomain`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTunnel,
}

func init() {
	rootCmd.Flags().StringVar(&subdomain, "subdomain", "", "Request a specific subdomain (disables multi-URL)")
	rootCmd.Flags().StringVar(&server, "server", DefaultServer, "Tunnel server URL")
	rootCmd.Flags().IntVar(&urls, "urls", NumURLs, "Number of URLs to generate (default 3)")
}

func runTunnel(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}

	port := 0
	if _, err := fmt.Sscanf(args[0], "%d", &port); err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port: %s", args[0])
	}

	// Generate subdomains
	var subdomains []string
	if subdomain != "" {
		// User specified a subdomain, use only that
		subdomains = []string{subdomain}
	} else {
		// Generate multiple random subdomains
		for i := 0; i < urls; i++ {
			subdomains = append(subdomains, generateSubdomain())
		}
	}

	// Silent logger (only errors)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Build client config with multiple tunnels
	cfg := common.DefaultClientConfig()
	cfg.ServerAddr = server
	cfg.Token = "public" // Public access token

	for _, sub := range subdomains {
		cfg.Tunnels = append(cfg.Tunnels, protocol.TunnelConfig{
			Subdomain: sub,
			LocalPort: port,
			LocalHost: "127.0.0.1",
			Protocol:  "http",
		})
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	tunnel, err := client.NewTunnel(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Show the URLs
	host := strings.TrimPrefix(server, "wss://")
	host = strings.TrimPrefix(host, "ws://")

	fmt.Println()
	fmt.Println("  AnyHost - Your tunnel is ready!")
	fmt.Println()
	fmt.Printf("  Forwarding to http://localhost:%d\n", port)
	fmt.Println()
	fmt.Println("  Public URLs (share any of these):")
	for i, sub := range subdomains {
		fmt.Printf("    [%d] https://%s/%s\n", i+1, host, sub)
	}
	fmt.Println()
	fmt.Println("  Press Ctrl+C to stop")
	fmt.Println()

	return tunnel.Run()
}

func generateSubdomain() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
