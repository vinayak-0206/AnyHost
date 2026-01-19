package common

import (
	"os"
	"testing"

	"github.com/anyhost/gotunnel/internal/protocol"
)

func TestServerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ServerConfig
		wantErr bool
	}{
		{
			name:    "default config is valid",
			config:  *DefaultServerConfig(),
			wantErr: false,
		},
		{
			name: "missing control addr",
			config: ServerConfig{
				ControlAddr: "",
				HTTPAddr:    ":8080",
				Domain:      "example.com",
			},
			wantErr: true,
		},
		{
			name: "missing domain",
			config: ServerConfig{
				ControlAddr: ":9000",
				HTTPAddr:    ":8080",
				Domain:      "",
			},
			wantErr: true,
		},
		{
			name: "missing both http addrs",
			config: ServerConfig{
				ControlAddr: ":9000",
				HTTPAddr:    "",
				HTTPSAddr:   "",
				Domain:      "example.com",
			},
			wantErr: true,
		},
		{
			name: "TLS enabled without cert",
			config: ServerConfig{
				ControlAddr: ":9000",
				HTTPAddr:    ":8080",
				Domain:      "example.com",
				TLS: TLSConfig{
					Enabled:  true,
					AutoCert: false,
					CertFile: "",
					KeyFile:  "",
				},
			},
			wantErr: true,
		},
		{
			name: "TLS with auto cert is valid",
			config: ServerConfig{
				ControlAddr: ":9000",
				HTTPAddr:    ":8080",
				Domain:      "example.com",
				TLS: TLSConfig{
					Enabled:  true,
					AutoCert: true,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServerConfig_GetAllowedOrigins(t *testing.T) {
	tests := []struct {
		name           string
		config         ServerConfig
		expectedCount  int
		expectContains string
	}{
		{
			name: "custom origins",
			config: ServerConfig{
				Domain: "example.com",
				CORS: CORSConfig{
					AllowedOrigins: []string{"https://custom.com"},
				},
			},
			expectedCount:  1,
			expectContains: "https://custom.com",
		},
		{
			name: "default origins from domain",
			config: ServerConfig{
				Domain: "tunnel.example.com",
				CORS:   CORSConfig{},
			},
			expectedCount:  6, // http + https for domain + localhost variants
			expectContains: "https://tunnel.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origins := tt.config.GetAllowedOrigins()
			if len(origins) != tt.expectedCount {
				t.Errorf("GetAllowedOrigins() count = %d, want %d", len(origins), tt.expectedCount)
			}

			found := false
			for _, o := range origins {
				if o == tt.expectContains {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("GetAllowedOrigins() should contain %q", tt.expectContains)
			}
		})
	}
}

func TestServerConfig_IsOriginAllowed(t *testing.T) {
	config := ServerConfig{
		Domain: "example.com",
		CORS: CORSConfig{
			AllowedOrigins: []string{"https://allowed.com", "*"},
		},
	}

	tests := []struct {
		origin  string
		allowed bool
	}{
		{"https://allowed.com", true},
		{"https://other.com", true}, // wildcard allows all
	}

	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			if got := config.IsOriginAllowed(tt.origin); got != tt.allowed {
				t.Errorf("IsOriginAllowed(%q) = %v, want %v", tt.origin, got, tt.allowed)
			}
		})
	}
}

func TestClientConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ClientConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: ClientConfig{
				ServerAddr: "localhost:9000",
				Token:      "test-token",
				Tunnels: []protocol.TunnelConfig{
					{Subdomain: "myapp", LocalPort: 3000},
				},
			},
			wantErr: false,
		},
		{
			name: "missing server addr",
			config: ClientConfig{
				Token: "test-token",
				Tunnels: []protocol.TunnelConfig{
					{Subdomain: "myapp", LocalPort: 3000},
				},
			},
			wantErr: true,
		},
		{
			name: "missing token",
			config: ClientConfig{
				ServerAddr: "localhost:9000",
				Tunnels: []protocol.TunnelConfig{
					{Subdomain: "myapp", LocalPort: 3000},
				},
			},
			wantErr: true,
		},
		{
			name: "no tunnels",
			config: ClientConfig{
				ServerAddr: "localhost:9000",
				Token:      "test-token",
				Tunnels:    []protocol.TunnelConfig{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadServerConfig(t *testing.T) {
	// Create a temporary config file
	content := `
domain: "test.example.com"
control_addr: ":9000"
http_addr: ":8080"
log_level: "debug"
`
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	// Load and verify
	config, err := LoadServerConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("LoadServerConfig failed: %v", err)
	}

	if config.Domain != "test.example.com" {
		t.Errorf("Domain = %q, want %q", config.Domain, "test.example.com")
	}
	if config.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", config.LogLevel, "debug")
	}
}
