// network_flow.bpf.c - STB eBPF network flow collector
// Uses BPF_PROG_TYPE_SOCKET_FILTER + AF_PACKET for kernel 5.4 compatibility
// No RINGBUF, no BTF required

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/in.h>

struct flow_event {
    __u64 timestamp_ns;
    __u32 pid;
    __u32 bytes;
    __u16 src_port;
    __u16 dst_port;
    __u8  protocol;
    __u8  direction;  // 0=rx, 1=tx
    __u8  tcp_flags;
    __u8  _pad;
    __u32 src_ip;
    __u32 dst_ip;
};

// Socket filter: capture all IPv4 packets
SEC("socket")
int flow_filter(struct __sk_buff *skb) {
    void *data_end = (void *)(long)skb->data_end;
    void *data = (void *)(long)skb->data;

    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return 0;

    // Only IPv4
    if (eth->h_proto != __constant_htons(ETH_P_IP))
        return 0;

    struct iphdr *ip = (void *)(eth + 1);
    if ((void *)(ip + 1) > data_end)
        return 0;

    // Capture all IP packets (return packet length for stats)
    return skb->len;
}

char _license[] SEC("license") = "GPL";
