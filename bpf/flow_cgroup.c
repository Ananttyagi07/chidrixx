// SPDX-License-Identifier: GPL-2.0
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>

char LICENSE[] SEC("license") = "GPL";

// The value we accumulate per cgroup. First real measurement.
struct flow_stat {
    __u64 bytes_tx;
    __u64 packets_tx;
};

// A hash map: key = cgroup_id (which container), value = its byte/packet totals.
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, __u64);
    __type(value, struct flow_stat);
} flows SEC(".maps");

SEC("cgroup_skb/egress")
int kharcha_egress(struct __sk_buff *skb) {
    __u64 cgid = bpf_skb_cgroup_id(skb);          // which container sent this?

    struct flow_stat *st = bpf_map_lookup_elem(&flows, &cgid);
    if (st) {
        st->bytes_tx   += skb->len;               // add this packet's bytes
        st->packets_tx += 1;
    } else {
        struct flow_stat init = { .bytes_tx = skb->len, .packets_tx = 1 };
        bpf_map_update_elem(&flows, &cgid, &init, BPF_ANY);
    }
    return 1;                                       // still always allow
}
