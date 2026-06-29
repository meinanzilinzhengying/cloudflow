// cpu_sample.bpf.c - On-CPU profiling via perf event sampling
// Samples running processes at regular intervals to find CPU hotspots
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

struct cpu_sample {
    __u32 pid;
    __u32 cpu;
    __u64 ktime_ns;
    char comm[16];
};

SEC("perf_event")
int cpu_profile(struct bpf_perf_event_data *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;
    __u32 cpu = bpf_get_smp_processor_id();

    struct cpu_sample s = {};
    s.pid = pid;
    s.cpu = cpu;
    s.ktime_ns = bpf_ktime_get_ns();
    bpf_get_current_comm(&s.comm, sizeof(s.comm));

    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &s, sizeof(s));
    return 0;
}

char _license[] SEC("license") = "GPL";
