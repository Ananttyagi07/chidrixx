// SPDX-License-Identifier: Apache-2.0
package main

import (
	"testing"
	"time"
)

func replicaCount(n int32) *int32 { return &n }

func deployment(namespace, name string, replicas int32) struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Replicas *int32 `json:"replicas"`
	} `json:"spec"`
} {
	var d struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			Replicas *int32 `json:"replicas"`
		} `json:"spec"`
	}
	d.Metadata.Namespace = namespace
	d.Metadata.Name = name
	d.Spec.Replicas = replicaCount(replicas)
	return d
}

func TestDiffReplicasSkipsFirstObservation(t *testing.T) {
	list := kubeDeploymentList{Items: []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			Replicas *int32 `json:"replicas"`
		} `json:"spec"`
	}{deployment("checkout", "checkout", 3)}}

	updated, events := diffReplicas(list, nil, time.Now())

	if len(events) != 0 {
		t.Fatalf("expected no event on first-ever observation (no baseline to diff against), got: %+v", events)
	}
	if updated["checkout/checkout"] != 3 {
		t.Fatalf("expected baseline recorded as 3, got %+v", updated)
	}
}

func TestDiffReplicasDetectsIncreaseAndDecrease(t *testing.T) {
	list := kubeDeploymentList{Items: []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			Replicas *int32 `json:"replicas"`
		} `json:"spec"`
	}{
		deployment("checkout", "checkout", 8),
		deployment("ai-gateway", "ai-gateway", 1),
	}}
	last := map[string]int32{
		"checkout/checkout":     2, // 2 -> 8, increase
		"ai-gateway/ai-gateway": 4, // 4 -> 1, decrease
	}

	_, events := diffReplicas(list, last, time.Now())

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(events), events)
	}

	byNamespace := make(map[string]DeployEvent)
	for _, e := range events {
		byNamespace[e.Namespace] = e
	}

	if ev := byNamespace["checkout"]; ev.Message != "replicas increased from 2 to 8" {
		t.Errorf("checkout event = %+v, want increased 2->8", ev)
	}
	if ev := byNamespace["ai-gateway"]; ev.Message != "replicas decreased from 4 to 1" {
		t.Errorf("ai-gateway event = %+v, want decreased 4->1", ev)
	}
}

func TestDiffReplicasNoEventWhenUnchanged(t *testing.T) {
	list := kubeDeploymentList{Items: []struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			Replicas *int32 `json:"replicas"`
		} `json:"spec"`
	}{deployment("checkout", "checkout", 5)}}
	last := map[string]int32{"checkout/checkout": 5}

	_, events := diffReplicas(list, last, time.Now())

	if len(events) != 0 {
		t.Fatalf("expected no events for an unchanged replica count, got: %+v", events)
	}
}

func TestDrainDeployEventsClearsBuffer(t *testing.T) {
	r := &KubernetesResolver{
		pendingEvents: []DeployEvent{{Namespace: "checkout", Name: "checkout", Reason: "ReplicaCountChanged"}},
	}

	drained := r.DrainDeployEvents()
	if len(drained) != 1 {
		t.Fatalf("expected 1 drained event, got %d", len(drained))
	}

	drainedAgain := r.DrainDeployEvents()
	if len(drainedAgain) != 0 {
		t.Fatalf("expected an empty buffer after drain, got %d events", len(drainedAgain))
	}
}
