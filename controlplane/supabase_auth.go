// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SupabaseAuthenticator verifies a Supabase Auth access token by asking
// Supabase itself whether it's valid -- calling GET /auth/v1/user with the
// token as Bearer auth. This is deliberately not local JWT verification
// (no JWKS fetching/caching, no signing-algorithm assumptions to get
// wrong): Supabase is the actual source of truth for whether a session is
// valid or has been revoked, so asking it directly is the most honestly
// correct approach, at the cost of one extra HTTP round-trip per
// authenticated request. Good enough for an MVP's request volume;
// revisit with local JWKS verification + caching if that round-trip ever
// shows up as a real bottleneck.
type SupabaseAuthenticator struct {
	baseURL        string
	publishableKey string
	httpClient     *http.Client
}

func NewSupabaseAuthenticator(baseURL, publishableKey string) *SupabaseAuthenticator {
	return &SupabaseAuthenticator{
		baseURL:        strings.TrimRight(baseURL, "/"),
		publishableKey: publishableKey,
		httpClient:     &http.Client{Timeout: 5 * time.Second},
	}
}

// SupabaseUser is the subset of Supabase's /auth/v1/user response this
// control plane actually needs.
type SupabaseUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

var ErrInvalidSupabaseToken = errors.New("invalid or expired supabase token")

// VerifyToken calls Supabase's own user-info endpoint with the given
// access token. A 200 with a real user body is the only thing trusted as
// "authenticated" -- any other status is treated as invalid, not
// papered over.
func (a *SupabaseAuthenticator) VerifyToken(accessToken string) (*SupabaseUser, error) {
	req, err := http.NewRequest(http.MethodGet, a.baseURL+"/auth/v1/user", nil)
	if err != nil {
		return nil, fmt.Errorf("build supabase user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("apikey", a.publishableKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call supabase: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrInvalidSupabaseToken
	}

	var user SupabaseUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode supabase user response: %w", err)
	}
	if user.ID == "" {
		return nil, ErrInvalidSupabaseToken
	}

	return &user, nil
}
