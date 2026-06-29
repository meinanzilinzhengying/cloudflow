// process_exec.bpf.c - Track process exec via kprobe on ARM32
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

struct exec_event {
    __u32 pid;
    __u32 ppid;
    char comm[16];
};

SEC("kprobe/do_execve")
int trace_exec(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;

    struct exec_event e = {};
    e.pid = pid;
    e.ppid = 0;
    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &e, sizeof(e));
    return 0;
}

char _license[] SEC("license") = "GPL";
