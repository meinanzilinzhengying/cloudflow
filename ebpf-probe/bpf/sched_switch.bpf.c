// sched_switch.bpf.c - Track on-cpu/off-cpu via kprobe on sched_switch
// Records context switches to calculate CPU time and sleep time per process
#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

struct pt_regs {
    unsigned long uregs[18];
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 8192);
    __type(key, __u32);      // pid
    __type(value, __u64);    // timestamp when scheduled in
} sched_in_ts SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
    __uint(key_size, 4);
    __uint(value_size, 4);
} events SEC(".maps");

struct sched_event {
    __u32 pid;
    __u32 prev_pid;
    __u64 on_cpu_ns;     // time spent on CPU
    __u64 off_cpu_ns;    // time spent off CPU (sleeping)
    char comm[16];
    char prev_comm[16];
};

// kprobe on __schedule - called when context switch happens
SEC("kprobe/__schedule")
int trace_schedule(struct pt_regs *ctx) {
    return 0;
}

// kretprobe on __schedule - after context switch completes
SEC("kretprobe/__schedule")
int trace_schedule_ret(struct pt_regs *ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;
    __u64 now = bpf_ktime_get_ns();

    // Check if this task was previously sleeping (off-cpu time)
    __u64 *prev_ts = bpf_map_lookup_elem(&sched_in_ts, &pid);
    __u64 off_cpu_ns = 0;
    if (prev_ts) {
        off_cpu_ns = now - *prev_ts;
        bpf_map_delete_elem(&sched_in_ts, &pid);
    }

    // Record when this task was scheduled in
    bpf_map_update_elem(&sched_in_ts, &pid, &now, BPF_ANY);

    // Only emit event if there's meaningful off-cpu time (> 1ms)
    if (off_cpu_ns > 1000000) {
        struct sched_event e = {};
        e.pid = pid;
        e.off_cpu_ns = off_cpu_ns;
        bpf_get_current_comm(&e.comm, sizeof(e.comm));

        bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &e, sizeof(e));
    }

    return 0;
}

char _license[] SEC("license") = "GPL";
