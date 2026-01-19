package server

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/anyhost/gotunnel/internal/protocol"
)

// subdomainRegex validates subdomain format.
// Must start with a letter, contain only lowercase alphanumeric and hyphens,
// and be between 3 and 63 characters.
var subdomainRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{2,62}$`)

// TunnelEntry represents a registered tunnel in the registry.
type TunnelEntry struct {
	Subdomain string
	LocalPort int
	LocalHost string
	Protocol  string
	Session   *Session
}

// SubdomainOwnerChecker checks subdomain ownership in the database.
type SubdomainOwnerChecker interface {
	GetSubdomainOwner(subdomain string) (string, error)
}

// Registry manages the mapping of subdomains to active client sessions.
// It is safe for concurrent access.
type Registry struct {
	mu sync.RWMutex

	// tunnels maps subdomain -> TunnelEntry
	tunnels map[string]*TunnelEntry

	// sessions maps sessionID -> Session
	sessions map[string]*Session

	// reservedSubdomains is a set of subdomains that cannot be claimed.
	reservedSubdomains map[string]struct{}

	// domain is the base domain (e.g., "example.com").
	domain string

	// ownerChecker is used to verify subdomain ownership from database.
	ownerChecker SubdomainOwnerChecker
}

// NewRegistry creates a new registry with the given base domain and reserved subdomains.
func NewRegistry(domain string, reservedSubdomains []string) *Registry {
	reserved := make(map[string]struct{}, len(reservedSubdomains))
	for _, s := range reservedSubdomains {
		reserved[strings.ToLower(s)] = struct{}{}
	}

	return &Registry{
		tunnels:            make(map[string]*TunnelEntry),
		sessions:           make(map[string]*Session),
		reservedSubdomains: reserved,
		domain:             domain,
	}
}

// SetOwnerChecker sets the subdomain owner checker for database validation.
func (r *Registry) SetOwnerChecker(checker SubdomainOwnerChecker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ownerChecker = checker
}

// ValidateSubdomain checks if a subdomain is valid for registration.
func (r *Registry) ValidateSubdomain(subdomain string) error {
	subdomain = strings.ToLower(subdomain)

	if !subdomainRegex.MatchString(subdomain) {
		return fmt.Errorf("%w: must be 3-63 lowercase alphanumeric characters starting with a letter", protocol.ErrSubdomainInvalid)
	}

	if _, reserved := r.reservedSubdomains[subdomain]; reserved {
		return fmt.Errorf("%w: '%s' is reserved", protocol.ErrSubdomainReserved, subdomain)
	}

	return nil
}

// Register registers tunnels for a session.
// Returns a list of TunnelStatus for each requested tunnel.
func (r *Registry) Register(session *Session, tunnels []protocol.TunnelConfig) []protocol.TunnelStatus {
	r.mu.Lock()
	defer r.mu.Unlock()

	results := make([]protocol.TunnelStatus, 0, len(tunnels))

	for _, tc := range tunnels {
		subdomain := strings.ToLower(tc.Subdomain)
		status := protocol.TunnelStatus{
			Subdomain: subdomain,
			LocalPort: tc.LocalPort,
		}

		// Validate subdomain format
		if err := r.ValidateSubdomain(subdomain); err != nil {
			status.Status = "error"
			status.Error = err.Error()
			results = append(results, status)
			continue
		}

		// Check database ownership if owner checker is set
		if r.ownerChecker != nil {
			owner, err := r.ownerChecker.GetSubdomainOwner(subdomain)
			if err == nil && owner != "" {
				// Subdomain is reserved in database - check ownership
				if owner != session.Token {
					status.Status = "error"
					status.Error = "subdomain is reserved by another user"
					results = append(results, status)
					continue
				}
			}
			// If no owner or error, allow first-come-first-served
		}

		// Check if subdomain is already taken by another session
		if existing, exists := r.tunnels[subdomain]; exists {
			if existing.Session.ID != session.ID {
				status.Status = "error"
				status.Error = protocol.ErrSubdomainTaken.Error()
				results = append(results, status)
				continue
			}
		}

		// Register the tunnel
		entry := &TunnelEntry{
			Subdomain: subdomain,
			LocalPort: tc.LocalPort,
			LocalHost: tc.LocalHost,
			Protocol:  tc.Protocol,
			Session:   session,
		}
		r.tunnels[subdomain] = entry

		status.Status = "active"
		status.URL = r.buildURL(subdomain, tc.Protocol)
		results = append(results, status)
	}

	// Register session
	r.sessions[session.ID] = session

	return results
}

// Unregister removes all tunnels for a session.
func (r *Registry) Unregister(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove all tunnels belonging to this session
	for subdomain, entry := range r.tunnels {
		if entry.Session.ID == sessionID {
			delete(r.tunnels, subdomain)
		}
	}

	// Remove session
	delete(r.sessions, sessionID)
}

// UnregisterTunnel removes a specific tunnel for a session.
func (r *Registry) UnregisterTunnel(sessionID, subdomain string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	subdomain = strings.ToLower(subdomain)
	entry, exists := r.tunnels[subdomain]
	if !exists {
		return protocol.ErrTunnelNotFound
	}

	if entry.Session.ID != sessionID {
		return protocol.ErrUnauthorized
	}

	delete(r.tunnels, subdomain)
	return nil
}

// Lookup finds the tunnel entry for a given subdomain.
func (r *Registry) Lookup(subdomain string) (*TunnelEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	subdomain = strings.ToLower(subdomain)
	entry, exists := r.tunnels[subdomain]
	return entry, exists
}

// LookupByHost extracts the subdomain from a host header and looks up the tunnel.
func (r *Registry) LookupByHost(host string) (*TunnelEntry, bool) {
	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// Extract subdomain from host
	host = strings.ToLower(host)
	suffix := "." + r.domain
	if !strings.HasSuffix(host, suffix) {
		return nil, false
	}

	subdomain := strings.TrimSuffix(host, suffix)
	return r.Lookup(subdomain)
}

// GetSession returns a session by ID.
func (r *Registry) GetSession(sessionID string) (*Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, exists := r.sessions[sessionID]
	return session, exists
}

// GetAllSessions returns all active sessions.
func (r *Registry) GetAllSessions() []*Session {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sessions := make([]*Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// GetTunnelCount returns the number of active tunnels.
func (r *Registry) GetTunnelCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tunnels)
}

// GetSessionCount returns the number of active sessions.
func (r *Registry) GetSessionCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sessions)
}

// buildURL constructs the public URL for a tunnel.
func (r *Registry) buildURL(subdomain, tunnelProtocol string) string {
	scheme := "http"
	// Note: In production, this would check TLS configuration
	return fmt.Sprintf("%s://%s.%s", scheme, subdomain, r.domain)
}

// GetTunnelsForSession returns all tunnels for a given session.
func (r *Registry) GetTunnelsForSession(sessionID string) []*TunnelEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tunnels []*TunnelEntry
	for _, entry := range r.tunnels {
		if entry.Session.ID == sessionID {
			tunnels = append(tunnels, entry)
		}
	}
	return tunnels
}
