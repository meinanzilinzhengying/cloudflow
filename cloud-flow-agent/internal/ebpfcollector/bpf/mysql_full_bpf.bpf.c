/*
 * mysql_full_bpf.bpf.c - MySQL 协议解析 eBPF 程序
 * 捕获并解析 MySQL 查询语句
 *
 * 编译: clang -O2 -target bpf -c mysql_full_bpf.bpf.c -o mysql_full_bpf.bpf.o
 */
#include <linux/bpf.h>
#include <linux/tcp.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

#define MAX_SQL_LEN 128

/* MySQL 数据包头部 */
struct mysql_header {
    __u32 payload_len: 24;
    __u8 sequence_id;
} __attribute__((packed));

/* MySQL COM_QUERY 命令 */
#define MYSQL_COM_QUERY 0x03

/* MySQL 事件 */
struct mysql_event {
    __u64 timestamp;
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
    __u8 command;
    __u8 sql_len;
    char sql[MAX_SQL_LEN];
};

/* MySQL 查询统计 Map */
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 4096);
    __type(key, __u32);  /* 目标端口 */
    __type(value, __u64); /* 查询计数 */
} mysql_query_count SEC(".maps");

/* MySQL 事件 Ring Buffer */
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 19);  /* 512KB */
} mysql_events SEC(".maps");

/*
 * 检测 MySQL COM_QUERY 命令
 */
static __always_inline int is_mysql_query(const char *data, __u32 len, __u8 *command,
                                           char *sql_out, __u8 *sql_len_out) {
    if (len < 5) return 0;
    
    struct mysql_header *hdr = (struct mysql_header *)data;
    __u32 payload_len = hdr->payload_len;
    
    if (payload_len < 1 || payload_len > len - 4) {
        return 0;
    }
    
    __u8 cmd = data[4];
    *command = cmd;
    
    if (cmd == MYSQL_COM_QUERY && payload_len > 1) {
        __u8 sql_len = payload_len - 1;
        if (sql_len > MAX_SQL_LEN) {
            sql_len = MAX_SQL_LEN;
        }
        __builtin_memcpy(sql_out, data + 5, sql_len);
        *sql_len_out = sql_len;
        return 1;
    }
    
    return 0;
}

/*
 * TC 钩子 - MySQL 流量解析
 */
SEC("tc")
int mysql_full_capture(struct __sk_buff *skb) {
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
    
    /* MySQL 端口: 3306 */
    __u16 dport = bpf_ntohs(tcp->dest);
    __u16 sport = bpf_ntohs(tcp->source);
    if (dport != 3306 && sport != 3306) {
        return TC_ACT_OK;
    }
    
    void *payload = (void *)tcp + sizeof(*tcp);
    __u32 payload_len = data_end - payload;
    
    if (payload_len < 5) {
        return TC_ACT_OK;
    }
    
    __u8 command = 0;
    char sql[MAX_SQL_LEN] = {0};
    __u8 sql_len = 0;
    
    /* 检测 MySQL 查询 */
    if (is_mysql_query(payload, payload_len, &command, sql, &sql_len)) {
        /* 更新统计 */
        __u32 port_key = 3306;
        __u64 *count = bpf_map_lookup_elem(&mysql_query_count, &port_key);
        if (count) {
            (*count)++;
        } else {
            __u64 init = 1;
            bpf_map_update_elem(&mysql_query_count, &port_key, &init, BPF_ANY);
        }
        
        /* 发送事件到用户空间 */
        struct mysql_event *event = bpf_ringbuf_reserve(&mysql_events, sizeof(*event), 0);
        if (event) {
            event->timestamp = bpf_ktime_get_ns();
            event->saddr = ip->saddr;
            event->daddr = ip->daddr;
            event->sport = tcp->source;
            event->dport = tcp->dest;
            event->command = command;
            event->sql_len = sql_len;
            __builtin_memcpy(event->sql, sql, sql_len);
            
            bpf_ringbuf_submit(event, 0);
        }
    }
    
    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";
