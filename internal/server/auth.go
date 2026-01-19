package server

import (
	"bufio"
	"crypto/subtle"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/anyhost/gotunnel/internal/common"
	"github.com/anyhost/gotunnel/internal/protocol"
)

// Authenticator validates client tokens.
type Authenticator interface {
	// Validate checks if a token is valid.
	Validate(token string) (bool, error)

	// GetUserID returns the user ID associated with a token.
	GetUserID(token string) (string, error)
}

// NoOpAuthenticator accepts all tokens (for development/testing).
type NoOpAuthenticator struct{}

// Validate always returns true.
func (a *NoOpAuthenticator) Validate(token string) (bool, error) {
	return true, nil
}

// GetUserID returns the token as the user ID.
func (a *NoOpAuthenticator) GetUserID(token string) (string, error) {
	return token, nil
}

// TokenAuthenticator validates tokens against a static list.
type TokenAuthenticator struct {
	mu     sync.RWMutex
	tokens map[string]string // token -> userID
}

// NewTokenAuthenticator creates a new token authenticator.
func NewTokenAuthenticator() *TokenAuthenticator {
	return &TokenAuthenticator{
		tokens: make(map[string]string),
	}
}

// AddToken adds a token to the authenticator.
func (a *TokenAuthenticator) AddToken(token, userID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokens[token] = userID
}

// RemoveToken removes a token from the authenticator.
func (a *TokenAuthenticator) RemoveToken(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.tokens, token)
}

// LoadFromFile loads tokens from a file (one token per line, format: token:userID or just token).
func (a *TokenAuthenticator) LoadFromFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open token file: %w", err)
	}
	defer file.Close()

	a.mu.Lock()
	defer a.mu.Unlock()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse format: token:userID or just token
		parts := strings.SplitN(line, ":", 2)
		token := strings.TrimSpace(parts[0])
		userID := token // Default userID to token if not specified

		if len(parts) == 2 {
			userID = strings.TrimSpace(parts[1])
		}

		if token == "" {
			return fmt.Errorf("invalid token on line %d", lineNum)
		}

		a.tokens[token] = userID
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read token file: %w", err)
	}

	return nil
}

// Validate checks if a token is valid using constant-time comparison.
func (a *TokenAuthenticator) Validate(token string) (bool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Use constant-time comparison to prevent timing attacks
	for storedToken := range a.tokens {
		if subtle.ConstantTimeCompare([]byte(storedToken), []byte(token)) == 1 {
			return true, nil
		}
	}

	return false, nil
}

// GetUserID returns the user ID associated with a token.
func (a *TokenAuthenticator) GetUserID(token string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Find the matching token using constant-time comparison
	for storedToken, userID := range a.tokens {
		if subtle.ConstantTimeCompare([]byte(storedToken), []byte(token)) == 1 {
			return userID, nil
		}
	}

	return "", protocol.ErrUnauthorized
}

// TokenCount returns the number of loaded tokens.
func (a *TokenAuthenticator) TokenCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.tokens)
}

// DatabaseAuthenticator validates tokens against the database (user IDs).
type DatabaseAuthenticator struct {
	db       interface{ UserExists(userID string) bool }
	fallback Authenticator
}

// NewDatabaseAuthenticator creates a new database authenticator with optional fallback.
func NewDatabaseAuthenticator(db interface{ UserExists(userID string) bool }, fallback Authenticator) *DatabaseAuthenticator {
	return &DatabaseAuthenticator{
		db:       db,
		fallback: fallback,
	}
}

// Validate checks if a token (user ID) exists in the database.
func (a *DatabaseAuthenticator) Validate(token string) (bool, error) {
	// First check if it's a valid user ID in the database
	if a.db != nil {
		exists := a.db.UserExists(token)
		if exists {
			return true, nil
		}
	}

	// Fall back to other authenticator (e.g., dev-token)
	if a.fallback != nil {
		return a.fallback.Validate(token)
	}

	return false, nil
}

// GetUserID returns the token as the user ID (since token IS the user ID).
func (a *DatabaseAuthenticator) GetUserID(token string) (string, error) {
	valid, err := a.Validate(token)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", protocol.ErrUnauthorized
	}
	return token, nil
}

// NewAuthenticatorFromConfig creates an authenticator based on the auth configuration.
func NewAuthenticatorFromConfig(cfg *common.AuthConfig) (Authenticator, error) {
	switch cfg.Mode {
	case "none":
		return &NoOpAuthenticator{}, nil

	case "token":
		auth := NewTokenAuthenticator()
		if cfg.TokenFile != "" {
			if err := auth.LoadFromFile(cfg.TokenFile); err != nil {
				return nil, fmt.Errorf("failed to load tokens: %w", err)
			}
		}
		return auth, nil

	case "jwt":
		// JWT authentication would be implemented here
		return nil, fmt.Errorf("JWT authentication not yet implemented")

	default:
		return nil, fmt.Errorf("unknown auth mode: %s", cfg.Mode)
	}
}
