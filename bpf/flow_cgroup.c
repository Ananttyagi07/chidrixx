// SPDX-License-Identifier: GPL-2.0
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

char LICENSE[] SEC("license") = "GPL";

// The flow identity: who (cgroup) + where (5-tuple). IPv4 only for this inch.
struct flow_key {
    __u64 cgroup_id;
    __u32 saddr;   // local  IP
    __u32 daddr;   // remote IP
    __u16 sport;
    __u16 dport;
    __u8  proto;
    __u8  _pad[3]; // keep key bytes deterministic
};

struct flow_stat { __u64 bytes_tx; __u64 packets_tx; };

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 4096);
    __type(key, struct flow_key);
    __type(value, struct flow_stat);
} flows SEC(".maps");

SEC("cgroup_skb/egress")
int kharcha_egress(struct __sk_buff *skb) {
    struct flow_key key;
    __builtin_memset(&key, 0, sizeof(key));   // no uninitialized key bytes
    key.cgroup_id = bpf_skb_cgroup_id(skb);

    // cgroup_skb data starts at L3 (IP header). Read the IPv4 header.
    struct iphdr iph;
    if (bpf_skb_load_bytes(skb, 0, &iph, sizeof(iph)) < 0)
        return 1;                              // can't read -> allow, skip
    if (iph.version != 4)
        return 1;                              // IPv6 later; skip for now

    key.saddr = iph.saddr;
    key.daddr = iph.daddr;
    key.proto = iph.protocol;

    __u32 l4_off = (iph.ihl & 0x0F) * 4;       // IP header length in bytes

    // ports live in the first 4 bytes of TCP/UDP headers
    if (iph.protocol == IPPROTO_TCP || iph.protocol == IPPROTO_UDP) {
        __be16 ports[2];
        if (bpf_skb_load_bytes(skb, l4_off, ports, sizeof(ports)) == 0) {
            key.sport = bpf_ntohs(ports[0]);
            key.dport = bpf_ntohs(ports[1]);
        }
    }

    struct flow_stat *st = bpf_map_lookup_elem(&flows, &key);
    if (st) {
        st->bytes_tx   += skb->len;
        st->packets_tx += 1;
    } else {
        struct flow_stat init = { .bytes_tx = skb->len, .packets_tx = 1 };
        bpf_map_update_elem(&flows, &key, &init, BPF_ANY);
    }
    return 1;
}
