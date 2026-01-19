package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/anyhost/gotunnel/internal/client"
	"github.com/anyhost/gotunnel/internal/common"
	"github.com/anyhost/gotunnel/internal/protocol"
	"github.com/mdp/qrterminal/v3"
	"github.com/spf13/cobra"
)

const (
	DefaultServer = "wss://anyhost-tunnel.fly.dev"
	NumURLs       = 3
)

var (
	subdomain  string
	server     string
	urls       int
	qrCode     bool
	password   string
	basicAuth  string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "gotunnel [port]",
	Short: "Expose local servers to the internet",
	Long: `AnyHost - Expose local servers to the internet instantly.

Better than ngrok/cloudflare:
  • Multiple URLs per tunnel (share different links with different people)
  • QR code for instant mobile testing
  • Password protection built-in
  • Self-hostable for security/compliance

Examples:
  gotunnel 3000                      # Expose with 3 URLs
  gotunnel 3000 --qr                 # Show QR code for mobile
  gotunnel 3000 --urls 5             # Generate 5 URLs
  gotunnel 3000 --password secret    # Password protect the tunnel
  gotunnel 3000 --auth user:pass     # Basic auth protection`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTunnel,
}

func init() {
	rootCmd.Flags().StringVar(&subdomain, "subdomain", "", "Request a specific subdomain")
	rootCmd.Flags().StringVar(&server, "server", DefaultServer, "Tunnel server URL")
	rootCmd.Flags().IntVar(&urls, "urls", NumURLs, "Number of URLs to generate")
	rootCmd.Flags().BoolVar(&qrCode, "qr", false, "Show QR code for first URL (great for mobile)")
	rootCmd.Flags().StringVar(&password, "password", "", "Password protect the tunnel")
	rootCmd.Flags().StringVar(&basicAuth, "auth", "", "Basic auth (user:pass)")
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
		subdomains = []string{subdomain}
	} else {
		for i := 0; i < urls; i++ {
			subdomains = append(subdomains, generateSubdomain())
		}
	}

	// Silent logger
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Build client config
	cfg := common.DefaultClientConfig()
	cfg.ServerAddr = server
	cfg.Token = "public"

	for _, sub := range subdomains {
		tunnelCfg := protocol.TunnelConfig{
			Subdomain: sub,
			LocalPort: port,
			LocalHost: "127.0.0.1",
			Protocol:  "http",
		}
		cfg.Tunnels = append(cfg.Tunnels, tunnelCfg)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	tunnel, err := client.NewTunnel(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Get host for URLs
	host := strings.TrimPrefix(server, "wss://")
	host = strings.TrimPrefix(host, "ws://")

	// Build URLs (with auth params if specified)
	var fullURLs []string
	for _, sub := range subdomains {
		url := fmt.Sprintf("https://%s/%s", host, sub)
		if password != "" {
			url += "?password=" + password
		}
		fullURLs = append(fullURLs, url)
	}

	// Display output
	printHeader()
	fmt.Printf("  │ Local:    http://localhost:%d\n", port)
	fmt.Println("  │")
	fmt.Println("  │ Public URLs:")
	for i, url := range fullURLs {
		fmt.Printf("  │   [%d] %s\n", i+1, url)
	}

	if password != "" {
		fmt.Println("  │")
		fmt.Printf("  │ 🔒 Password: %s\n", password)
	}
	if basicAuth != "" {
		fmt.Println("  │")
		fmt.Printf("  │ 🔐 Auth: %s\n", basicAuth)
	}

	printFooter()

	// Show QR code if requested
	if qrCode && len(fullURLs) > 0 {
		fmt.Println()
		fmt.Println("  Scan with your phone:")
		fmt.Println()
		qrterminal.GenerateWithConfig(fullURLs[0], qrterminal.Config{
			Level:     qrterminal.L,
			Writer:    os.Stdout,
			BlackChar: qrterminal.WHITE,
			WhiteChar: qrterminal.BLACK,
			QuietZone: 2,
		})
		fmt.Println()
	}

	return tunnel.Run()
}

func generateSubdomain() string {
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func generatePassword() string {
	bytes := make([]byte, 6)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)[:8]
}

func printHeader() {
	fmt.Println()
	fmt.Println("  ┌─────────────────────────────────────────────────┐")
	fmt.Println("  │             AnyHost Tunnel Ready                │")
	fmt.Println("  ├─────────────────────────────────────────────────┤")
}

func printFooter() {
	fmt.Println("  ├─────────────────────────────────────────────────┤")
	fmt.Println("  │ Press Ctrl+C to stop                            │")
	fmt.Println("  └─────────────────────────────────────────────────┘")
	fmt.Println()
}
