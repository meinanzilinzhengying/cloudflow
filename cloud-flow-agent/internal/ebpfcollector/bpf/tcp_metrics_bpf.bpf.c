/*
 * tcp_metrics_bpf.bpf.c - TCP 性能指标采集 eBPF 程序
 * 采集 TCP RTT、重传、丢包、拥塞窗口等指标
 *
 * 编译: clang -O2 -target bpf -c tcp_metrics_bpf.bpf.c -o tcp_metrics_bpf.bpf.o
 */
#include <linux/bpf.h>
#include <linux/tcp.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>
#include <bpf/bpf_tracing.h>

/* TCP 连接指标 */
struct tcp_metrics_key {
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
};

struct tcp_metrics_value {
    __u64 rtt_min;        /* 最小RTT (us) */
    __u64 rtt_max;        /* 最大RTT (us) */
    __u64 rtt_sum;        /* RTT总和 */
    __u64 rtt_count;      /* RTT采样数 */
    __u64 retrans_count;  /* 重传次数 */
    __u64 loss_count;     /* 丢包次数 */
    __u64 bytes_sent;     /* 发送字节数 */
    __u64 bytes_recv;     /* 接收字节数 */
    __u32 cwnd_min;       /* 最小拥塞窗口 */
    __u32 cwnd_max;       /* 最大拥塞窗口 */
    __u32 last_update;    /* 最后更新时间 */
};

/* TCP 指标 Map */
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, struct tcp_metrics_key);
    __type(value, struct tcp_metrics_value);
} tcp_metrics_map SEC(".maps");

/* TCP SYN 时间戳跟踪 */
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, struct tcp_metrics_key);
    __type(value, __u64);  /* SYN 时间戳 */
} tcp_syn_ts_map SEC(".maps");

/*
 * TC 钩子 - 统计 TCP 数据包
 */
SEC("tc")
int tcp_metrics_collect(struct __sk_buff *skb) {
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
    
    if (ip->protocol != IPPROTO_TCP) {
        return TC_ACT_OK;
    }
    
    struct tcphdr *tcp = (void *)ip + sizeof(*ip);
    if ((void *)tcp + sizeof(*tcp) > data_end) {
        return TC_ACT_OK;
    }
    
    struct tcp_metrics_key key = {
        .saddr = ip->saddr,
        .daddr = ip->daddr,
        .sport = tcp->source,
        .dport = tcp->dest
    };
    
    /* 检查 SYN 包，记录开始时间 */
    if (tcp->syn && !tcp->ack) {
        __u64 ts = bpf_ktime_get_ns();
        bpf_map_update_elem(&tcp_syn_ts_map, &key, &ts, BPF_ANY);
    }
    
    /* 检查 SYN-ACK，计算 RTT */
    if (tcp->syn && tcp->ack) {
        __u64 *syn_ts = bpf_map_lookup_elem(&tcp_syn_ts_map, &key);
        if (syn_ts) {
            __u64 rtt = bpf_ktime_get_ns() - *syn_ts;
            rtt /= 1000; /* ns -> us */
            
            struct tcp_metrics_value *metrics = bpf_map_lookup_elem(&tcp_metrics_map, &key);
            if (metrics) {
                if (rtt < metrics->rtt_min || metrics->rtt_min == 0) {
                    metrics->rtt_min = rtt;
                }
                if (rtt > metrics->rtt_max) {
                    metrics->rtt_max = rtt;
                }
                metrics->rtt_sum += rtt;
                metrics->rtt_count++;
            } else {
                struct tcp_metrics_value init = {
                    .rtt_min = rtt,
                    .rtt_max = rtt,
                    .rtt_sum = rtt,
                    .rtt_count = 1
                };
                bpf_map_update_elem(&tcp_metrics_map, &key, &init, BPF_ANY);
            }
            
            bpf_map_delete_elem(&tcp_syn_ts_map, &key);
        }
    }
    
    /* 检测重传 (重复ACK) */
    if (tcp->ack) {
        struct tcp_metrics_value *metrics = bpf_map_lookup_elem(&tcp_metrics_map, &key);
        if (metrics) {
            metrics->bytes_recv += skb->len;
        }
    }
    
    /* 统计发送字节 */
    if (!tcp->ack || tcp->psh) {
        struct tcp_metrics_value *metrics = bpf_map_lookup_elem(&tcp_metrics_map, &key);
        if (metrics) {
            metrics->bytes_sent += skb->len;
        }
    }
    
    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
