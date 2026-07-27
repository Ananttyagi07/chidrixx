package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/cilium/ebpf"
)

type FlowKey struct {
	CgroupID uint64
	Saddr    uint32
	Daddr    uint32
	Sport    uint16
	Dport    uint16
	Proto    uint8
	Pad      [3]byte
}

type FlowStat struct {
	BytesTx   uint64
	PacketsTx uint64
	BytesRx   uint64
	PacketsRx uint64
}

func ipString(v uint32) string {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return net.IP(b).String()
}

// delta computes cur-prev, guarding against LRU eviction resetting a counter
// lower than it was — a classic first-eBPF-agent bug (see LLD §4.2).
func delta(prev, cur FlowStat) FlowStat {
	if cur.BytesTx < prev.BytesTx || cur.BytesRx < prev.BytesRx {
		return cur // counter reset (evicted+reused) -> treat current as the delta
	}
	return FlowStat{
		BytesTx:   cur.BytesTx - prev.BytesTx,
		PacketsTx: cur.PacketsTx - prev.PacketsTx,
		BytesRx:   cur.BytesRx - prev.BytesRx,
		PacketsRx: cur.PacketsRx - prev.PacketsRx,
	}
}

func scrape(m *ebpf.Map, prev map[FlowKey]FlowStat) map[FlowKey]FlowStat {
	cur := make(map[FlowKey]FlowStat)
	var key FlowKey
	var perCPU []FlowStat

	iter := m.Iterate()
	for iter.Next(&key, &perCPU) {
		var sum FlowStat
		for _, s := range perCPU {
			sum.BytesTx += s.BytesTx
			sum.PacketsTx += s.PacketsTx
			sum.BytesRx += s.BytesRx
			sum.PacketsRx += s.PacketsRx
		}
		cur[key] = sum

		d := delta(prev[key], sum)
		if d.BytesTx == 0 && d.BytesRx == 0 {
			continue // nothing new this window
		}
		fmt.Printf("[Δ15s] cgroup=%d  %s:%d -> %d  proto=%d  tx=%dB(%dpkt)  rx=%dB(%dpkt)\n",
			key.CgroupID, ipString(key.Saddr), key.Sport, key.Dport, key.Proto,
			d.BytesTx, d.PacketsTx, d.BytesRx, d.PacketsRx)
	}
	if err := iter.Err(); err != nil {
		log.Printf("iterate error: %v", err)
	}
	return cur
}

func main() {
	m, err := ebpf.LoadPinnedMap("/sys/fs/bpf/chidrixx/flows", nil)
	if err != nil {
		log.Fatalf("open pinned map: %v", err)
	}
	defer m.Close()

	fmt.Println("chidrixx-reader: scraping every 15s, Ctrl+C to stop")
	prev := make(map[FlowKey]FlowStat)
	for range time.Tick(15 * time.Second) {
		prev = scrape(m, prev)
	}
}
