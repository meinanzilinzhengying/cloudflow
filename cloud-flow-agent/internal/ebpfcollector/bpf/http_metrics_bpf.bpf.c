/*
 * http_metrics_bpf.bpf.c - HTTP 指标采集 eBPF 程序
 * 统计 HTTP 请求/响应延迟、状态码、QPS 等指标
 *
 * 编译: clang -O2 -target bpf -c http_metrics_bpf.bpf.c -o http_metrics_bpf.bpf.o
 */
#include <linux/bpf.h>
#include <linux/tcp.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

/* HTTP 请求键 */
struct http_key {
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
    __u32 stream_id;  /* TCP 流标识 */
};

/* HTTP 指标值 */
struct http_metrics {
    __u64 request_start;    /* 请求开始时间 */
    __u64 latency;          /* 请求延迟 (us) */
    __u16 status_code;      /* HTTP 状态码 */
    __u8 method;            /* HTTP 方法: 1=GET,2=POST,3=PUT,4=DELETE,5=OTHER */
    __u8 is_request;        /* 1=请求, 0=响应 */
    __u32 bytes;            /* 数据字节数 */
};

/* HTTP 请求跟踪 Map */
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, struct http_key);
    __type(value, struct http_metrics);
} http_request_map SEC(".maps");

/* HTTP 统计汇总 Map */
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 4096);
    __type(key, __u16);     /* 状态码 / 端口 */
    __type(value, __u64);   /* 计数 */
} http_status_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 16);
    __type(key, __u8);      /* HTTP 方法 */
    __type(value, __u64);   /* 计数 */
} http_method_map SEC(".maps");

/*
 * 检测 HTTP 请求方法
 */
static __always_inline __u8 detect_http_method(const char *data, __u32 len) {
    if (len < 4) return 0;
    
    if (__builtin_memcmp(data, "GET ", 4) == 0) return 1;
    if (__builtin_memcmp(data, "POST", 4) == 0) return 2;
    if (__builtin_memcmp(data, "PUT ", 4) == 0) return 3;
    if (__builtin_memcmp(data, "DELE", 4) == 0) return 4;
    if (__builtin_memcmp(data, "HEAD", 4) == 0) return 5;
    if (__builtin_memcmp(data, "OPTI", 4) == 0) return 5;
    if (__builtin_memcmp(data, "PATC", 4) == 0) return 5;
    
    return 5; /* OTHER */
}

/*
 * 检测 HTTP 响应状态码
 */
static __always_inline __u16 detect_http_status(const char *data, __u32 len) {
    if (len < 12) return 0;
    
    /* HTTP/1.x 200 OK */
    if (__builtin_memcmp(data, "HTTP/", 5) == 0) {
        /* 解析状态码: HTTP/1.1 XXX */
        if (data[9] >= '0' && data[9] <= '9' &&
            data[10] >= '0' && data[10] <= '9' &&
            data[11] >= '0' && data[11] <= '9') {
            return (data[9] - '0') * 100 + 
                   (data[10] - '0') * 10 + 
                   (data[11] - '0');
        }
    }
    return 0;
}

/*
 * TC 钩子 - HTTP 流量分析
 */
SEC("tc")
int http_metrics_collect(struct __sk_buff *skb) {
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
    
    /* 只关注 HTTP 常用端口 */
    __u16 dport = bpf_ntohs(tcp->dest);
    __u16 sport = bpf_ntohs(tcp->source);
    if (dport != 80 && dport != 8080 && dport != 3000 &&
        sport != 80 && sport != 8080 && sport != 3000) {
        return TC_ACT_OK;
    }
    
    void *payload = (void *)tcp + sizeof(*tcp);
    __u32 payload_len = data_end - payload;
    
    if (payload_len < 16 || payload_len > 1500) {
        return TC_ACT_OK;
    }
    
    struct http_key key = {
        .saddr = ip->saddr,
        .daddr = ip->daddr,
        .sport = tcp->source,
        .dport = tcp->dest,
        .stream_id = tcp->seq
    };
    
    /* 检测 HTTP 请求 */
    __u8 method = detect_http_method(payload, payload_len);
    if (method > 0) {
        struct http_metrics metrics = {
            .request_start = bpf_ktime_get_ns(),
            .method = method,
            .is_request = 1,
            .bytes = payload_len
        };
        bpf_map_update_elem(&http_request_map, &key, &metrics, BPF_ANY);
        
        /* 更新方法计数 */
        __u64 *count = bpf_map_lookup_elem(&http_method_map, &method);
        if (count) {
            (*count)++;
        } else {
            __u64 init = 1;
            bpf_map_update_elem(&http_method_map, &method, &init, BPF_ANY);
        }
    }
    
    /* 检测 HTTP 响应 */
    __u16 status = detect_http_status(payload, payload_len);
    if (status > 0) {
        /* 查找对应的请求计算延迟 */
        struct http_metrics *req = bpf_map_lookup_elem(&http_request_map, &key);
        if (req) {
            req->latency = (bpf_ktime_get_ns() - req->request_start) / 1000;
            req->status_code = status;
        }
        
        /* 更新状态码计数 */
        __u64 *count = bpf_map_lookup_elem(&http_status_map, &status);
        if (count) {
            (*count)++;
        } else {
            __u64 init = 1;
            bpf_map_update_elem(&http_status_map, &status, &init, BPF_ANY);
        }
    }
    
    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
