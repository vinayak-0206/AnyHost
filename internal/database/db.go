package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type DB struct {
	*sql.DB
}

func New(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	
	if err := db.Ping(); err != nil {
		return nil, err
	}

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return &DB{db}, nil
}

func migrate(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			is_admin BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS api_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			name TEXT,
			last_used_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id)
		);`,
		// Organizations/Teams table
		`CREATE TABLE IF NOT EXISTS organizations (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			slug TEXT UNIQUE NOT NULL,
			owner_id TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(owner_id) REFERENCES users(id)
		);`,
		// Organization membership (many-to-many)
		`CREATE TABLE IF NOT EXISTS organization_members (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'member',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(organization_id, user_id),
			FOREIGN KEY(organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		// Subdomains can belong to user OR organization
		`CREATE TABLE IF NOT EXISTS subdomains (
			id TEXT PRIMARY KEY,
			user_id TEXT,
			organization_id TEXT,
			subdomain TEXT UNIQUE NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id),
			FOREIGN KEY(organization_id) REFERENCES organizations(id)
		);`,
		`CREATE TABLE IF NOT EXISTS request_logs (
			id TEXT PRIMARY KEY,
			subdomain TEXT NOT NULL,
			method TEXT,
			path TEXT,
			status_code INTEGER,
			duration_ms INTEGER,
			client_ip TEXT,
			user_agent TEXT,
			request_headers TEXT,
			response_headers TEXT,
			request_body TEXT,
			response_body TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		// Create indexes for better query performance
		`CREATE INDEX IF NOT EXISTS idx_request_logs_subdomain ON request_logs(subdomain);`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_subdomains_user_id ON subdomains(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_subdomains_organization_id ON subdomains(organization_id);`,
		`CREATE INDEX IF NOT EXISTS idx_org_members_org_id ON organization_members(organization_id);`,
		`CREATE INDEX IF NOT EXISTS idx_org_members_user_id ON organization_members(user_id);`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// --- User Methods ---

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

func (db *DB) CreateUser(email, password string) (*User, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	id := uuid.New().String()
	_, err = db.Exec("INSERT INTO users (id, email, password_hash) VALUES (?, ?, ?)", 
		id, email, string(hashed))
	if err != nil {
		return nil, err
	}

	return &User{ID: id, Email: email, CreatedAt: time.Now()}, nil
}

func (db *DB) AuthenticateUser(email, password string) (*User, error) {
	var user User
	var hash string
	err := db.QueryRow("SELECT id, email, password_hash, is_admin, created_at FROM users WHERE email = ?", email).Scan(
		&user.ID, &user.Email, &hash, &user.IsAdmin, &user.CreatedAt)
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	return &user, nil
}

// UserExists checks if a user ID exists in the database.
func (db *DB) UserExists(userID string) bool {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", userID).Scan(&count)
	return err == nil && count > 0
}

// --- Subdomain Methods ---

func (db *DB) ReserveSubdomain(userID, subdomain string) error {
	id := uuid.New().String()
	_, err := db.Exec("INSERT INTO subdomains (id, user_id, subdomain) VALUES (?, ?, ?)", 
		id, userID, subdomain)
	return err
}

func (db *DB) GetSubdomainOwner(subdomain string) (string, error) {
	var userID string
	err := db.QueryRow("SELECT user_id FROM subdomains WHERE subdomain = ?", subdomain).Scan(&userID)
	if err == sql.ErrNoRows {
		return "", nil // Not reserved
	}
	return userID, err
}

func (db *DB) GetUserSubdomains(userID string) ([]string, error) {
	rows, err := db.Query("SELECT subdomain FROM subdomains WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		subs = append(subs, s)
	}
	return subs, nil
}

// --- Organization Methods ---

// Organization represents a team or organization.
type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	OwnerID   string    `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
}

// OrganizationMember represents a user's membership in an organization.
type OrganizationMember struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	UserID         string    `json:"user_id"`
	Role           string    `json:"role"` // "owner", "admin", "member"
	CreatedAt      time.Time `json:"created_at"`
}

// CreateOrganization creates a new organization and adds the creator as owner.
func (db *DB) CreateOrganization(name, slug, ownerID string) (*Organization, error) {
	id := uuid.New().String()
	now := time.Now()

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Create the organization
	_, err = tx.Exec(
		"INSERT INTO organizations (id, name, slug, owner_id, created_at) VALUES (?, ?, ?, ?, ?)",
		id, name, slug, ownerID, now)
	if err != nil {
		return nil, err
	}

	// Add owner as a member with "owner" role
	memberID := uuid.New().String()
	_, err = tx.Exec(
		"INSERT INTO organization_members (id, organization_id, user_id, role, created_at) VALUES (?, ?, ?, ?, ?)",
		memberID, id, ownerID, "owner", now)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &Organization{
		ID:        id,
		Name:      name,
		Slug:      slug,
		OwnerID:   ownerID,
		CreatedAt: now,
	}, nil
}

// GetOrganization retrieves an organization by ID.
func (db *DB) GetOrganization(id string) (*Organization, error) {
	var org Organization
	err := db.QueryRow(
		"SELECT id, name, slug, owner_id, created_at FROM organizations WHERE id = ?", id,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.OwnerID, &org.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// GetOrganizationBySlug retrieves an organization by slug.
func (db *DB) GetOrganizationBySlug(slug string) (*Organization, error) {
	var org Organization
	err := db.QueryRow(
		"SELECT id, name, slug, owner_id, created_at FROM organizations WHERE slug = ?", slug,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.OwnerID, &org.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// GetUserOrganizations retrieves all organizations a user is a member of.
func (db *DB) GetUserOrganizations(userID string) ([]Organization, error) {
	rows, err := db.Query(`
		SELECT o.id, o.name, o.slug, o.owner_id, o.created_at
		FROM organizations o
		JOIN organization_members om ON o.id = om.organization_id
		WHERE om.user_id = ?
		ORDER BY o.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orgs []Organization
	for rows.Next() {
		var org Organization
		if err := rows.Scan(&org.ID, &org.Name, &org.Slug, &org.OwnerID, &org.CreatedAt); err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	return orgs, nil
}

// AddOrganizationMember adds a user to an organization.
func (db *DB) AddOrganizationMember(orgID, userID, role string) error {
	id := uuid.New().String()
	_, err := db.Exec(
		"INSERT INTO organization_members (id, organization_id, user_id, role) VALUES (?, ?, ?, ?)",
		id, orgID, userID, role)
	return err
}

// RemoveOrganizationMember removes a user from an organization.
func (db *DB) RemoveOrganizationMember(orgID, userID string) error {
	_, err := db.Exec(
		"DELETE FROM organization_members WHERE organization_id = ? AND user_id = ?",
		orgID, userID)
	return err
}

// GetOrganizationMembers retrieves all members of an organization.
func (db *DB) GetOrganizationMembers(orgID string) ([]OrganizationMember, error) {
	rows, err := db.Query(`
		SELECT id, organization_id, user_id, role, created_at
		FROM organization_members
		WHERE organization_id = ?`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []OrganizationMember
	for rows.Next() {
		var m OrganizationMember
		if err := rows.Scan(&m.ID, &m.OrganizationID, &m.UserID, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, nil
}

// GetUserRoleInOrganization returns the user's role in an organization, or empty if not a member.
func (db *DB) GetUserRoleInOrganization(orgID, userID string) (string, error) {
	var role string
	err := db.QueryRow(
		"SELECT role FROM organization_members WHERE organization_id = ? AND user_id = ?",
		orgID, userID).Scan(&role)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return role, err
}

// IsOrganizationMember checks if a user is a member of an organization.
func (db *DB) IsOrganizationMember(orgID, userID string) bool {
	role, err := db.GetUserRoleInOrganization(orgID, userID)
	return err == nil && role != ""
}

// ReserveSubdomainForOrg reserves a subdomain for an organization.
func (db *DB) ReserveSubdomainForOrg(orgID, subdomain string) error {
	id := uuid.New().String()
	_, err := db.Exec(
		"INSERT INTO subdomains (id, organization_id, subdomain) VALUES (?, ?, ?)",
		id, orgID, subdomain)
	return err
}

// GetOrganizationSubdomains retrieves all subdomains belonging to an organization.
func (db *DB) GetOrganizationSubdomains(orgID string) ([]string, error) {
	rows, err := db.Query("SELECT subdomain FROM subdomains WHERE organization_id = ?", orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, nil
}

// --- Request Logging ---

// RequestLog represents a logged HTTP request.
type RequestLog struct {
	ID              string    `json:"id"`
	Subdomain       string    `json:"subdomain"`
	Method          string    `json:"method"`
	Path            string    `json:"path"`
	StatusCode      int       `json:"status_code"`
	DurationMs      int       `json:"duration_ms"`
	ClientIP        string    `json:"client_ip"`
	UserAgent       string    `json:"user_agent"`
	RequestHeaders  string    `json:"request_headers,omitempty"`
	ResponseHeaders string    `json:"response_headers,omitempty"`
	RequestBody     string    `json:"request_body,omitempty"`
	ResponseBody    string    `json:"response_body,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// LogRequestFull logs a request with full details (for request inspector).
func (db *DB) LogRequestFull(log *RequestLog) {
	// Fire and forget (don't block)
	go func() {
		db.Exec(`INSERT INTO request_logs
			(id, subdomain, method, path, status_code, duration_ms, client_ip, user_agent,
			 request_headers, response_headers, request_body, response_body, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.New().String(), log.Subdomain, log.Method, log.Path, log.StatusCode,
			log.DurationMs, log.ClientIP, log.UserAgent, log.RequestHeaders,
			log.ResponseHeaders, log.RequestBody, log.ResponseBody, time.Now())
	}()
}

func (db *DB) LogRequest(subdomain, method, path string, status, duration int) {
	// Fire and forget (don't block)
	go func() {
		db.Exec("INSERT INTO request_logs (id, subdomain, method, path, status_code, duration_ms) VALUES (?, ?, ?, ?, ?, ?)",
			uuid.New().String(), subdomain, method, path, status, duration)
	}()
}

// GetRequestLogs retrieves request logs for a subdomain with pagination.
func (db *DB) GetRequestLogs(subdomain string, limit, offset int) ([]RequestLog, error) {
	rows, err := db.Query(`
		SELECT id, subdomain, method, path, status_code, duration_ms,
		       COALESCE(client_ip, '') as client_ip,
		       COALESCE(user_agent, '') as user_agent,
		       COALESCE(request_headers, '') as request_headers,
		       COALESCE(response_headers, '') as response_headers,
		       COALESCE(request_body, '') as request_body,
		       COALESCE(response_body, '') as response_body,
		       created_at
		FROM request_logs
		WHERE subdomain = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`, subdomain, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		var log RequestLog
		if err := rows.Scan(&log.ID, &log.Subdomain, &log.Method, &log.Path,
			&log.StatusCode, &log.DurationMs, &log.ClientIP, &log.UserAgent,
			&log.RequestHeaders, &log.ResponseHeaders, &log.RequestBody,
			&log.ResponseBody, &log.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, nil
}

// GetRequestLog retrieves a single request log by ID.
func (db *DB) GetRequestLog(id string) (*RequestLog, error) {
	var log RequestLog
	err := db.QueryRow(`
		SELECT id, subdomain, method, path, status_code, duration_ms,
		       COALESCE(client_ip, '') as client_ip,
		       COALESCE(user_agent, '') as user_agent,
		       COALESCE(request_headers, '') as request_headers,
		       COALESCE(response_headers, '') as response_headers,
		       COALESCE(request_body, '') as request_body,
		       COALESCE(response_body, '') as response_body,
		       created_at
		FROM request_logs WHERE id = ?`, id).Scan(
		&log.ID, &log.Subdomain, &log.Method, &log.Path,
		&log.StatusCode, &log.DurationMs, &log.ClientIP, &log.UserAgent,
		&log.RequestHeaders, &log.ResponseHeaders, &log.RequestBody,
		&log.ResponseBody, &log.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &log, nil
}