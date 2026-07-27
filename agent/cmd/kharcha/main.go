package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/cilium/ebpf"
)

// FlowKey must exactly match the key layout used by the eBPF flows map.
type FlowKey struct {
	CgroupID uint64
	Saddr    uint32
	Daddr    uint32
	Sport    uint16
	Dport    uint16
	Proto    uint8
	Pad      [3]byte
}

// FlowStat must exactly match the value layout used by the eBPF flows map.
type FlowStat struct {
	BytesTx   uint64
	PacketsTx uint64
	BytesRx   uint64
	PacketsRx uint64
}

// ipString converts the IPv4 uint32 stored by eBPF
// into a human-readable IPv4 address.
func ipString(v uint32) string {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return net.IP(b).String()
}

// delta calculates traffic generated since the previous scrape.
//
// If counters become smaller than the previous values, assume the
// LRU map entry was evicted/reset and treat the current counters
// as the new delta.
func delta(prev, cur FlowStat) FlowStat {
	if cur.BytesTx < prev.BytesTx || cur.BytesRx < prev.BytesRx {
		return cur
	}

	return FlowStat{
		BytesTx:   cur.BytesTx - prev.BytesTx,
		PacketsTx: cur.PacketsTx - prev.PacketsTx,
		BytesRx:   cur.BytesRx - prev.BytesRx,
		PacketsRx: cur.PacketsRx - prev.PacketsRx,
	}
}

// scrape reads the current per-CPU eBPF flow map,
// calculates traffic deltas, resolves the originating workload,
// and feeds the result into the cumulative aggregator.
func scrape(
	m *ebpf.Map,
	prev map[FlowKey]FlowStat,
	agg *Aggregate,
	resolver *WorkloadResolver,
) map[FlowKey]FlowStat {

	cur := make(map[FlowKey]FlowStat)

	var key FlowKey
	var perCPU []FlowStat

	iter := m.Iterate()

	for iter.Next(&key, &perCPU) {
		var sum FlowStat

		// The eBPF map is PERCPU, so combine the counters
		// from every CPU into one FlowStat.
		for _, s := range perCPU {
			sum.BytesTx += s.BytesTx
			sum.PacketsTx += s.PacketsTx
			sum.BytesRx += s.BytesRx
			sum.PacketsRx += s.PacketsRx
		}

		cur[key] = sum

		// Calculate only the traffic added since the last scrape.
		d := delta(prev[key], sum)

		if d.BytesTx == 0 && d.BytesRx == 0 {
			continue
		}

		// Daddr currently represents the normalized remote endpoint
		// produced by our eBPF flow accounting logic.
		remote := net.ParseIP(ipString(key.Daddr))

		// WorkloadResolver itself lives in workload.go.
		workload := resolver.Resolve(key.CgroupID)

		// Add this window's traffic to the cumulative report.
		agg.Add(
			key.CgroupID,
			workload,
			remote,
			d.BytesTx,
			d.BytesRx,
		)
	}

	if err := iter.Err(); err != nil {
		log.Printf("iterate error: %v", err)
	}

	return cur
}

func main() {
	const mapPath = "/sys/fs/bpf/chidrixx/flows"

	// Open the flow map pinned by the eBPF loader.
	m, err := ebpf.LoadPinnedMap(mapPath, nil)
	if err != nil {
		log.Fatalf("open pinned map %s: %v", mapPath, err)
	}
	defer m.Close()

	fmt.Println("chidrixx-reader: scraping every 15s, Ctrl+C to stop")

	// Previous map snapshot used to calculate deltas.
	prev := make(map[FlowKey]FlowStat)

	// Cumulative network cost aggregator.
	agg := NewAggregate()

	// Resolves cgroup IDs into human-readable workload names.
	// Implementation is in workload.go.
	resolver := NewWorkloadResolver()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		prev = scrape(
			m,
			prev,
			agg,
			resolver,
		)

		// Print the ten highest estimated network-cost findings.
		agg.PrintTop(10)
	}
}
