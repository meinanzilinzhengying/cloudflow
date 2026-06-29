// syscall.bpf.c - Track file operations via kprobe on ARM32
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

struct file_event {
    __u32 pid;
    __u32 op_type;  // 0=open, 1=close
    __u64 latency_ns;
    char comm[16];
};

// Track do_filp_open (file open)
SEC("kprobe/do_filp_open")
int trace_file_open_entry(struct pt_regs *ctx) {
    return 0;
}

SEC("kretprobe/do_filp_open")
int trace_file_open_exit(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;

    struct file_event e = {};
    e.pid = pid;
    e.op_type = 0;  // open
    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &e, sizeof(e));
    return 0;
}

// Track close
SEC("kprobe/__arm64_sys_close")
int trace_close_entry(struct pt_regs *ctx) {
    return 0;
}

SEC("kretprobe/__arm64_sys_close")
int trace_close_exit(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;

    struct file_event e = {};
    e.pid = pid;
    e.op_type = 1;  // close
    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &e, sizeof(e));
    return 0;
}

char _license[] SEC("license") = "GPL";
