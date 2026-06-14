#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/pkt_cls.h>
#include <bpf/bpf_helpers.h>
#include "flow_tracker.h"

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_FLOWS);
    __type(key, struct flow_key);
    __type(value, struct flow_value);
} flows SEC(".maps");

static __always_inline int track_flow(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    if (data + sizeof(struct ethhdr) > data_end) return TC_ACT_OK;

    struct ethhdr *eth = (struct ethhdr *)data;
    if (eth->h_proto != __constant_htons(0x0800)) return TC_ACT_OK;

    struct iphdr *ip = (struct iphdr *)(data + sizeof(struct ethhdr));
    if ((void *)(ip + 1) > data_end) return TC_ACT_OK;

    struct flow_key key = {};
    key.src_ip = ip->saddr;
    key.dst_ip = ip->daddr;
    key.protocol = ip->protocol;

    if (ip->protocol == 6 && (void *)(ip + 1) + 20 <= data_end) {
        struct tcphdr *tcp = (void *)(ip + 1);
        key.src_port = tcp->source;
        key.dst_port = tcp->dest;
    } else if (ip->protocol == 17 && (void *)(ip + 1) + 8 <= data_end) {
        struct udphdr *udp = (void *)(ip + 1);
        key.src_port = udp->source;
        key.dst_port = udp->dest;
    }

    __u64 now = bpf_ktime_get_ns() / 1000000000;
    struct flow_value *val = bpf_map_lookup_elem(&flows, &key);
    if (val) {
        val->bytes += skb->len;
        val->packets += 1;
        val->last_seen = now;
    } else {
        struct flow_value new_val = {};
        new_val.bytes = skb->len;
        new_val.packets = 1;
        new_val.first_seen = now;
        new_val.last_seen = now;
        bpf_map_update_elem(&flows, &key, &new_val, BPF_ANY);
    }
    return TC_ACT_OK;
}

SEC("tc")
int tc_ingress(struct __sk_buff *skb) { return track_flow(skb); }

SEC("tc")
int tc_egress(struct __sk_buff *skb) { return track_flow(skb); }

char _license[] SEC("license") = "GPL";
