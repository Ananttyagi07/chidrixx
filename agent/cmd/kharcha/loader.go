// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

// Loader owns the lifetime of the loaded eBPF collection and the links that
// attach its programs to a cgroup. Closing it detaches the programs and
// releases every kernel resource the collection holds.
type Loader struct {
	coll  *ebpf.Collection
	links []link.Link
}

// Load reads the compiled cgroup_skb object at objPath and attaches its
// egress/ingress programs at cgroupPath (a cgroup v2 mount, normally the
// root), returning the flows map the agent scrapes on its own ticker.
//
// This replaces the earlier assumption that something outside the agent
// (bpftool, a shell script) had already loaded and pinned the map.
func Load(objPath, cgroupPath string) (*Loader, *ebpf.Map, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, nil, fmt.Errorf("remove memlock rlimit: %w", err)
	}

	spec, err := ebpf.LoadCollectionSpec(objPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load collection spec %s: %w", objPath, err)
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, nil, fmt.Errorf("load bpf collection (needs root/CAP_BPF): %w", err)
	}

	l := &Loader{coll: coll}

	if err := l.attach(cgroupPath, "kharcha_egress", ebpf.AttachCGroupInetEgress); err != nil {
		l.Close()
		return nil, nil, err
	}

	if err := l.attach(cgroupPath, "kharcha_ingress", ebpf.AttachCGroupInetIngress); err != nil {
		l.Close()
		return nil, nil, err
	}

	m := coll.Maps["flows"]
	if m == nil {
		l.Close()
		return nil, nil, fmt.Errorf("flows map not present in %s", objPath)
	}

	return l, m, nil
}

func (l *Loader) attach(cgroupPath, progName string, attachType ebpf.AttachType) error {
	prog := l.coll.Programs[progName]
	if prog == nil {
		return fmt.Errorf("program %s not present in object", progName)
	}

	lnk, err := link.AttachCgroup(link.CgroupOptions{
		Path:    cgroupPath,
		Attach:  attachType,
		Program: prog,
	})
	if err != nil {
		// EPERM here is the specific, recurring symptom of a cgroup
		// namespace boundary (Docker's default cgroupns=private on
		// cgroup v2 hosts, 20.10+): the capability check for attaching
		// cgroup_skb programs is scoped to the namespace it was granted
		// in, so a privileged Pod still gets "operation not permitted"
		// if its node container is itself one level removed from the
		// host's real cgroup namespace. This hits Docker-in-Docker style
		// local clusters (kind/k3d) on a cgroup v2 host; real managed
		// Kubernetes nodes (EKS/GKE/AKS) don't have this problem. See
		// the README's "Known limitation: cgroup namespaces" section for
		// the fix (default-cgroupns-mode: host in the Docker daemon).
		if errors.Is(err, syscall.EPERM) {
			return fmt.Errorf(
				"attach %s at %s: %w — this is the known cgroup-namespace limitation "+
					"(privileged: true isn't enough on kind/k3d), not a generic permissions bug; "+
					"see the README's \"Known limitation: cgroup namespaces\" section",
				progName, cgroupPath, err,
			)
		}

		return fmt.Errorf("attach %s at %s: %w", progName, cgroupPath, err)
	}

	l.links = append(l.links, lnk)
	return nil
}

// Close detaches every program and releases the underlying collection. Safe
// to call once; subsequent calls are no-ops.
func (l *Loader) Close() error {
	var firstErr error

	for _, lnk := range l.links {
		if err := lnk.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	l.links = nil

	if l.coll != nil {
		l.coll.Close()
		l.coll = nil
	}

	return firstErr
}
