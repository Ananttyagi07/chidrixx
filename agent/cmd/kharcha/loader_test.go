package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadAttachesAndDetaches proves the actual Step-3 loader path — not
// just BPF_PROG_TEST_RUN in isolation, but the real link.AttachCgroup/Close
// lifecycle main() depends on. It attaches only to this test process's own
// cgroup v2 path (the container's own delegated cgroup when run inside a
// container) rather than a real host-wide root, so running it never affects
// traffic outside the test.
func TestLoadAttachesAndDetaches(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root/CAP_BPF to load and attach a BPF program; run as root or in CI")
	}

	loader, m, err := Load("../../../bpf/flow_cgroup.o", ownCgroupPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer loader.Close()

	if m == nil {
		t.Fatal("Load returned a nil flows map")
	}

	if err := loader.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// ownCgroupPath resolves the absolute cgroup v2 path of the current
// process from /proc/self/cgroup, so the test attaches only to its own
// (container-scoped) cgroup instead of a real host-wide root.
func ownCgroupPath(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		t.Fatalf("read /proc/self/cgroup: %v", err)
	}

	// cgroup v2 lines look like "0::/some/path"; take the token after the
	// last colon.
	line := strings.TrimSpace(string(data))

	idx := strings.LastIndex(line, ":")
	if idx < 0 {
		t.Fatalf("unexpected /proc/self/cgroup format: %q", line)
	}

	return filepath.Join("/sys/fs/cgroup", line[idx+1:])
}
