/*
 * dns_full_bpf.bpf.c - DNS 协议解析 eBPF 程序
 * 捕获并解析 DNS 查询/响应报文
 *
 * 编译: clang -O2 -target bpf -c dns_full_bpf.bpf.c -o dns_full_bpf.bpf.o
 */
#include <linux/bpf.h>
#include <linux/udp.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

/* DNS 头部结构 */
struct dns_header {
    __u16 id;
    __u16 flags;
    __u16 qdcount;
    __u16 ancount;
    __u16 nscount;
    __u16 arcount;
} __attribute__((packed));

/* DNS 查询统计 */
struct dns_key {
    __u32 saddr;
    __u32 daddr;
    __u16 qtype;
};

struct dns_stats {
    __u64 query_count;
    __u64 response_count;
    __u64 nxdomain_count;
    __u64 latency_sum;
    __u64 latency_count;
};

/* DNS 查询跟踪 Map */
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, __u16);  /* DNS Transaction ID */
    __type(value, __u64); /* 查询开始时间 */
} dns_query_ts SEC(".maps");

/* DNS 统计 Map */
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 4096);
    __type(key, struct dns_key);
    __type(value, struct dns_stats);
} dns_stats_map SEC(".maps");

/* DNS 事件 Ring Buffer */
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 19);  /* 512KB */
} dns_events SEC(".maps");

#define DNS_EVENT_SIZE 128

struct dns_event {
    __u64 timestamp;
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
    __u16 txid;
    __u16 qtype;
    __u16 flags;
    __u8 is_query;
    __u8 name_len;
    char name[64];
};

/*
 * 解析 DNS 域名 (简化版)
 */
static __always_inline int parse_dns_name(const char *data, __u32 offset, __u32 max_len,
                                          char *out, __u8 *out_len) {
    __u32 pos = offset;
    __u8 len = 0;
    __u8 i = 0;
    
    #pragma unroll
    for (i = 0; i < 16 && pos < max_len - 1; i++) {
        __u8 label_len = data[pos];
        if (label_len == 0) {
            break;
        }
        /* 检查是否为指针 (前两位为11) */
        if ((label_len & 0xC0) == 0xC0) {
            break;  /* 简化处理：不处理压缩指针 */
        }
        if (label_len > 63 || pos + 1 + label_len >= max_len) {
            break;
        }
        if (len > 0 && len < 63) {
            out[len++] = '.';
        }
        __builtin_memcpy(out + len, data + pos + 1, label_len < (63 - len) ? label_len : (63 - len));
        len += label_len;
        pos += 1 + label_len;
    }
    
    *out_len = len;
    out[len] = 0;
    return 0;
}

/*
 * TC 钩子 - DNS 流量解析
 */
SEC("tc")
int dns_full_capture(struct __sk_buff *skb) {
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
    
    if (ip->protocol != IPPROTO_UDP) {
        return TC_ACT_OK;
    }
    
    struct udphdr *udp = (void *)ip + sizeof(*ip);
    if ((void *)udp + sizeof(*udp) > data_end) {
        return TC_ACT_OK;
    }
    
    /* DNS 端口: 53 */
    __u16 dport = bpf_ntohs(udp->dest);
    __u16 sport = bpf_ntohs(udp->source);
    if (dport != 53 && sport != 53) {
        return TC_ACT_OK;
    }
    
    void *payload = (void *)udp + sizeof(*udp);
    __u32 payload_len = data_end - payload;
    
    if (payload_len < sizeof(struct dns_header)) {
        return TC_ACT_OK;
    }
    
    struct dns_header *dns = payload;
    __u16 flags = bpf_ntohs(dns->flags);
    __u16 txid = bpf_ntohs(dns->id);
    __u8 is_query = (flags & 0x8000) ? 0 : 1;  /* QR位: 0=查询, 1=响应 */
    
    /* DNS 查询，记录开始时间 */
    if (is_query) {
        __u64 ts = bpf_ktime_get_ns();
        bpf_map_update_elem(&dns_query_ts, &txid, &ts, BPF_ANY);
    }
    
    /* DNS 响应，计算延迟 */
    if (!is_query) {
        __u64 *query_ts = bpf_map_lookup_elem(&dns_query_ts, &txid);
        if (query_ts) {
            __u64 latency = (bpf_ktime_get_ns() - *query_ts) / 1000;  /* us */
            bpf_map_delete_elem(&dns_query_ts, &txid);
        }
    }
    
    /* 发送 DNS 事件到用户空间 */
    struct dns_event *event = bpf_ringbuf_reserve(&dns_events, sizeof(*event), 0);
    if (event) {
        event->timestamp = bpf_ktime_get_ns();
        event->saddr = ip->saddr;
        event->daddr = ip->daddr;
        event->sport = udp->source;
        event->dport = udp->dest;
        event->txid = txid;
        event->flags = flags;
        event->is_query = is_query;
        event->qtype = 1;  /* 默认 A 记录 */
        event->name_len = 0;
        
        /* 尝试解析查询名称 */
        void *dns_data = payload + sizeof(struct dns_header);
        __u32 dns_data_len = payload_len - sizeof(struct dns_header);
        if (dns_data_len > 0) {
            parse_dns_name(dns_data, 0, dns_data_len, event->name, &event->name_len);
        }
        
        bpf_ringbuf_submit(event, 0);
    }
    
    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
