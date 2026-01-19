package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/anyhost/gotunnel/internal/database"
)

type API struct {
	db       *database.DB
	registry *Registry
	control  *ControlPlane
}

func NewAPI(db *database.DB, reg *Registry, cp *ControlPlane) *API {
	return &API{db: db, registry: reg, control: cp}
}

// Helper for JSON responses
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// --- Auth Handlers ---

func (a *API) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400); return
	}

	user, err := a.db.CreateUser(req.Email, req.Password)
	if err != nil {
		http.Error(w, "Registration failed: "+err.Error(), 500); return
	}
	
	jsonResponse(w, 201, user)
}

func (a *API) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), 400); return
	}

	user, err := a.db.AuthenticateUser(req.Email, req.Password)
	if err != nil {
		http.Error(w, "Invalid credentials", 401); return
	}

	// In a real app, generate a JWT here. For simplicity, we return the user ID.
	jsonResponse(w, 200, map[string]string{"token": user.ID, "user_id": user.ID})
}

// --- Tunnel Handlers ---

func (a *API) HandleReserve(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID") // Populated by Middleware
	var req struct { Subdomain string `json:"subdomain"` }
	json.NewDecoder(r.Body).Decode(&req)

	if err := a.db.ReserveSubdomain(userID, req.Subdomain); err != nil {
		http.Error(w, "Could not reserve: "+err.Error(), 409); return
	}
	jsonResponse(w, 200, map[string]string{"status": "reserved"})
}

func (a *API) HandleListTunnels(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")

	// 1. Get reserved subdomains
	reserved, _ := a.db.GetUserSubdomains(userID)

	// 2. Check active status in Registry
	type TunnelStatus struct {
		Subdomain string `json:"subdomain"`
		Status    string `json:"status"` // "online" or "offline"
		URL       string `json:"url"`
	}

	var list []TunnelStatus
	for _, sub := range reserved {
		status := "offline"
		if _, found := a.registry.Lookup(sub); found {
			status = "online"
		}
		list = append(list, TunnelStatus{
			Subdomain: sub,
			Status:    status,
			URL:       "https://" + sub + ".yourdomain.com",
		})
	}

	jsonResponse(w, 200, list)
}

// --- Request Inspector Handlers ---

// HandleGetRequestLogs returns request logs for a subdomain
func (a *API) HandleGetRequestLogs(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")

	// Extract subdomain from path: /api/requests/{subdomain}
	subdomain := strings.TrimPrefix(r.URL.Path, "/api/requests/")
	if subdomain == "" || strings.Contains(subdomain, "/") {
		// Check if it's a request for a specific log
		parts := strings.Split(subdomain, "/")
		if len(parts) == 2 {
			a.handleGetSingleRequest(w, r, userID, parts[0], parts[1])
			return
		}
		http.Error(w, "Subdomain required", http.StatusBadRequest)
		return
	}

	// Verify user owns this subdomain
	owner, err := a.db.GetSubdomainOwner(subdomain)
	if err != nil || owner != userID {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	// Parse pagination params
	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := parseInt(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := parseInt(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	logs, err := a.db.GetRequestLogs(subdomain, limit, offset)
	if err != nil {
		http.Error(w, "Failed to fetch logs", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]interface{}{
		"subdomain": subdomain,
		"logs":      logs,
		"limit":     limit,
		"offset":    offset,
	})
}

func (a *API) handleGetSingleRequest(w http.ResponseWriter, r *http.Request, userID, subdomain, requestID string) {
	// Verify user owns this subdomain
	owner, err := a.db.GetSubdomainOwner(subdomain)
	if err != nil || owner != userID {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	log, err := a.db.GetRequestLog(requestID)
	if err != nil {
		http.Error(w, "Request not found", http.StatusNotFound)
		return
	}

	// Verify the log belongs to this subdomain
	if log.Subdomain != subdomain {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	jsonResponse(w, http.StatusOK, log)
}

// --- Organization Handlers ---

// HandleCreateOrganization creates a new organization
func (a *API) HandleCreateOrganization(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")

	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Slug == "" {
		http.Error(w, "Name and slug are required", http.StatusBadRequest)
		return
	}

	org, err := a.db.CreateOrganization(req.Name, req.Slug, userID)
	if err != nil {
		http.Error(w, "Failed to create organization: "+err.Error(), http.StatusConflict)
		return
	}

	jsonResponse(w, http.StatusCreated, org)
}

// HandleListOrganizations lists user's organizations
func (a *API) HandleListOrganizations(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")

	orgs, err := a.db.GetUserOrganizations(userID)
	if err != nil {
		http.Error(w, "Failed to fetch organizations", http.StatusInternalServerError)
		return
	}

	if orgs == nil {
		orgs = []database.Organization{}
	}

	jsonResponse(w, http.StatusOK, orgs)
}

// HandleGetOrganization gets organization details
func (a *API) HandleGetOrganization(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")

	// Extract org ID from path: /api/orgs/{id}
	orgID := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	if orgID == "" {
		http.Error(w, "Organization ID required", http.StatusBadRequest)
		return
	}

	// Check membership
	if !a.db.IsOrganizationMember(orgID, userID) {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	org, err := a.db.GetOrganization(orgID)
	if err != nil {
		http.Error(w, "Organization not found", http.StatusNotFound)
		return
	}

	jsonResponse(w, http.StatusOK, org)
}

// HandleGetOrganizationMembers lists organization members
func (a *API) HandleGetOrganizationMembers(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")

	// Extract from path: /api/orgs/{id}/members
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "members" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	orgID := parts[0]

	// Check membership
	if !a.db.IsOrganizationMember(orgID, userID) {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	members, err := a.db.GetOrganizationMembers(orgID)
	if err != nil {
		http.Error(w, "Failed to fetch members", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, http.StatusOK, members)
}

// HandleAddOrganizationMember adds a member to an organization
func (a *API) HandleAddOrganizationMember(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")

	// Extract org ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/orgs/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "members" {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	orgID := parts[0]

	// Check if user is admin/owner
	role, _ := a.db.GetUserRoleInOrganization(orgID, userID)
	if role != "owner" && role != "admin" {
		http.Error(w, "Only admins can add members", http.StatusForbidden)
		return
	}

	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Role == "" {
		req.Role = "member"
	}

	if err := a.db.AddOrganizationMember(orgID, req.UserID, req.Role); err != nil {
		http.Error(w, "Failed to add member: "+err.Error(), http.StatusConflict)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]string{"status": "added"})
}

// --- Helper Functions ---

func parseInt(s string) (int, error) {
	var n int
	err := json.Unmarshal([]byte(s), &n)
	return n, err
}

// --- Middleware ---

func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			http.Error(w, "Unauthorized", 401)
			return
		}
		// Simplified: We are using the UserID directly as the token for this MVP
		r.Header.Set("X-User-ID", strings.TrimPrefix(token, "Bearer "))
		next(w, r)
	}
}