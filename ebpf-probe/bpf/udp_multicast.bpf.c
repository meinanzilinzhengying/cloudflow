// udp_multicast.bpf.c - Track UDP multicast for IPTV monitoring on ARM32
#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

struct pt_regs {
    unsigned long uregs[18];
};

struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
    __uint(key_size, 4);
    __uint(value_size, 4);
} events SEC(".maps");

struct multicast_event {
    __u32 pid;
    __u32 daddr;
    __u16 dport;
    __u32 bytes;
    char comm[16];
};

SEC("kprobe/udp_sendmsg")
int trace_udp_send(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;

    // Check if destination is multicast (224.0.0.0 - 239.255.255.255)
    // We can't easily access the socket struct here, so we'll track all UDP sends
    // and filter multicast in userspace

    struct multicast_event e = {};
    e.pid = pid;
    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &e, sizeof(e));
    return 0;
}

char _license[] SEC("license") = "GPL";
