package main

import (
	"fmt"
	"net"
	"sort"
)

type Finding struct {
	CgroupID uint64
	Workload string
	Class    PathClass
	RemoteIP string
	BytesTx  uint64
	BytesRx  uint64
	CostINR  float64
}

// Aggregate stores cumulative traffic/cost findings since the agent started.
type Aggregate struct {
	byKey map[string]*Finding
}

func NewAggregate() *Aggregate {
	return &Aggregate{
		byKey: make(map[string]*Finding),
	}
}

// Add adds one traffic delta to the cumulative report.
//
// We aggregate by:
//   - cgroup/workload
//   - traffic class
//   - remote IP
//
// This allows Chidrixx to answer:
//
//	"Which workload is talking to which destination, and how much
//	estimated network cost is associated with it?"
func (a *Aggregate) Add(
	cgroupID uint64,
	workload string,
	remote net.IP,
	tx uint64,
	rx uint64,
) {
	class := Classify(remote)

	ip := "?"
	if remote != nil {
		ip = remote.String()
	}

	key := fmt.Sprintf(
		"%d|%s|%s",
		cgroupID,
		class,
		ip,
	)

	f := a.byKey[key]

	if f == nil {
		f = &Finding{
			CgroupID: cgroupID,
			Workload: workload,
			Class:    class,
			RemoteIP: ip,
		}

		a.byKey[key] = f
	}

	f.BytesTx += tx
	f.BytesRx += rx

	// Illustrative pricing for the current MVP.
	// Real cloud pricing logic will be added later.
	f.CostINR += CostINR(class, tx+rx)
}

// PrintTop prints the highest-cost traffic findings seen since startup.
func (a *Aggregate) PrintTop(n int) {
	var all []*Finding

	for _, f := range a.byKey {
		all = append(all, f)
	}

	// Highest estimated cost first.
	sort.Slice(all, func(i, j int) bool {
		return all[i].CostINR > all[j].CostINR
	})

	if n > len(all) {
		n = len(all)
	}

	fmt.Println()
	fmt.Println("=== Chidrixx Top Waste Report (illustrative ₹/GB) ===")

	fmt.Printf(
		"%-3s %-24s %-10s %-18s %12s %12s %10s\n",
		"#",
		"workload",
		"class",
		"remote",
		"tx bytes",
		"rx bytes",
		"₹ cost",
	)

	for i, f := range all[:n] {
		fmt.Printf(
			"%-3d %-24.24s %-10s %-18s %12d %12d %10.4f\n",
			i+1,
			f.Workload,
			f.Class,
			f.RemoteIP,
			f.BytesTx,
			f.BytesRx,
			f.CostINR,
		)
	}
}
