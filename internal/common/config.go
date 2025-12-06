package common

import (
	"fmt"
	"os"
	"time"

	"github.com/anyhost/gotunnel/internal/protocol"
	"gopkg.in/yaml.v3"
)

// ServerConfig holds configuration for the tunnel server.
type ServerConfig struct {
	// ControlAddr is the address for client connections (e.g., ":9000").
	ControlAddr string `yaml:"control_addr"`

	// HTTPAddr is the address for public HTTP traffic (e.g., ":80").
	HTTPAddr string `yaml:"http_addr"`

	// HTTPSAddr is the address for public HTTPS traffic (e.g., ":443").
	HTTPSAddr string `yaml:"https_addr"`

	// Domain is the base domain for subdomain routing (e.g., "example.com").
	Domain string `yaml:"domain"`

	// TLS configuration for HTTPS.
	TLS TLSConfig `yaml:"tls"`

	// Auth configuration for client authentication.
	Auth AuthConfig `yaml:"auth"`

	// Limits configuration for rate limiting and resource constraints.
	Limits LimitsConfig `yaml:"limits"`

	// Timeouts configuration for various operations.
	Timeouts TimeoutsConfig `yaml:"timeouts"`

	// ReservedSubdomains is a list of subdomains that cannot be claimed.
	ReservedSubdomains []string `yaml:"reserved_subdomains"`

	// LogLevel sets the logging verbosity (debug, info, warn, error).
	LogLevel string `yaml:"log_level"`
}

// TLSConfig holds TLS-related configuration.
type TLSConfig struct {
	// Enabled indicates whether TLS is enabled.
	Enabled bool `yaml:"enabled"`

	// CertFile is the path to the TLS certificate file.
	CertFile string `yaml:"cert_file"`

	// KeyFile is the path to the TLS private key file.
	KeyFile string `yaml:"key_file"`

	// AutoCert enables automatic certificate management via Let's Encrypt.
	AutoCert bool `yaml:"auto_cert"`

	// AutoCertDir is the directory for storing auto-generated certificates.
	AutoCertDir string `yaml:"auto_cert_dir"`
}

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	// Mode is the authentication mode: "token", "jwt", or "none".
	Mode string `yaml:"mode"`

	// TokenFile is the path to a file containing valid tokens (one per line).
	TokenFile string `yaml:"token_file"`

	// JWTSecret is the secret for validating JWT tokens.
	JWTSecret string `yaml:"jwt_secret"`
}

// LimitsConfig holds rate limiting and resource constraint configuration.
type LimitsConfig struct {
	// MaxConnectionsPerUser is the maximum concurrent connections per user.
	MaxConnectionsPerUser int `yaml:"max_connections_per_user"`

	// MaxTunnelsPerConnection is the maximum tunnels per connection.
	MaxTunnelsPerConnection int `yaml:"max_tunnels_per_connection"`

	// MaxRequestsPerMinute is the rate limit for requests per subdomain.
	MaxRequestsPerMinute int `yaml:"max_requests_per_minute"`

	// MaxRequestBodySize is the maximum request body size in bytes.
	MaxRequestBodySize int64 `yaml:"max_request_body_size"`

	// MaxBandwidthBytesPerSec is the bandwidth limit per tunnel in bytes/sec.
	MaxBandwidthBytesPerSec int64 `yaml:"max_bandwidth_bytes_per_sec"`
}

// TimeoutsConfig holds timeout configuration.
type TimeoutsConfig struct {
	// HandshakeTimeout is the timeout for completing the handshake.
	HandshakeTimeout time.Duration `yaml:"handshake_timeout"`

	// IdleTimeout is the timeout for idle connections.
	IdleTimeout time.Duration `yaml:"idle_timeout"`

	// RequestTimeout is the timeout for proxied requests.
	RequestTimeout time.Duration `yaml:"request_timeout"`

	// DialTimeout is the timeout for dialing local services.
	DialTimeout time.Duration `yaml:"dial_timeout"`

	// WriteTimeout is the timeout for write operations.
	WriteTimeout time.Duration `yaml:"write_timeout"`

	// ReadTimeout is the timeout for read operations.
	ReadTimeout time.Duration `yaml:"read_timeout"`
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		ControlAddr: ":9000",
		HTTPAddr:    ":8080",
		HTTPSAddr:   ":8443",
		Domain:      "localhost",
		TLS: TLSConfig{
			Enabled:     false,
			AutoCertDir: "./certs",
		},
		Auth: AuthConfig{
			Mode: "token",
		},
		Limits: LimitsConfig{
			MaxConnectionsPerUser:   5,
			MaxTunnelsPerConnection: 10,
			MaxRequestsPerMinute:    1000,
			MaxRequestBodySize:      50 * 1024 * 1024, // 50MB
			MaxBandwidthBytesPerSec: 0,                // unlimited
		},
		Timeouts: TimeoutsConfig{
			HandshakeTimeout: 10 * time.Second,
			IdleTimeout:      5 * time.Minute,
			RequestTimeout:   30 * time.Second,
			DialTimeout:      5 * time.Second,
			WriteTimeout:     10 * time.Second,
			ReadTimeout:      10 * time.Second,
		},
		ReservedSubdomains: []string{
			"www", "api", "admin", "mail", "smtp", "pop", "imap",
			"ftp", "ssh", "dns", "ns", "mx", "app", "static",
			"cdn", "assets", "img", "images", "css", "js",
		},
		LogLevel: "info",
	}
}

