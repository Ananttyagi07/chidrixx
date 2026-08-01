// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// TeamOwnership is one real namespace -> team mapping an admin configured
// for their tenant -- "who owns this?" answered from data the operator
// actually typed in, never guessed.
type TeamOwnership struct {
	Namespace string `json:"namespace"`
	Team      string `json:"team"`
}

// SetTeamOwnership upserts one namespace's team mapping.
func (s *Store) SetTeamOwnership(tenantID int64, namespace, team string) error {
	_, err := s.db.Exec(
		`INSERT INTO team_ownership (tenant_id, namespace, team, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(tenant_id, namespace) DO UPDATE SET team = excluded.team`,
		tenantID, namespace, team, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("set team ownership: %w", err)
	}
	return nil
}

// DeleteTeamOwnership removes one namespace's mapping -- it falls back to
// "Unassigned" in SpendByTeam, not silently disappearing from view.
func (s *Store) DeleteTeamOwnership(tenantID int64, namespace string) error {
	_, err := s.db.Exec(`DELETE FROM team_ownership WHERE tenant_id = ? AND namespace = ?`, tenantID, namespace)
	if err != nil {
		return fmt.Errorf("delete team ownership: %w", err)
	}
	return nil
}

// ListTeamOwnership returns every namespace->team mapping configured for
// a tenant, alphabetical by namespace.
func (s *Store) ListTeamOwnership(tenantID int64) ([]TeamOwnership, error) {
	rows, err := s.db.Query(
		`SELECT namespace, team FROM team_ownership WHERE tenant_id = ? ORDER BY namespace`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("query team ownership: %w", err)
	}
	defer rows.Close()

	out := make([]TeamOwnership, 0)
	for rows.Next() {
		var t TeamOwnership
		if err := rows.Scan(&t.Namespace, &t.Team); err != nil {
			return nil, fmt.Errorf("scan team ownership: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// namespaceLabelPattern matches a real Kubernetes namespace name (RFC 1123
// DNS label). Used to tell an actual "namespace/pod" source (agent's
// KubeWorkload.DisplayName(), agent/cmd/kharcha/kubernetes.go) apart from
// a raw cgroup path fallback like
// "user-1000.slice/user@1000.service/app.slice/...", which also contains
// slashes but never looks like this -- a cgroup segment always has a dot
// or an "@" in it, which no real namespace name can contain.
var namespaceLabelPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// extractNamespace pulls the real Kubernetes namespace out of a finding's
// Source string, returning ok=false when the source isn't a real
// namespace/pod pair -- e.g. a host-level cgroup path, which genuinely
// has no namespace to attribute and must fall back to "Unassigned" rather
// than a fabricated guess.
func extractNamespace(source string) (namespace string, ok bool) {
	prefix, _, found := strings.Cut(source, "/")
	if !found || prefix == "" {
		return "", false
	}
	if !namespaceLabelPattern.MatchString(prefix) {
		return "", false
	}
	return prefix, true
}

// TeamSpend is the real cost attributed to one team, computed by mapping
// each finding's real namespace through the tenant's configured ownership
// table -- unmapped or non-k8s-resolved sources fold into "Unassigned"
// rather than being dropped or guessed at.
type TeamSpend struct {
	Team         string  `json:"team"`
	CostHighINR  float64 `json:"cost_high_inr"`
	FindingCount int     `json:"finding_count"`
}

const unassignedTeam = "Unassigned"

// computeSpendByTeam groups the given findings' real cost by team, using
// the real namespace->team mapping already loaded. Kept as a pure function
// over data the caller already fetched (LatestFindings) rather than a new
// query, since dashboard-summary already has both in hand.
func computeSpendByTeam(findings []FindingRow, ownership []TeamOwnership) []TeamSpend {
	teamByNamespace := make(map[string]string, len(ownership))
	for _, o := range ownership {
		teamByNamespace[o.Namespace] = o.Team
	}

	byTeam := make(map[string]*TeamSpend)
	order := make([]string, 0)

	for _, f := range findings {
		team := unassignedTeam
		if ns, ok := extractNamespace(f.Source); ok {
			if mapped, exists := teamByNamespace[ns]; exists && mapped != "" {
				team = mapped
			}
		}

		ts, exists := byTeam[team]
		if !exists {
			ts = &TeamSpend{Team: team}
			byTeam[team] = ts
			order = append(order, team)
		}
		ts.CostHighINR += f.CostHighINR
		ts.FindingCount++
	}

	out := make([]TeamSpend, 0, len(order))
	for _, team := range order {
		out = append(out, *byTeam[team])
	}

	// Highest spend first, but keep "Unassigned" out of the way at the end
	// when it's not actually the biggest bucket -- it's a fallback, not
	// necessarily the headline.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].CostHighINR > out[j-1].CostHighINR; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}

	return out
}
