// SPDX-License-Identifier: GPL-2.0
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

char LICENSE[] SEC("license") = "GPL";

struct flow_key {
    __u64 cgroup_id;
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
    __u8  proto;
    __u8  _pad[3];
};

struct flow_stat {
    __u64 bytes_tx; __u64 packets_tx;
    __u64 bytes_rx; __u64 packets_rx;
};

struct {
    __uint(type, BPF_MAP_TYPE_LRU_PERCPU_HASH);
    // 4096 silently LRU-evicted under real load: a 20x500-connection
    // synthetic flow test (test/load/) plateaued at ~3.6k tracked flows
    // instead of the ~9.7k actually-open connections. Sized up so NFR-1's
    // 10k-concurrent-flow bar doesn't get quietly discarded by the map
    // itself before the agent even sees it.
    __uint(max_entries, 16384);
    __type(key, struct flow_key);
    __type(value, struct flow_stat);
} flows SEC(".maps");

static __always_inline int build_key(struct __sk_buff *skb, struct flow_key *key, int is_egress) {
    __builtin_memset(key, 0, sizeof(*key));
    key->cgroup_id = bpf_skb_cgroup_id(skb);

    struct iphdr iph;
    if (bpf_skb_load_bytes(skb, 0, &iph, sizeof(iph)) < 0) return -1;
    if (iph.version != 4) return -1;

    __u16 sp = 0, dp = 0;
    __u32 l4_off = (iph.ihl & 0x0F) * 4;
    if (iph.protocol == IPPROTO_TCP || iph.protocol == IPPROTO_UDP) {
        __be16 ports[2];
        if (bpf_skb_load_bytes(skb, l4_off, ports, sizeof(ports)) == 0) {
            sp = bpf_ntohs(ports[0]);
            dp = bpf_ntohs(ports[1]);
        }
    }
    key->proto = iph.protocol;

    if (is_egress) {
        key->saddr = iph.saddr; key->daddr = iph.daddr;
        key->sport = sp;        key->dport = dp;
    } else {
        key->saddr = iph.daddr; key->daddr = iph.saddr;
        key->sport = dp;        key->dport = sp;
    }
    return 0;
}

SEC("cgroup_skb/egress")
int kharcha_egress(struct __sk_buff *skb) {
    struct flow_key key;
    if (build_key(skb, &key, 1) < 0) return 1;
    struct flow_stat *st = bpf_map_lookup_elem(&flows, &key);
    if (st) { st->bytes_tx += skb->len; st->packets_tx += 1; }
    else { struct flow_stat init = { .bytes_tx = skb->len, .packets_tx = 1 };
           bpf_map_update_elem(&flows, &key, &init, BPF_ANY); }
    return 1;
}

SEC("cgroup_skb/ingress")
int kharcha_ingress(struct __sk_buff *skb) {
    struct flow_key key;
    if (build_key(skb, &key, 0) < 0) return 1;
    struct flow_stat *st = bpf_map_lookup_elem(&flows, &key);
    if (st) { st->bytes_rx += skb->len; st->packets_rx += 1; }
    else { struct flow_stat init = { .bytes_rx = skb->len, .packets_rx = 1 };
           bpf_map_update_elem(&flows, &key, &init, BPF_ANY); }
    return 1;
}
