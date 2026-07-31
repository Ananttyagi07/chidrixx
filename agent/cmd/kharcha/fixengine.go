// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"net"
	"strings"
)

// generateFixManifest returns a concrete, copy-pasteable NetworkPolicy for
// path classes where the remediation is mechanically determinable from
// what this agent already knows for certain -- the source namespace and
// the flagged destination IP -- rather than a sentence telling the reader
// to go figure it out themselves.
//
// The policy allows all egress except the flagged destination: NetworkPolicies
// are allow-lists (once any policy selects a pod, everything not explicitly
// allowed is denied), so surgically blocking one wasteful destination means
// "allow 0.0.0.0/0 except this one," not a bare deny rule.
//
// Scope is deliberately limited to classes where namespace + destination IP
// is enough to act: INTERNET_EGRESS, NAT_EGRESS, CROSS_REGION. CROSS_AZ and
// MANAGED_SERVICE's real fix (co-locating workloads, Topology Aware Routing)
// depends on the destination's pod labels, which this agent doesn't resolve
// today -- guessing a label selector that might not match the real
// Deployment would be worse than the plain-text hint it already gets, so
// those stay a sentence rather than a fabricated manifest. podSelector: {}
// targets every pod in the namespace; narrowing it to one workload's own
// labels is left to the reader, who has that information and we don't.
func generateFixManifest(class PathClass, namespace, destIP string) string {
	if namespace == "" || net.ParseIP(destIP) == nil {
		return ""
	}

	switch class {
	case PathInternetEgress, PathNATEgress, PathCrossRegion:
		return networkPolicyDenyDestination(namespace, destIP)
	default:
		return ""
	}
}

func networkPolicyDenyDestination(namespace, destIP string) string {
	allCIDR := "0.0.0.0/0"
	destCIDR := destIP + "/32"

	if ip := net.ParseIP(destIP); ip != nil && ip.To4() == nil {
		allCIDR = "::/0"
		destCIDR = destIP + "/128"
	}

	name := "deny-" + strings.ReplaceAll(strings.ReplaceAll(destIP, ".", "-"), ":", "-")

	return fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: %s
  namespace: %s
spec:
  podSelector: {}   # narrow this to the specific workload's own labels
  policyTypes: [Egress]
  egress:
    - to:
        - ipBlock:
            cidr: %s
            except: ["%s"]
`, name, namespace, allCIDR, destCIDR)
}
