package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

type WorkloadResolver struct {
	mu    sync.RWMutex
	cache map[uint64]string
}

func NewWorkloadResolver() *WorkloadResolver {
	return &WorkloadResolver{
		cache: make(map[uint64]string),
	}
}

func (r *WorkloadResolver) Resolve(cgroupID uint64) string {
	r.mu.RLock()
	if name, ok := r.cache[cgroupID]; ok {
		r.mu.RUnlock()
		return name
	}
	r.mu.RUnlock()

	name := findCgroupName(cgroupID)

	r.mu.Lock()
	r.cache[cgroupID] = name
	r.mu.Unlock()

	return name
}

func findCgroupName(target uint64) string {
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

		// inode of a cgroup v2 directory corresponds to the cgroup ID
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil
		}

		if stat.Ino == target {
			rel, err := filepath.Rel(root, path)
			if err == nil {
				found = cleanCgroupName(rel)
			}
		}

		return nil
	})

	if found == "" {
		return "cgroup-" + strconv.FormatUint(target, 10)
	}

	return found
}

func cleanCgroupName(path string) string {
	if path == "." {
		return "root"
	}

	path = strings.TrimPrefix(path, "/")

	if len(path) > 80 {
		return "..." + path[len(path)-77:]
	}

	return path
}
