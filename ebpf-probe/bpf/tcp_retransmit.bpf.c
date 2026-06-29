// tcp_retransmit.bpf.c - Detect TCP retransmissions via kprobe on ARM32
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

struct retransmit_event {
    __u32 pid;
    __u32 saddr;
    __u32 daddr;
    __u16 sport;
    __u16 dport;
    char comm[16];
};

SEC("kprobe/tcp_retransmit_skb")
int trace_retransmit(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;

    struct retransmit_event e = {};
    e.pid = pid;
    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &e, sizeof(e));
    return 0;
}

char _license[] SEC("license") = "GPL";
