/* SPDX-License-Identifier: GPL-2.0 */
/* network_flow.bpf.c - TC ingress/egress eBPF program using ringbuf */

#include "vmlinux.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "bpf_core_read.h"
#include "bpf_endian.h"
#include "common.h"

char __license[] SEC("license") = "GPL";

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} rb SEC(".maps");

static __always_inline int handle_packet(struct __sk_buff *skb, __u8 direction) {
    void *data_end = (void *)(long)skb->data_end;
    void *data = (void *)(long)skb->data;

    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return TC_ACT_OK;

    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return TC_ACT_OK;

    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return TC_ACT_OK;

    struct event *e = bpf_ringbuf_reserve(&rb, sizeof(struct event), 0);
    if (!e)
        return TC_ACT_OK;

    e->timestamp_ns = bpf_ktime_get_ns();
    e->type = EVENT_TYPE_FLOW;
    e->pid = 0;
    e->ppid = 0;
    e->src_ip = BPF_CORE_READ(ip, saddr);
    e->dst_ip = BPF_CORE_READ(ip, daddr);
    e->src_port = 0;
    e->dst_port = 0;
    e->protocol = BPF_CORE_READ(ip, protocol);
    e->bytes = skb->len;
    e->packets = 1;
    e->latency_ns = 0;
    e->count = 0;
    __builtin_memset(e->comm, 0, TASK_COMM_LEN);
    __builtin_memset(e->data, 0, DATA_LEN);

    if (ip->protocol == IPPROTO_TCP) {
        struct tcphdr *tcp = (void *)(ip + 1);
        if ((void *)(tcp + 1) <= data_end) {
            e->src_port = bpf_ntohs(BPF_CORE_READ(tcp, source));
            e->dst_port = bpf_ntohs(BPF_CORE_READ(tcp, dest));
        }
    } else if (ip->protocol == IPPROTO_UDP) {
        struct udphdr *udp = (void *)(ip + 1);
        if ((void *)(udp + 1) <= data_end) {
            e->src_port = bpf_ntohs(BPF_CORE_READ(udp, source));
            e->dst_port = bpf_ntohs(BPF_CORE_READ(udp, dest));
        }
    }

    bpf_ringbuf_submit(e, 0);
    return TC_ACT_OK;
}

SEC("tc")
int tc_ingress(struct __sk_buff *skb) {
    return handle_packet(skb, 0);
}

SEC("tc")
int tc_egress(struct __sk_buff *skb) {
    return handle_packet(skb, 1);
}
