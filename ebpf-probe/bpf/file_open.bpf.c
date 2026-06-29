// file_open.bpf.c - Minimal file open monitor for kernel 5.4
#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

// ARM32 pt_regs
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
    char comm[16];
    char filename[64];
};

SEC("kprobe/do_filp_open")
int trace_file_open(struct pt_regs *ctx) {
    __u32 pid = bpf_get_current_pid_tgid() >> 32;

    struct file_event e = {};
    e.pid = pid;
    bpf_get_current_comm(&e.comm, sizeof(e.comm));

    // Try to read filename from arg1 (struct filename *)
    const char *filename = (const char *)ctx->uregs[0];
    bpf_probe_read_str(&e.filename, sizeof(e.filename), filename);

    bpf_perf_event_output(ctx, &events, BPF_F_CURRENT_CPU, &e, sizeof(e));
    return 0;
}

char _license[] SEC("license") = "GPL";
