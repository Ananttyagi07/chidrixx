// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"fmt"
	"time"
)

// DeployEvent is a real, observed change to a Deployment's replica count --
// the concrete signal behind "cost jumped right after this deployment
// scaled" correlation (build manual gap: no automatic root-cause
// analysis). Never a guess: it's a direct diff between two real API
// snapshots of spec.replicas for the same namespace/name.
type DeployEvent struct {
	Namespace  string    `json:"namespace"`
	Name       string    `json:"name"`
	Reason     string    `json:"reason"`
	Message    string    `json:"message"`
	OccurredAt time.Time `json:"occurred_at"`
}

type kubeDeploymentList struct {
	Items []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			Replicas *int32 `json:"replicas"`
		} `json:"spec"`
	} `json:"items"`
}

// diffReplicas is the pure comparison at the heart of deploy-event
// detection: given the current API snapshot and the last-seen replica
// count per namespace/deployment, it returns the updated snapshot to keep
// and one DeployEvent per deployment whose replica count actually changed.
// Kept separate from the network fetch in refreshDeployEvents so the real
// diff logic is unit-testable without a live (or mocked) Kubernetes API.
func diffReplicas(list kubeDeploymentList, last map[string]int32, now time.Time) (updated map[string]int32, events []DeployEvent) {
	updated = make(map[string]int32, len(last))
	for k, v := range last {
		updated[k] = v
	}

	for _, d := range list.Items {
		if d.Spec.Replicas == nil {
			continue
		}
		key := d.Metadata.Namespace + "/" + d.Metadata.Name
		replicas := *d.Spec.Replicas
		prev, seen := last[key]
		updated[key] = replicas

		if !seen || prev == replicas {
			continue
		}

		direction := "increased"
		if replicas < prev {
			direction = "decreased"
		}
		events = append(events, DeployEvent{
			Namespace:  d.Metadata.Namespace,
			Name:       d.Metadata.Name,
			Reason:     "ReplicaCountChanged",
			Message:    fmt.Sprintf("replicas %s from %d to %d", direction, prev, replicas),
			OccurredAt: now,
		})
	}

	return updated, events
}

// refreshDeployEvents fetches every Deployment's current replica count and
// diffs it against the last-seen snapshot (diffReplicas), buffering any
// resulting events. Best-effort: an agent whose ClusterRole predates this
// feature (RBAC not yet upgraded) degrades to "no events detected" rather
// than breaking the refresh loop that pod/service/node resolution depends
// on -- deploy correlation is a bonus signal layered on top of the core
// attribution guarantee, not something worth failing ingest over.
func (r *KubernetesResolver) refreshDeployEvents() {
	out, err := r.fetch("/apis/apps/v1/deployments", "get", "deployments", "-A", "-o", "json")
	if err != nil {
		return
	}

	var list kubeDeploymentList
	if err := json.Unmarshal(out, &list); err != nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	updated, events := diffReplicas(list, r.lastReplicas, time.Now())
	r.lastReplicas = updated
	r.pendingEvents = append(r.pendingEvents, events...)
}

// DrainDeployEvents returns every buffered event since the last call and
// clears the buffer -- called from the ship loop (main.go), which runs on
// a different ticker than the Kubernetes metadata refresh loop that
// produces these.
func (r *KubernetesResolver) DrainDeployEvents() []DeployEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := r.pendingEvents
	r.pendingEvents = nil
	return out
}
