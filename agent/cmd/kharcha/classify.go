package main

import (
	"net"
	"strings"
)

// PathClass labels the network path a flow took, mirroring the build
// manual's decision tree (§10.1 / Step 8): topology first (same node, same
// zone, cross-zone, cross-region), then off-cluster destinations (managed
// service, private, NAT-egress heuristic, internet).
type PathClass string

const (
	PathSameNode          PathClass = "SAME_NODE"
	PathSameAZ            PathClass = "SAME_AZ"
	PathCrossAZ           PathClass = "CROSS_AZ"
	PathCrossRegion       PathClass = "CROSS_REGION"
	PathManagedService    PathClass = "MANAGED_SERVICE"
	PathPrivateOffCluster PathClass = "PRIVATE_OFFCLUSTER"
	PathNATEgress         PathClass = "NAT_EGRESS"
	PathInternetEgress    PathClass = "INTERNET_EGRESS"
)

// Confidence bands the pricing engine widens or narrows its estimate
// around — never a false-precision single number for a heuristic.
const (
	ConfHigh = "high"
	ConfMed  = "med"
	ConfLow  = "low"
)

// ClassifyInput is everything the classifier needs about one flow's
// endpoints. Fields the caller couldn't resolve are left zero/empty — the
// classifier degrades to a lower confidence instead of guessing wrong with
// false certainty.
type ClassifyInput struct {
	Remote net.IP

	SourceNode string // node the local workload runs on, if known
	DestNode   string // node the remote workload runs on, if known

	Managed bool // remote matched a configured managed-service CIDR

	// NodeHasPublicIP is a deployment-time fact about this node's own
	// internet path, not something a cgroup_skb hook can observe — it
	// comes from config/flag. Defaults true so a plain dev box doesn't get
	// every internet flow mislabeled as NAT egress.
	NodeHasPublicIP bool
}

// ZoneLookup resolves a node name to its topology.kubernetes.io/zone
// label. Returning "" means "unknown," never "same zone."
type ZoneLookup func(node string) string

// Classify labels a flow's path and states how confident that label is.
// Confidence is "high" whenever the label follows directly from
// Kubernetes-observed facts (node identity, zone labels, EndpointSlice
// membership); it drops for heuristics — the NAT-egress guess, or
// in-cluster traffic on nodes with no zone label at all (single-node lab
// clusters, bare kubeadm without a cloud-controller-manager) — exactly the
// cases the build manual says must never be presented as high-confidence.
func Classify(in ClassifyInput, zoneOf ZoneLookup) (PathClass, string) {
	if in.Remote == nil {
		return PathInternetEgress, ConfLow
	}

	if in.Remote.IsLoopback() {
		return PathSameNode, ConfHigh
	}

	if in.DestNode != "" {
		if in.SourceNode != "" && in.DestNode == in.SourceNode {
			return PathSameNode, ConfHigh
		}

		srcZone, dstZone := zoneOf(in.SourceNode), zoneOf(in.DestNode)

		if srcZone == "" || dstZone == "" {
			// Both endpoints are in the cluster, but at least one node
			// carries no zone label. We know it's in-cluster traffic; we
			// don't know if it crosses a billed zone boundary.
			return PathSameAZ, ConfLow
		}

		if srcZone == dstZone {
			return PathSameAZ, ConfHigh
		}

		if region(srcZone) == region(dstZone) {
			return PathCrossAZ, ConfHigh
		}

		return PathCrossRegion, ConfHigh
	}

	if in.Managed {
		return PathManagedService, ConfHigh
	}

	if isPrivate(in.Remote) {
		return PathPrivateOffCluster, ConfHigh
	}

	if !in.NodeHasPublicIP {
		return PathNATEgress, ConfMed
	}

	return PathInternetEgress, ConfHigh
}

func isPrivate(ip net.IP) bool {
	for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return true
		}
	}

	return false
}

// region strips a zone's trailing letter suffix ("ap-south-1a" ->
// "ap-south-1"), matching AWS/GCP zone-naming convention. Zones that don't
// follow it just compare unequal to everything, which only ever pushes the
// classifier to the more conservative CROSS_REGION instead of CROSS_AZ —
// never wrong in the direction that hides cost.
func region(zone string) string {
	return strings.TrimRight(zone, "abcdefghijklmnopqrstuvwxyz")
}
