/*
 * http_full_bpf.bpf.c - HTTP 完整报文捕获 eBPF 程序
 * 捕获 HTTP 请求/响应完整内容，用于深度分析
 *
 * 编译: clang -O2 -target bpf -c http_full_bpf.bpf.c -o http_full_bpf.bpf.o
 */
#include <linux/bpf.h>
#include <linux/tcp.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

/* 最大捕获 payload 大小 */
#define MAX_PAYLOAD_SIZE 256

/* HTTP 完整报文键 */
struct http_full_key {
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
    __u32 seq;
};

/* HTTP 完整报文值 */
struct http_full_value {
    __u64 timestamp;
    __u32 payload_len;
    __u8 payload[MAX_PAYLOAD_SIZE];
    __u8 direction;  /* 0=ingress, 1=egress */
    __u8 reserved[3];
};

/* HTTP 完整报文 Map (perf event 方式) */
struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
    __uint(key_size, sizeof(__u32));
    __uint(value_size, sizeof(__u32));
    __uint(max_entries, 128);
} http_events SEC(".maps");

/* HTTP 报文 Ring Buffer */
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 20);  /* 1MB */
} http_ringbuf SEC(".maps");

/*
 * 检测是否为 HTTP 流量
 */
static __always_inline int is_http_traffic(const char *data, __u32 len) {
    if (len < 4) return 0;
    
    /* HTTP 请求方法 */
    if (__builtin_memcmp(data, "GET ", 4) == 0) return 1;
    if (__builtin_memcmp(data, "POST", 4) == 0) return 1;
    if (__builtin_memcmp(data, "PUT ", 4) == 0) return 1;
    if (__builtin_memcmp(data, "DELE", 4) == 0) return 1;
    if (__builtin_memcmp(data, "HEAD", 4) == 0) return 1;
    if (__builtin_memcmp(data, "OPTI", 4) == 0) return 1;
    if (__builtin_memcmp(data, "PATC", 4) == 0) return 1;
    if (__builtin_memcmp(data, "CONN", 4) == 0) return 1;
    
    /* HTTP 响应 */
    if (__builtin_memcmp(data, "HTTP/", 5) == 0) return 1;
    
    return 0;
}

/*
 * TC 钩子 - HTTP 完整报文捕获
 */
SEC("tc")
int http_full_capture(struct __sk_buff *skb) {
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
    
    /* HTTP 端口过滤 */
    __u16 dport = bpf_ntohs(tcp->dest);
    __u16 sport = bpf_ntohs(tcp->source);
    if (dport != 80 && dport != 8080 && dport != 3000 &&
        sport != 80 && sport != 8080 && sport != 3000) {
        return TC_ACT_OK;
    }
    
    void *payload = (void *)tcp + sizeof(*tcp);
    __u32 payload_len = data_end - payload;
    
    if (payload_len < 8) {
        return TC_ACT_OK;
    }
    
    /* 检测 HTTP 流量 */
    if (!is_http_traffic(payload, payload_len)) {
        return TC_ACT_OK;
    }
    
    /* 限制捕获大小 */
    __u32 capture_len = payload_len;
    if (capture_len > MAX_PAYLOAD_SIZE) {
        capture_len = MAX_PAYLOAD_SIZE;
    }
    
    /* 通过 perf event 发送到用户空间 */
    struct http_full_value *event = bpf_ringbuf_reserve(&http_ringbuf, sizeof(*event), 0);
    if (event) {
        event->timestamp = bpf_ktime_get_ns();
        event->payload_len = capture_len;
        event->direction = (dport == 80 || dport == 8080 || dport == 3000) ? 1 : 0;
        
        __builtin_memcpy(&event->payload, payload, capture_len);
        
        bpf_ringbuf_submit(event, 0);
    }
    
    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
