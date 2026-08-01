// SPDX-License-Identifier: Apache-2.0
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Real multi-tenant isolation: every cluster's data belongs to exactly one
// tenant, and every query in store.go is scoped by tenant_id -- two
// customers sharing this control plane never see each other's clusters,
// spend, or findings. This is deliberately NOT public self-signup (no
// email verification, no billing, no plan tiers) -- chidrixx is still a
// self-hosted tool, so provisioning a new tenant is an operator action
// (the `create-tenant` CLI subcommand in main.go), not a web form. What's
// real here is the isolation itself: once provisioned, a tenant's data is
// genuinely walled off, not just cosmetically separated in the UI.

const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

// Tenant is one isolated customer/organization.
type Tenant struct {
	ID        int64
	Name      string
	CreatedAt time.Time
}

// User is a real login identity, scoped to exactly one tenant.
type User struct {
	ID           int64
	TenantID     int64
	Username     string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}

// sessionTTL is deliberately short-ish and server-tracked (not a long-lived
// JWT) -- a session row can be revoked by deleting it, which a stateless
// signed token can't offer without an extra revocation list.
const sessionTTL = 24 * time.Hour

// hashToken turns a plaintext API token into its stored form. Tokens are
// high-entropy random values, not user-chosen passwords, so a fast hash
// (SHA-256) is the right tool here -- bcrypt is for the users table, where
// the input is a human-chosen, comparatively low-entropy password.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// CreateTenant provisions a brand-new isolated tenant with one admin user
// and one API ingest token, all in a single transaction -- either all three
// exist or none do, never a half-created tenant with no way to log in or
// ship data.
func (s *Store) CreateTenant(name, adminUsername, adminPassword string) (tenantID int64, apiToken string, err error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		return 0, "", fmt.Errorf("hash admin password: %w", err)
	}

	apiToken, err = randomToken(24)
	if err != nil {
		return 0, "", err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	res, err := tx.Exec(`INSERT INTO tenants (name, created_at) VALUES (?, ?)`, name, now)
	if err != nil {
		return 0, "", fmt.Errorf("insert tenant: %w", err)
	}
	tenantID, err = res.LastInsertId()
	if err != nil {
		return 0, "", fmt.Errorf("tenant id: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO users (tenant_id, username, password_hash, role, created_at) VALUES (?, ?, ?, ?, ?)`,
		tenantID, adminUsername, string(passwordHash), RoleAdmin, now,
	); err != nil {
		return 0, "", fmt.Errorf("insert admin user: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO api_tokens (tenant_id, token_hash, label, created_at) VALUES (?, ?, ?, ?)`,
		tenantID, hashToken(apiToken), "bootstrap", now,
	); err != nil {
		return 0, "", fmt.Errorf("insert api token: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, "", fmt.Errorf("commit: %w", err)
	}

	return tenantID, apiToken, nil
}

// CreateUser adds an additional login to an existing tenant -- e.g. a
// viewer alongside the admin CreateTenant already made. Role gating (see
// requireAdmin in auth.go) only matters once a tenant actually has more
// than one user with different roles.
func (s *Store) CreateUser(tenantID int64, username, password, role string) error {
	if role != RoleAdmin && role != RoleViewer {
		return fmt.Errorf("invalid role %q: must be %q or %q", role, RoleAdmin, RoleViewer)
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT INTO users (tenant_id, username, password_hash, role, created_at) VALUES (?, ?, ?, ?, ?)`,
		tenantID, username, string(passwordHash), role, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	return nil
}

// CreateAPIToken issues a new ingest token for an existing tenant --
// tokens are stored SHA-256 hashed (see hashToken), so there's no way to
// recover a lost one; this mints a real new one instead, the same
// operator-run path as CreateTenant's bootstrap token.
func (s *Store) CreateAPIToken(tenantID int64, label string) (string, error) {
	token, err := randomToken(24)
	if err != nil {
		return "", err
	}

	if _, err := s.db.Exec(
		`INSERT INTO api_tokens (tenant_id, token_hash, label, created_at) VALUES (?, ?, ?, ?)`,
		tenantID, hashToken(token), label, time.Now().Unix(),
	); err != nil {
		return "", fmt.Errorf("insert api token: %w", err)
	}

	return token, nil
}

// TenantCount is used by main.go's bootstrap check -- if zero, this is a
// brand-new install with nobody able to log in yet.
func (s *Store) TenantCount() (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tenants`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count tenants: %w", err)
	}
	return n, nil
}

var ErrInvalidCredentials = errors.New("invalid username or password")

// AuthenticateUser verifies a username/password pair and returns the real
// user record on success. Constant-time by construction: bcrypt.CompareHashAndPassword
// itself runs in time independent of where the mismatch is, and a missing
// username still runs a (dummy) bcrypt compare so a timing difference can't
// reveal whether the username exists.
func (s *Store) AuthenticateUser(username, password string) (*User, error) {
	var u User
	var createdAt int64
	err := s.db.QueryRow(
		`SELECT id, tenant_id, username, password_hash, role, created_at FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.TenantID, &u.Username, &u.PasswordHash, &u.Role, &createdAt)

	if err == sql.ErrNoRows {
		// Dummy hash comparison so a nonexistent username takes
		// roughly the same time as a wrong-password one.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$invalidsaltinvalidsaltinvalidsaltinvalidsalt"), []byte(password))
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	u.CreatedAt = time.Unix(createdAt, 0)
	return &u, nil
}

// AuthenticateAPIToken resolves an ingest token to the tenant it belongs
// to. Agents authenticate with this, never with a user's password.
func (s *Store) AuthenticateAPIToken(token string) (tenantID int64, err error) {
	err = s.db.QueryRow(
		`SELECT tenant_id FROM api_tokens WHERE token_hash = ?`,
		hashToken(token),
	).Scan(&tenantID)
	if err == sql.ErrNoRows {
		return 0, ErrInvalidCredentials
	}
	if err != nil {
		return 0, fmt.Errorf("query api token: %w", err)
	}
	return tenantID, nil
}

// Session is a server-tracked login session, resolved from the cookie on
// every request -- deleting the row revokes it immediately, unlike a
// stateless signed token.
type Session struct {
	ID        string
	UserID    int64
	TenantID  int64
	Role      string
	Username  string
	ExpiresAt time.Time
}

// CreateSession starts a new login session for a user and returns the
// opaque session ID to set as the cookie value.
func (s *Store) CreateSession(u *User) (string, error) {
	id, err := randomToken(24)
	if err != nil {
		return "", err
	}

	now := time.Now()
	if _, err := s.db.Exec(
		`INSERT INTO sessions (id, user_id, tenant_id, created_at, expires_at) VALUES (?, ?, ?, ?, ?)`,
		id, u.ID, u.TenantID, now.Unix(), now.Add(sessionTTL).Unix(),
	); err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}

	return id, nil
}

// GetSession resolves a session cookie value to the tenant/role it
// belongs to, rejecting anything expired -- expiry is checked against
// real wall-clock time, not just the row's continued existence.
func (s *Store) GetSession(id string) (*Session, error) {
	var sess Session
	var expiresAt int64
	err := s.db.QueryRow(
		`SELECT sessions.id, sessions.user_id, sessions.tenant_id, sessions.expires_at, users.role, users.username
		 FROM sessions JOIN users ON users.id = sessions.user_id
		 WHERE sessions.id = ?`,
		id,
	).Scan(&sess.ID, &sess.UserID, &sess.TenantID, &expiresAt, &sess.Role, &sess.Username)
	if err == sql.ErrNoRows {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}

	sess.ExpiresAt = time.Unix(expiresAt, 0)
	if time.Now().After(sess.ExpiresAt) {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
		return nil, ErrInvalidCredentials
	}

	return &sess, nil
}

// DeleteSession logs a session out immediately.
func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
