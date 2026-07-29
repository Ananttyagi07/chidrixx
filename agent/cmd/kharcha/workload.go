// SPDX-License-Identifier: Apache-2.0
package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type WorkloadIdentity struct {
	CgroupID   uint64
	CgroupPath string
	Kubernetes *KubeWorkload
}

func (w WorkloadIdentity) DisplayName() string {
	if w.Kubernetes != nil {
		return w.Kubernetes.DisplayName()
	}

	return w.CgroupPath
}

type WorkloadResolver struct {
	mu    sync.RWMutex
	cache map[uint64]WorkloadIdentity
	kube  *KubernetesResolver
}

func NewWorkloadResolver(kube *KubernetesResolver) *WorkloadResolver {
	return &WorkloadResolver{
		cache: make(map[uint64]WorkloadIdentity),
		kube:  kube,
	}
}

func (r *WorkloadResolver) Resolve(cgroupID uint64) WorkloadIdentity {
	r.mu.RLock()
	if identity, ok := r.cache[cgroupID]; ok {
		r.mu.RUnlock()
		return identity
	}
	r.mu.RUnlock()

	path := findCgroupPath(cgroupID)

	identity := WorkloadIdentity{
		CgroupID:   cgroupID,
		CgroupPath: cleanCgroupName(path),
	}

	if r.kube != nil {
		podUID, containerID := extractKubernetesIDs(path)

		if containerID != "" {
			identity.Kubernetes = r.kube.ResolveContainerID(containerID)
		}

		if identity.Kubernetes == nil && podUID != "" {
			identity.Kubernetes = r.kube.ResolvePodUID(podUID)
		}
	}

	r.mu.Lock()
	r.cache[cgroupID] = identity
	r.mu.Unlock()

	return identity
}

func findCgroupPath(target uint64) string {
	root := "/sys/fs/cgroup"

	var found string

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}

		if !d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil
		}

		if stat.Ino == target {
			rel, err := filepath.Rel(root, path)
			if err == nil {
				found = rel
			}
		}

		return nil
	})

	if found == "" {
		return "cgroup-" + strconv.FormatUint(target, 10)
	}

	return found
}

func extractKubernetesIDs(path string) (string, string) {
	if path == "" {
		return "", ""
	}

	parts := strings.Split(filepath.ToSlash(path), "/")

	var podUID string
	var containerID string

	for i, part := range parts {
		if strings.HasPrefix(part, "pod") && len(part) > 3 {
			candidate := strings.TrimPrefix(part, "pod")

			// Kubernetes UIDs have hyphens. This prevents accidentally
			// interpreting unrelated directories as pod IDs.
			if strings.Count(candidate, "-") >= 4 {
				podUID = candidate

				if i+1 < len(parts) {
					next := parts[i+1]

					// A containerd container ID is normally a
					// 64-character hexadecimal string.
					if len(next) >= 12 {
						containerID = next
					}
				}
			}
		}
	}

	return podUID, containerID
}

func cleanCgroupName(path string) string {
	if path == "." || path == "" {
		return "root"
	}

	path = strings.TrimPrefix(path, "/")

	if len(path) > 80 {
		return "..." + path[len(path)-77:]
	}

	return path
}