// LoadServerConfig loads server configuration from a YAML file.
func LoadServerConfig(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := DefaultServerConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

// Validate checks if the server configuration is valid.
func (c *ServerConfig) Validate() error {
	if c.ControlAddr == "" {
		return fmt.Errorf("control_addr is required")
	}
	if c.HTTPAddr == "" && c.HTTPSAddr == "" {
		return fmt.Errorf("at least one of http_addr or https_addr is required")
	}
	if c.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	if c.TLS.Enabled && !c.TLS.AutoCert {
		if c.TLS.CertFile == "" || c.TLS.KeyFile == "" {
			return fmt.Errorf("tls.cert_file and tls.key_file are required when TLS is enabled")
		}
	}
	return nil
}

// ClientConfig holds configuration for the tunnel client.
type ClientConfig struct {
	// ServerAddr is the address of the tunnel server (e.g., "tunnel.example.com:9000").
	ServerAddr string `yaml:"server_addr"`

	// Token is the authentication token.
	Token string `yaml:"token"`

	// ClientID is an optional identifier for this client.
	ClientID string `yaml:"client_id"`

	// Tunnels is the list of tunnels to establish.
	Tunnels []protocol.TunnelConfig `yaml:"tunnels"`

	// Reconnect configuration for automatic reconnection.
	Reconnect ReconnectConfig `yaml:"reconnect"`

	// LocalServer configuration for the local inspection dashboard.
	LocalServer LocalServerConfig `yaml:"local_server"`

	// LogLevel sets the logging verbosity (debug, info, warn, error).
	LogLevel string `yaml:"log_level"`
}

// ReconnectConfig holds reconnection settings.
type ReconnectConfig struct {
	// Enabled indicates whether automatic reconnection is enabled.
	Enabled bool `yaml:"enabled"`

	// InitialDelay is the initial delay before the first reconnection attempt.
	InitialDelay time.Duration `yaml:"initial_delay"`

	// MaxDelay is the maximum delay between reconnection attempts.
	MaxDelay time.Duration `yaml:"max_delay"`

	// Multiplier is the factor by which the delay increases after each attempt.
	Multiplier float64 `yaml:"multiplier"`

	// MaxAttempts is the maximum number of reconnection attempts (0 = unlimited).
	MaxAttempts int `yaml:"max_attempts"`
}

// LocalServerConfig holds configuration for the local inspection server.
type LocalServerConfig struct {
	// Enabled indicates whether the local server is enabled.
	Enabled bool `yaml:"enabled"`

	// Addr is the address for the local server (e.g., ":4040").
	Addr string `yaml:"addr"`
}

// DefaultClientConfig returns a ClientConfig with sensible defaults.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		ServerAddr: "localhost:9000",
		Reconnect: ReconnectConfig{
			Enabled:      true,
			InitialDelay: 1 * time.Second,
			MaxDelay:     30 * time.Second,
			Multiplier:   2.0,
			MaxAttempts:  0, // unlimited
		},
		LocalServer: LocalServerConfig{
			Enabled: false,
			Addr:    ":4040",
		},
		LogLevel: "info",
	}
}

// LoadClientConfig loads client configuration from a YAML file.
func LoadClientConfig(path string) (*ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := DefaultClientConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

// Validate checks if the client configuration is valid.
func (c *ClientConfig) Validate() error {
	if c.ServerAddr == "" {
		return fmt.Errorf("server_addr is required")
	}
	if c.Token == "" {
		return fmt.Errorf("token is required")
	}
	if len(c.Tunnels) == 0 {
		return fmt.Errorf("at least one tunnel is required")
	}
	for i, tunnel := range c.Tunnels {
		if err := tunnel.Validate(); err != nil {
			return fmt.Errorf("tunnel[%d]: %w", i, err)
		}
	}
	return nil
}
