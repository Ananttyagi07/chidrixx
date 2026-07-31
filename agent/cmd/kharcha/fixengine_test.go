// SPDX-License-Identifier: Apache-2.0
package main

import (
	"strings"
	"testing"
)

func TestGenerateFixManifestActionableClasses(t *testing.T) {
	for _, class := range []PathClass{PathInternetEgress, PathNATEgress, PathCrossRegion} {
		manifest := generateFixManifest(class, "checkout", "203.0.113.10")

		if manifest == "" {
			t.Fatalf("%s: expected a manifest, got none", class)
		}

		if !strings.Contains(manifest, "namespace: checkout") {
			t.Errorf("%s: manifest doesn't reference the real namespace:\n%s", class, manifest)
		}

		if !strings.Contains(manifest, `except: ["203.0.113.10/32"]`) {
			t.Errorf("%s: manifest doesn't scope to the real destination IP:\n%s", class, manifest)
		}

		if !strings.Contains(manifest, "kind: NetworkPolicy") {
			t.Errorf("%s: not a NetworkPolicy manifest:\n%s", class, manifest)
		}
	}
}

func TestGenerateFixManifestIPv6(t *testing.T) {
	manifest := generateFixManifest(PathInternetEgress, "checkout", "2001:db8::1")

	if !strings.Contains(manifest, "cidr: ::/0") {
		t.Errorf("expected an IPv6 allow-all block, got:\n%s", manifest)
	}

	if !strings.Contains(manifest, `except: ["2001:db8::1/128"]`) {
		t.Errorf("expected the IPv6 destination scoped with /128, got:\n%s", manifest)
	}
}

func TestGenerateFixManifestNoLabelsAvailable(t *testing.T) {
	// CROSS_AZ/MANAGED_SERVICE's real fix depends on pod labels this agent
	// doesn't resolve -- fabricating a label selector would be worse than
	// no manifest at all, so these must stay text-hint-only.
	for _, class := range []PathClass{PathCrossAZ, PathManagedService, PathSameNode, PathSameAZ} {
		if manifest := generateFixManifest(class, "checkout", "203.0.113.10"); manifest != "" {
			t.Errorf("%s: expected no manifest (would require fabricated labels), got:\n%s", class, manifest)
		}
	}
}

func TestGenerateFixManifestMissingInputs(t *testing.T) {
	cases := []struct {
		name      string
		namespace string
		destIP    string
	}{
		{"empty namespace", "", "203.0.113.10"},
		{"unresolved destination", "checkout", "?"},
		{"empty destination", "checkout", ""},
	}

	for _, c := range cases {
		if manifest := generateFixManifest(PathInternetEgress, c.namespace, c.destIP); manifest != "" {
			t.Errorf("%s: expected no manifest without complete information, got:\n%s", c.name, manifest)
		}
	}
}
