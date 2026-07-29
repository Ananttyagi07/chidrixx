// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
)

const ethHeaderLen = 14

// buildIPv4TCP returns a minimal IPv4/TCP packet starting at the IP header —
// cgroup_skb sees no ethernet header, so this matches what kharcha_egress
// actually parses in production.
func buildIPv4TCP(srcPort, dstPort uint16, payloadLen int) []byte {
	const ipHeaderLen = 20
	const tcpHeaderLen = 20

	total := ipHeaderLen + tcpHeaderLen + payloadLen
	pkt := make([]byte, total)

	// IPv4 header.
	pkt[0] = 0x45 // version 4, IHL 5 (20 bytes, no options)
	binary.BigEndian.PutUint16(pkt[2:4], uint16(total))
	pkt[9] = 6 // protocol: TCP
	copy(pkt[12:16], []byte{10, 0, 0, 1})
	copy(pkt[16:20], []byte{10, 0, 1, 1})

	// TCP header (no options).
	tcp := pkt[ipHeaderLen:]
	binary.BigEndian.PutUint16(tcp[0:2], srcPort)
	binary.BigEndian.PutUint16(tcp[2:4], dstPort)
	tcp[12] = 0x50 // data offset: 5 words

	return pkt
}

// buildTestFrame prepends a throwaway 14-byte Ethernet header to an IPv4/TCP
// packet built by buildIPv4TCP. BPF_PROG_TEST_RUN's skb harness always
// assumes an L2 frame and strips ETH_HLEN bytes (skb_pull + reset network
// header) before invoking a non-L2 program type like cgroup_skb — see
// cilium/ebpf's Program.Test doc: "the kernel expects at least 14 bytes
// input for an ethernet header for XDP and SKB programs." Production
// traffic never carries this header (cgroup_skb genuinely starts at L3,
// confirmed by real traffic), but the test harness needs the padding
// anyway; it's gone from skb->len before kharcha_egress ever runs.
func buildTestFrame(srcPort, dstPort uint16, payloadLen int) (frame []byte, wireLen int) {
	l3 := buildIPv4TCP(srcPort, dstPort, payloadLen)

	frame = make([]byte, ethHeaderLen+len(l3))
	copy(frame[ethHeaderLen:], l3)

	return frame, len(l3)
}

// TestEgressByteAccounting is the "accuracy gate" from the build manual's
// Step 14.1, at unit-test scale: run a synthetic packet of known length
// through kharcha_egress via BPF_PROG_TEST_RUN and assert the program never
// drops it and that bytes_tx lands exactly on the packet length. It doesn't
// replace the real iperf3 validation against a live cluster (that needs a
// running node and two endpoints), but it catches the classic landmines —
// dropped packets, missing direction handling, off-by-N counting — without
// any network at all.
//
// Loading a BPF program requires CAP_BPF (effectively root); the test skips
// rather than fails when that's unavailable, since that's an environment
// limitation, not a program bug.
func TestEgressByteAccounting(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root/CAP_BPF to load a BPF program; run as root or in CI")
	}

	if err := rlimit.RemoveMemlock(); err != nil {
		t.Fatalf("remove memlock rlimit: %v", err)
	}

	spec, err := ebpf.LoadCollectionSpec("../../../bpf/flow_cgroup.o")
	if err != nil {
		t.Fatalf("load collection spec: %v", err)
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		t.Fatalf("load bpf collection: %v", err)
	}
	defer coll.Close()

	prog := coll.Programs["kharcha_egress"]
	if prog == nil {
		t.Fatal("kharcha_egress program not found in object")
	}

	pkt, wireLen := buildTestFrame(34567, 443, 100)

	ret, _, err := prog.Test(pkt)
	if err != nil {
		t.Fatalf("run kharcha_egress: %v", err)
	}
	if ret != 1 {
		t.Fatalf("want SK_PASS (1), got %d — the program must never drop a packet", ret)
	}

	m := coll.Maps["flows"]
	if m == nil {
		t.Fatal("flows map not found in object")
	}

	var (
		key    FlowKey
		perCPU []FlowStat
		found  bool
		total  uint64
	)

	iter := m.Iterate()
	for iter.Next(&key, &perCPU) {
		found = true
		for _, s := range perCPU {
			total += s.BytesTx
		}
	}
	if err := iter.Err(); err != nil {
		t.Fatalf("iterate flows map: %v", err)
	}

	if !found {
		t.Fatal("expected one flow entry after a single egress packet, found none")
	}
	if total != uint64(wireLen) {
		t.Fatalf("bytes_tx = %d, want %d (length of the IP+TCP packet, excluding synthetic test-harness L2 padding)", total, wireLen)
	}
}
