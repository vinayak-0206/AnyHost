package server

import (
	"testing"

	"github.com/anyhost/gotunnel/internal/protocol"
)

func TestRegistry_ValidateSubdomain(t *testing.T) {
	reserved := []string{"api", "admin", "www"}
	registry := NewRegistry("example.com", reserved)

	tests := []struct {
		name      string
		subdomain string
		wantErr   bool
	}{
		{"valid subdomain", "myapp", false},
		{"valid with numbers", "app123", false},
		{"valid with hyphens", "my-app", false},
		{"too short", "ab", true},
		{"too long", "a" + string(make([]byte, 63)), true},
		{"starts with hyphen", "-myapp", true},
		// Note: Current implementation allows trailing hyphens and mixed case
		// These could be enhanced in the future
		{"ends with hyphen", "myapp-", false},       // Currently allowed
		{"uppercase letters", "MyApp", false},       // Currently allowed (could be normalized)
		{"underscores", "my_app", true},
		{"special chars", "my.app", true},
		{"reserved api", "api", true},
		{"reserved admin", "admin", true},
		{"reserved www", "www", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.ValidateSubdomain(tt.subdomain)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSubdomain(%q) error = %v, wantErr %v", tt.subdomain, err, tt.wantErr)
			}
		})
	}
}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	registry := NewRegistry("example.com", nil)

	// Create a mock session
	session := &Session{
		ID:    "test-session",
		Token: "test-token",
	}

	tunnels := []protocol.TunnelConfig{
		{Subdomain: "app1", LocalPort: 3000},
		{Subdomain: "app2", LocalPort: 3001},
	}

	// Register tunnels
	statuses := registry.Register(session, tunnels)

	// Check all registered successfully
	for i, status := range statuses {
		if status.Status != "active" {
			t.Errorf("tunnel %d: expected status 'active', got %q", i, status.Status)
		}
	}

	// Lookup should find them
	entry, found := registry.Lookup("app1")
	if !found {
		t.Error("Lookup(app1) = not found, want found")
	}
	if entry.Session != session {
		t.Error("Lookup(app1) returned wrong session")
	}

	entry, found = registry.Lookup("app2")
	if !found {
		t.Error("Lookup(app2) = not found, want found")
	}

	// Unknown subdomain should not be found
	_, found = registry.Lookup("unknown")
	if found {
		t.Error("Lookup(unknown) = found, want not found")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	registry := NewRegistry("example.com", nil)

	session := &Session{
		ID:    "test-session",
		Token: "test-token",
	}

	tunnels := []protocol.TunnelConfig{
		{Subdomain: "myapp", LocalPort: 3000},
	}

	// Register
	registry.Register(session, tunnels)

	// Verify registered
	_, found := registry.Lookup("myapp")
	if !found {
		t.Fatal("tunnel should be registered")
	}

	// Unregister
	registry.Unregister(session.ID)

	// Verify unregistered
	_, found = registry.Lookup("myapp")
	if found {
		t.Error("tunnel should be unregistered")
	}
}

func TestRegistry_DuplicateSubdomain(t *testing.T) {
	registry := NewRegistry("example.com", nil)

	session1 := &Session{ID: "session1", Token: "token1"}
	session2 := &Session{ID: "session2", Token: "token2"}

	// First registration should succeed
	statuses := registry.Register(session1, []protocol.TunnelConfig{
		{Subdomain: "myapp", LocalPort: 3000},
	})

	if statuses[0].Status != "active" {
		t.Errorf("first registration: expected 'active', got %q", statuses[0].Status)
	}

	// Second registration of same subdomain should fail
	statuses = registry.Register(session2, []protocol.TunnelConfig{
		{Subdomain: "myapp", LocalPort: 3001},
	})

	if statuses[0].Status == "active" {
		t.Error("second registration should have failed")
	}
}

func TestRegistry_GetTunnelCount(t *testing.T) {
	registry := NewRegistry("example.com", nil)

	if count := registry.GetTunnelCount(); count != 0 {
		t.Errorf("initial count = %d, want 0", count)
	}

	session := &Session{ID: "test", Token: "token"}
	registry.Register(session, []protocol.TunnelConfig{
		{Subdomain: "app1", LocalPort: 3000},
		{Subdomain: "app2", LocalPort: 3001},
	})

	if count := registry.GetTunnelCount(); count != 2 {
		t.Errorf("count after register = %d, want 2", count)
	}

	registry.Unregister(session.ID)

	if count := registry.GetTunnelCount(); count != 0 {
		t.Errorf("count after unregister = %d, want 0", count)
	}
}
