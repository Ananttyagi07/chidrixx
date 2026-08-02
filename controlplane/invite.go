// SPDX-License-Identifier: Apache-2.0
package main

import (
	"database/sql"
	"fmt"
	"time"
)

// Invite is a real, admin-created pending invitation: a specific email
// address will join this specific tenant with this specific role the
// first time they ever sign in (see AcceptInvite, and the check in
// auth.go's handleSupabaseBearer that runs before falling back to
// provisioning a brand-new tenant) -- this is what lets an admin add a
// teammate without shell access to the control plane, the gap this was
// built to close.
type Invite struct {
	TenantID  int64
	Email     string
	Role      string
	CreatedAt time.Time
}

// CreateInvite upserts a pending invite for an email -- inviting the same
// email again (to the same or a different tenant/role) replaces the
// previous pending invite rather than erroring or duplicating; only the
// most recent invite for a given email can ever be pending at once,
// enforced by the real UNIQUE constraint on invites.email, not just
// application logic.
func (s *Store) CreateInvite(tenantID int64, email, role string) error {
	if role != RoleAdmin && role != RoleViewer {
		return fmt.Errorf("invalid role %q: must be %q or %q", role, RoleAdmin, RoleViewer)
	}

	_, err := s.db.Exec(
		`INSERT INTO invites (tenant_id, email, role, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(email) DO UPDATE SET tenant_id = excluded.tenant_id, role = excluded.role, created_at = excluded.created_at`,
		tenantID, email, role, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("create invite: %w", err)
	}
	return nil
}

// ListInvites returns every pending invite for a tenant, most recent
// first.
func (s *Store) ListInvites(tenantID int64) ([]Invite, error) {
	rows, err := s.db.Query(
		`SELECT tenant_id, email, role, created_at FROM invites WHERE tenant_id = ? ORDER BY created_at DESC`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("query invites: %w", err)
	}
	defer rows.Close()

	out := make([]Invite, 0)
	for rows.Next() {
		var inv Invite
		var createdAt int64
		if err := rows.Scan(&inv.TenantID, &inv.Email, &inv.Role, &createdAt); err != nil {
			return nil, fmt.Errorf("scan invite: %w", err)
		}
		inv.CreatedAt = time.Unix(createdAt, 0)
		out = append(out, inv)
	}
	return out, rows.Err()
}

// DeleteInvite revokes a pending invite -- an admin changing their mind
// before the invitee ever signs in.
func (s *Store) DeleteInvite(tenantID int64, email string) error {
	_, err := s.db.Exec(`DELETE FROM invites WHERE tenant_id = ? AND email = ?`, tenantID, email)
	if err != nil {
		return fmt.Errorf("delete invite: %w", err)
	}
	return nil
}

// getPendingInvite looks up a pending invite by email -- unexported since
// the only caller is the first-login resolution path in auth.go, which
// always needs to consume it via AcceptInvite in the same breath, never
// just read it.
func (s *Store) getPendingInvite(email string) (*Invite, error) {
	var inv Invite
	var createdAt int64
	err := s.db.QueryRow(
		`SELECT tenant_id, email, role, created_at FROM invites WHERE email = ?`,
		email,
	).Scan(&inv.TenantID, &inv.Email, &inv.Role, &createdAt)
	if err != nil {
		return nil, err
	}
	inv.CreatedAt = time.Unix(createdAt, 0)
	return &inv, nil
}

// AcceptInvite is ProvisionTenantForSupabaseUser's counterpart for a
// teammate joining an *existing* tenant instead of getting a brand-new
// one: creates the user row on the invite's tenant with the invite's
// role, linked to the real Supabase identity, and consumes (deletes) the
// invite -- all in one transaction, so a crash mid-accept never leaves a
// half-joined user or a stale-but-still-pending invite.
func (s *Store) AcceptInvite(supabaseUserID string, invite *Invite) (*User, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().Unix()

	res, err := tx.Exec(
		`INSERT INTO users (tenant_id, username, password_hash, role, supabase_user_id, created_at) VALUES (?, ?, '', ?, ?, ?)`,
		invite.TenantID, invite.Email, invite.Role, supabaseUserID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert invited user: %w", err)
	}
	userID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("user id: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM invites WHERE email = ?`, invite.Email); err != nil {
		return nil, fmt.Errorf("consume invite: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &User{
		ID: userID, TenantID: invite.TenantID, Username: invite.Email, Role: invite.Role,
		SupabaseUserID: supabaseUserID, CreatedAt: time.Unix(now, 0),
	}, nil
}

// ResolveOrProvisionSupabaseUser is the single real entry point
// handleSupabaseBearer (auth.go) calls for a first-ever-seen Supabase
// identity: join a pending invite's tenant if one exists for this real
// email, otherwise provision a brand-new tenant as the founding admin.
// Pulled into its own function (rather than left inline in auth.go)
// because this exact precedence -- invite before new-tenant -- is the
// one rule that actually matters here and deserves to be named, not
// buried in a middleware.
func (s *Store) ResolveOrProvisionSupabaseUser(supabaseUserID, email string) (*User, error) {
	invite, err := s.getPendingInvite(email)
	if err == nil {
		return s.AcceptInvite(supabaseUserID, invite)
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("check pending invite: %w", err)
	}

	user, _, err := s.ProvisionTenantForSupabaseUser(supabaseUserID, email)
	return user, err
}
