package main

import (
	"net"
	"os/exec"
	"testing"
)

// TestKubernetesResolverAgainstRealCluster exercises the K8s enrichment and
// classifier against whatever cluster kubectl is currently pointed at,
// rather than mocking the API. It's gated on the "chidrixx-test" namespace
// (this project's own lab fixture — server Service + client/server Pods
// created ahead of time) so it skips cleanly anywhere else instead of
// failing on missing fixtures.
func TestKubernetesResolverAgainstRealCluster(t *testing.T) {
	if err := exec.Command("kubectl", "get", "ns", "chidrixx-test").Run(); err != nil {
		t.Skip("chidrixx-test namespace not present; skipping real-cluster check")
	}

	kube := NewKubernetesResolver()
	if err := kube.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	svc := kube.ResolveServiceIP(mustParseIP(t, mustServiceClusterIP(t, "chidrixx-test", "server")))
	if svc == nil {
		t.Fatal("expected the chidrixx-test/server Service to resolve by ClusterIP")
	}
	if svc.Namespace != "chidrixx-test" || svc.Name != "server" {
		t.Fatalf("resolved service = %s/%s, want chidrixx-test/server", svc.Namespace, svc.Name)
	}

	backends := kube.ResolveServiceBackends(svc)
	if len(backends) == 0 {
		t.Fatal("expected at least one EndpointSlice backend for chidrixx-test/server")
	}

	// This lab is single-node, so every pod's node has no zone label —
	// Zone() must degrade to "" rather than fabricate one, and a
	// same-node flow must still classify as SAME_NODE with high
	// confidence even without any zone information at all.
	nodeName := backends[0].NodeName
	if nodeName == "" {
		t.Fatal("expected the resolved backend to carry a node name")
	}

	if zone := kube.Zone(nodeName); zone != "" {
		t.Fatalf("Zone(%s) = %q, want \"\" (this lab's node carries no zone label)", nodeName, zone)
	}

	class, confidence := Classify(ClassifyInput{
		Remote:          net.ParseIP("10.42.0.1"), // any address; only node identity matters here
		SourceNode:      nodeName,
		DestNode:        nodeName,
		NodeHasPublicIP: true,
	}, kube.Zone)

	if class != PathSameNode || confidence != ConfHigh {
		t.Fatalf("same-node flow classified as %s/%s, want %s/%s", class, confidence, PathSameNode, ConfHigh)
	}
}

func mustServiceClusterIP(t *testing.T, namespace, name string) string {
	t.Helper()

	out, err := exec.Command(
		"kubectl", "get", "svc", "-n", namespace, name,
		"-o", "jsonpath={.spec.clusterIP}",
	).Output()
	if err != nil {
		t.Fatalf("kubectl get svc %s/%s: %v", namespace, name, err)
	}

	return string(out)
}

func mustParseIP(t *testing.T, s string) net.IP {
	t.Helper()

	ip := net.ParseIP(s)
	if ip == nil {
		t.Fatalf("not a valid IP: %q", s)
	}

	return ip
}
