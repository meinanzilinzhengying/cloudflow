/*
 * tc_bpf.bpf.c - TC 流量控制 eBPF 程序
 * 用于捕获网络接口的 ingress/egress 流量
 *
 * 编译: clang -O2 -target bpf -c tc_bpf.bpf.c -o tc_bpf.bpf.o
 */
#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/pkt_cls.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

/* 网络流统计 Map */
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, __u32[4]);  // saddr, daddr, sport, dport
    __type(value, __u64[2]); // bytes, packets
} network_map SEC(".maps");

/*
 * TC ingress/egress 处理程序
 * 统计每个网络流的字节数和数据包数
 */
SEC("tc")
int tc_flow_stats(struct __sk_buff *skb) {
    void *data = (void *)(long)skb->data;
    void *data_end = (void *)(long)skb->data_end;
    
    struct ethhdr *eth = data;
    if (data + sizeof(*eth) > data_end) {
        return TC_ACT_OK;
    }
    
    if (eth->h_proto != bpf_htons(ETH_P_IP)) {
        return TC_ACT_OK;
    }
    
    struct iphdr *ip = data + sizeof(*eth);
    if ((void *)ip + sizeof(*ip) > data_end) {
        return TC_ACT_OK;
    }
    
    __u32 saddr = ip->saddr;
    __u32 daddr = ip->daddr;
    __u16 sport = 0;
    __u16 dport = 0;
    
    if (ip->protocol == IPPROTO_TCP) {
        struct tcphdr *tcp = (void *)ip + sizeof(*ip);
        if ((void *)tcp + sizeof(*tcp) > data_end) {
            return TC_ACT_OK;
        }
        sport = tcp->source;
        dport = tcp->dest;
    } else if (ip->protocol == IPPROTO_UDP) {
        struct udphdr *udp = (void *)ip + sizeof(*ip);
        if ((void *)udp + sizeof(*udp) > data_end) {
            return TC_ACT_OK;
        }
        sport = udp->source;
        dport = udp->dest;
    }
    
    __u32 key[4] = {saddr, daddr, sport, dport};
    __u64 *value = bpf_map_lookup_elem(&network_map, key);
    
    if (value) {
        value[0] += skb->len;
        value[1] += 1;
    } else {
        __u64 init_value[2] = {skb->len, 1};
        bpf_map_update_elem(&network_map, key, init_value, BPF_ANY);
    }
    
    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
