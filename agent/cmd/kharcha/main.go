package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"

	"github.com/cilium/ebpf"
)

// FlowKey mirrors bpf/flow_common.h's struct flow_key, byte for byte.
type FlowKey struct {
	CgroupID uint64
	Saddr    uint32
	Daddr    uint32
	Sport    uint16
	Dport    uint16
	Proto    uint8
	Pad      [3]byte
}

// FlowStat mirrors struct flow_stat.
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

func main() {
	// Open the pinned map by path — the SAME map your kernel program writes to.
	m, err := ebpf.LoadPinnedMap("/sys/fs/bpf/chidrixx/flows", nil)
	if err != nil {
		log.Fatalf("open pinned map (did you pinmaps the map too? see note below): %v", err)
	}
	defer m.Close()

	var (
		key   FlowKey
		perCPU []FlowStat // one entry per CPU core — THIS is why per-CPU changes the read
	)

	iter := m.Iterate()
	count := 0
	for iter.Next(&key, &perCPU) {
		var sum FlowStat
		for _, s := range perCPU { // sum across cores — the mandatory per-CPU step
			sum.BytesTx += s.BytesTx
			sum.PacketsTx += s.PacketsTx
			sum.BytesRx += s.BytesRx
			sum.PacketsRx += s.PacketsRx
		}
		if sum.BytesTx == 0 && sum.BytesRx == 0 {
			continue // cold entry
		}
		fmt.Printf("cgroup=%d  %s:%d -> %d  proto=%d  tx=%dB(%dpkt)  rx=%dB(%dpkt)\n",
			key.CgroupID, ipString(key.Saddr), key.Sport, key.Dport, key.Proto,
			sum.BytesTx, sum.PacketsTx, sum.BytesRx, sum.PacketsRx)
		count++
	}
	if err := iter.Err(); err != nil {
		log.Fatalf("iterate: %v", err)
	}
	fmt.Printf("\n%d active flows\n", count)
}
